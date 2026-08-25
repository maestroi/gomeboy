// Command gomeboy-agent runs a Game Boy ROM through pkg/gomeboy, bridged
// into the existing "web" display driver via webbridge.Adapter, and drives
// it with a stub agent loop that owns all emulator timing. The web display
// is an observer/controller only: it never advances the emulator.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/launch"
	"github.com/thelolagemann/gomeboy/pkg/display"
	"github.com/thelolagemann/gomeboy/pkg/display/web"
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"github.com/thelolagemann/gomeboy/pkg/webbridge"
	_ "net/http/pprof"
)

// ioToGomeboyButton maps each internal io.Button to the gomeboy.Button of
// the same name. Both types are uint8 enums but declare their constants in
// different orders (e.g. io.ButtonSelect is 2 while gomeboy.ButtonSelect is
// 3), so the mapping must be name-keyed: a bare gomeboy.Button(b) numeric
// cast would press the wrong button.
var ioToGomeboyButton = map[io.Button]gomeboy.Button{
	io.ButtonA:      gomeboy.ButtonA,
	io.ButtonB:      gomeboy.ButtonB,
	io.ButtonStart:  gomeboy.ButtonStart,
	io.ButtonSelect: gomeboy.ButtonSelect,
	io.ButtonUp:     gomeboy.ButtonUp,
	io.ButtonDown:   gomeboy.ButtonDown,
	io.ButtonLeft:   gomeboy.ButtonLeft,
	io.ButtonRight:  gomeboy.ButtonRight,
}

// runAgentLoop is the stub agent loop. It owns pacing: it steps the
// emulator on its own ticker, checks adapter.Paused() before every
// advancement, publishes each resulting frame through the bridge, and
// reports its state through the publisher. It never lets the display layer
// drive emulator timing. Extracted from main so it can be tested without a
// real WebSocket server.
func runAgentLoop(ctx context.Context, emu *gomeboy.Emulator, adapter *webbridge.Adapter, publisher web.AgentPublisher, frameInterval time.Duration) {
	var step uint64
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if adapter.Paused() {
				continue
			}

			// Stub decision: press no buttons, just step forward. A real
			// agent would decide input here and call emu.Press/Release.
			emu.StepFrame()
			adapter.PublishFrame()
			publisher.PublishAgentState(web.AgentState{
				Step:       step,
				Goal:       "stub: step the emulator",
				LastAction: "StepFrame",
				Status:     web.AgentRunning,
			})
			step++
		}
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is the testable boundary of the agent binary: it parses the CLI
// options, starts the emulator and the web display driver, and returns a
// contextual error on any failure. main logs the final error and exits
// non-zero.
func run(args []string) error {
	// the display drivers register their own flags (e.g. -web-listen) on
	// the global flag set, so the shared launch options are registered on
	// the same set and parsed together
	fs := flag.CommandLine
	fs.Init("gomeboy-agent", flag.ContinueOnError)

	// init display package (validates at least one driver is installed)
	display.Init()

	// the agent registers only the core launch options: it stays diskless
	// (no save flags) and always uses the web display driver (no -driver)
	launch.Register(fs)
	fps := fs.Int("fps", 60, "target frames per second for the agent loop")

	opts, err := launch.Parse(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if opts.ROM == "" {
		return fmt.Errorf("gomeboy-agent: -rom is required: usage: gomeboy-agent -rom <file.gb>")
	}
	if *fps <= 0 {
		return fmt.Errorf("gomeboy-agent: -fps must be positive, got %d", *fps)
	}

	logger, err := log.NewWithWriter(os.Stderr, opts.LogLevel)
	if err != nil {
		return err
	}

	stopPProf, err := launch.StartPProf(opts.PProfAddr, logger)
	if err != nil {
		return err
	}
	if stopPProf != nil {
		defer stopPProf()
	}

	emu, err := gomeboy.New(append(opts.PublicOptions(), gomeboy.WithROM(opts.ROM), gomeboy.Headless())...)
	if err != nil {
		return fmt.Errorf("gomeboy-agent: load ROM %s: %w", opts.ROM, err)
	}
	defer emu.Close()

	// framebuffer channel the display hub reads and the bridge publishes to
	fb := make(chan []byte, 120)
	adapter := webbridge.NewAdapter(emu, fb)

	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	driver := display.GetDriver("web")
	if driver == nil {
		return fmt.Errorf("gomeboy-agent: the web display driver is not installed")
	}

	// The web player broadcasts agent state to connected browser clients.
	publisher, ok := driver.(web.AgentPublisher)
	if !ok {
		return fmt.Errorf("gomeboy-agent: the web display driver does not implement AgentPublisher")
	}

	// Forward browser button presses to the emulator. This is the only
	// input path into the emulator besides the agent loop itself; there is
	// no arbitration between the two (accepted limitation of this stage).
	go func() {
		for {
			select {
			case b := <-pressed:
				emu.Press(ioToGomeboyButton[b])
			case b := <-released:
				emu.Release(ioToGomeboyButton[b])
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the display driver in the background so a startup failure (for
	// example the web hub failing to bind its listen address) surfaces as
	// an error instead of killing the process from a goroutine. It blocks
	// for the lifetime of the hub on success.
	startErr := make(chan error, 1)
	go func() {
		startErr <- driver.Start(adapter, fb, pressed, released)
	}()

	logger.Infof("gomeboy-agent: ROM %s loaded, web hub on %s", opts.ROM, webListenAddress())

	// The agent loop owns all emulator timing; it runs until ctx is done.
	go runAgentLoop(ctx, emu, adapter, publisher, time.Second/time.Duration(*fps))

	select {
	case err := <-startErr:
		if err != nil {
			return fmt.Errorf("gomeboy-agent: start display driver web: %w", err)
		}
		return fmt.Errorf("gomeboy-agent: the web display driver stopped unexpectedly")
	case <-ctx.Done():
		return nil
	}
}

// webListenAddress returns the address the web hub is listening on, as
// parsed from the -web-listen flag.
func webListenAddress() string {
	for _, d := range display.InstalledDrivers {
		if d.Name != "web" {
			continue
		}
		for _, opt := range d.Options {
			if opt.Name == "listen" {
				if addr, ok := opt.Value.(*string); ok {
					return *addr
				}
			}
		}
	}
	return ":8090"
}

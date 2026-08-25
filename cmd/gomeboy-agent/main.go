// Command gomeboy-agent runs a Game Boy ROM through pkg/gomeboy, bridged
// into the existing "web" display driver via webbridge.Adapter, and drives
// it with a stub agent loop that owns all emulator timing. The web display
// is an observer/controller only: it never advances the emulator.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/pkg/display"
	"github.com/thelolagemann/gomeboy/pkg/display/web"
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"github.com/thelolagemann/gomeboy/pkg/webbridge"
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
	// init display package (validates at least one driver is installed)
	display.Init()

	romFile := flag.String("rom", "", "The rom file to load")
	fps := flag.Int("fps", 60, "Target frames per second for the agent loop")
	flag.Parse()

	logger := log.New()

	if *romFile == "" {
		logger.Fatal("usage: gomeboy-agent -rom <file.gb>")
	}
	if *fps <= 0 {
		logger.Fatal("-fps must be positive")
	}

	emu, err := gomeboy.New(gomeboy.WithROM(*romFile), gomeboy.Headless())
	if err != nil {
		logger.Fatal(fmt.Sprintf("unable to load ROM %s: %s", *romFile, err))
	}
	defer emu.Close()

	// framebuffer channel the display hub reads and the bridge publishes to
	fb := make(chan []byte, 120)
	adapter := webbridge.NewAdapter(emu, fb)

	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	driver := display.GetDriver("web")
	if driver == nil {
		logger.Fatal("the web display driver is not installed")
	}

	// The web player broadcasts agent state to connected browser clients.
	publisher, ok := driver.(web.AgentPublisher)
	if !ok {
		logger.Fatal("the web display driver does not implement AgentPublisher")
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

	// Start the display driver (blocks forever consuming fb). It observes
	// frames published by the agent loop; it never steps the emulator.
	go func() {
		if err := driver.Start(adapter, fb, pressed, released); err != nil {
			logger.Fatal(err.Error())
		}
	}()

	logger.Infof("gomeboy-agent: ROM %s loaded, web hub on :8090", *romFile)

	runAgentLoop(ctx, emu, adapter, publisher, time.Second/time.Duration(*fps))
}

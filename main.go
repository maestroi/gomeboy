package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/launch"
	"github.com/thelolagemann/gomeboy/pkg/audio"
	"github.com/thelolagemann/gomeboy/pkg/display"
	_ "github.com/thelolagemann/gomeboy/pkg/display/fyne"
	_ "github.com/thelolagemann/gomeboy/pkg/display/glfw"
	_ "github.com/thelolagemann/gomeboy/pkg/display/web"
	"github.com/thelolagemann/gomeboy/pkg/log"
	_ "net/http/pprof"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is the testable boundary of the desktop binary: it parses the CLI
// options, starts the emulator and the display driver, and returns a
// contextual error on any failure. main logs the final error and exits
// non-zero.
func run(args []string) error {
	// the display drivers register their own flags (e.g. -web-listen) on
	// the global flag set, so the shared launch options are registered on
	// the same set and parsed together
	fs := flag.CommandLine
	fs.Init("gomeboy", flag.ContinueOnError)

	// init display package (validates at least one driver is installed)
	display.Init()

	launch.Register(fs)
	launch.RegisterSaves(fs)
	launch.RegisterDriver(fs, "auto")

	opts, err := launch.Parse(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
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

	gbOpts, err := opts.CoreOptions()
	if err != nil {
		return err
	}

	gb := gameboy.NewGameBoy(gbOpts...)

	if opts.ROM != "" {
		if err := gb.LoadROM(opts.ROM); err != nil {
			return fmt.Errorf("gomeboy: load ROM %s: %w", opts.ROM, err)
		}
	}

	// create framebuffer
	fb := make(chan []byte, 120)

	// create various channels
	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	if err := audio.OpenAudio(gb, fb); err != nil {
		return fmt.Errorf("gomeboy: open audio device: %w", err)
	}

	driver := display.GetDriver(opts.Driver)
	if driver == nil {
		return fmt.Errorf("gomeboy: unknown display driver %q: use auto or one of %s", opts.Driver, installedDriverNames())
	}

	// is the driver capable of debugging?
	if debugger, ok := driver.(display.DriverDebugger); ok {
		debugger.AttachGameboy(gb)
	}

	// handle input
	go func() {
		for {
			select {
			case b := <-pressed:
				gb.Bus.Press(b)
			case b := <-released:
				gb.Bus.Release(b)
			}
		}
	}()

	// start the display driver (blocking)
	if err := driver.Start(gb, fb, pressed, released); err != nil {
		return fmt.Errorf("gomeboy: start display driver %q: %w", opts.Driver, err)
	}

	// save after the driver has stopped (TODO: stop audio from driving the
	// Game Boy before this)
	if err := gb.Save(); err != nil {
		return fmt.Errorf("gomeboy: save battery RAM for ROM %s: %w", opts.ROM, err)
	}

	return nil
}

// installedDriverNames lists the display drivers compiled into this binary.
func installedDriverNames() string {
	names := make([]string, 0, len(display.InstalledDrivers))
	for _, d := range display.InstalledDrivers {
		names = append(names, d.Name)
	}
	return strings.Join(names, ", ")
}

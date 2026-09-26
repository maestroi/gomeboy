package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/launch"
	"github.com/maestroi/gomeboy/pkg/audio"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/gomeboy/pkg/display"
	_ "github.com/maestroi/gomeboy/pkg/display/glfw"
	"github.com/maestroi/gomeboy/pkg/log"
	_ "net/http/pprof"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.CommandLine
	fs.Init("gomeboy", flag.ContinueOnError)

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

	emuOpts := opts.DesktopOptions()
	if opts.ROM != "" {
		emuOpts = append(emuOpts, gomeboy.WithROM(opts.ROM))
	}
	emu, err := gomeboy.New(emuOpts...)
	if err != nil {
		if opts.ROM != "" {
			return fmt.Errorf("gomeboy: load ROM %s: %w", opts.ROM, err)
		}
		return fmt.Errorf("gomeboy: initialize emulator: %w", err)
	}
	defer emu.Close()

	fb := make(chan []byte, 120)
	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	if err := audio.OpenEmulator(emu, fb); err != nil {
		return fmt.Errorf("gomeboy: open audio device: %w", err)
	}

	driver := display.GetDriver(opts.Driver)
	if driver == nil {
		return fmt.Errorf("gomeboy: unknown display driver %q: use auto or one of %s", opts.Driver, installedDriverNames())
	}

	go func() {
		for {
			select {
			case b := <-pressed:
				if button, ok := publicButton(b); ok {
					emu.Press(button)
				}
			case b := <-released:
				if button, ok := publicButton(b); ok {
					emu.Release(button)
				}
			}
		}
	}()

	if err := driver.Start(emu, fb, pressed, released); err != nil {
		return fmt.Errorf("gomeboy: start display driver %q: %w", opts.Driver, err)
	}

	return nil
}

func installedDriverNames() string {
	names := make([]string, 0, len(display.InstalledDrivers))
	for _, d := range display.InstalledDrivers {
		names = append(names, d.Name)
	}
	return strings.Join(names, ", ")
}


func publicButton(button io.Button) (gomeboy.Button, bool) {
	switch button {
	case io.ButtonA:
		return gomeboy.ButtonA, true
	case io.ButtonB:
		return gomeboy.ButtonB, true
	case io.ButtonStart:
		return gomeboy.ButtonStart, true
	case io.ButtonSelect:
		return gomeboy.ButtonSelect, true
	case io.ButtonUp:
		return gomeboy.ButtonUp, true
	case io.ButtonDown:
		return gomeboy.ButtonDown, true
	case io.ButtonLeft:
		return gomeboy.ButtonLeft, true
	case io.ButtonRight:
		return gomeboy.ButtonRight, true
	default:
		return 0, false
	}
}

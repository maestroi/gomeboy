package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/launch"
	"github.com/maestroi/gomeboy/pkg/audio"
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

	fb := make(chan []byte, 120)
	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	if err := audio.OpenAudio(gb, fb); err != nil {
		return fmt.Errorf("gomeboy: open audio device: %w", err)
	}

	driver := display.GetDriver(opts.Driver)
	if driver == nil {
		return fmt.Errorf("gomeboy: unknown display driver %q: use auto or one of %s", opts.Driver, installedDriverNames())
	}

	if debugger, ok := driver.(display.DriverDebugger); ok {
		debugger.AttachGameboy(gb)
	}

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

	if err := driver.Start(gb, fb, pressed, released); err != nil {
		return fmt.Errorf("gomeboy: start display driver %q: %w", opts.Driver, err)
	}

	if err := gb.Save(); err != nil {
		return fmt.Errorf("gomeboy: save battery RAM for ROM %s: %w", opts.ROM, err)
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

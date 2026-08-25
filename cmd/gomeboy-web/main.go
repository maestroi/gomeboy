package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/ppu"
	"github.com/thelolagemann/gomeboy/internal/types"
	"github.com/thelolagemann/gomeboy/pkg/display"
	_ "github.com/thelolagemann/gomeboy/pkg/display/web"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"github.com/thelolagemann/gomeboy/pkg/utils"
)

func main() {
	// init display package
	display.Init()

	var logger = log.New()
	// create framebuffer
	fb := make(chan []byte, 120)

	// create various channels
	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)

	romFile := flag.String("rom", "", "The rom file to load")
	bootROM := flag.String("boot", "", "The boot rom file to load")
	asModel := flag.String("model", "auto", "The model to emulate. Can be auto, dmg or cgb")
	printer := flag.Bool("printer", false, "enable printer")
	displayDriver := flag.String("driver", "web", "The display driver to use. Can be web")

	flag.Parse()

	var gb *gameboy.GameBoy
	var opts []gameboy.Opt

	if *bootROM != "" {
		boot, err := utils.LoadFile(*bootROM)
		if err != nil {
			panic(err)
		}

		opts = append(opts, gameboy.WithBootROM(boot))
	}

	if *printer {
		opts = append(opts, gameboy.WithPrinter())
	}

	// has model been set?
	if *asModel != "auto" {
		opts = append(opts, gameboy.AsModel(types.StringToModel(*asModel)))
	}

	if *romFile != "" {
		base := strings.TrimSuffix(filepath.Base(*romFile), filepath.Ext(*romFile))
		opts = append(opts, gameboy.WithCheats(base+".cheats"))
	}

	// create a new gameboy
	gb = gameboy.NewGameBoy(opts...)

	if *romFile != "" {
		if err := gb.LoadROM(*romFile); err != nil {
			logger.Errorf("unable to load ROM %s: %s", *romFile, err)
		}
	}

	// no audio device in a headless container: drive the emulator from a
	// 60Hz ticker (replacing the audio callback in the desktop main.go)
	// and push rendered frames to the display driver
	go func() {
		t := time.NewTicker(time.Second / 60)
		defer t.Stop()
		buf := make([]byte, ppu.ScreenWidth*ppu.ScreenHeight*3)
		for range t.C {
			if gb.Paused() || !gb.Initialised() {
				continue
			}

			speed := gb.Speed()
			var frame [ppu.ScreenHeight][ppu.ScreenWidth][3]uint8
			for i := 0; i < speed; i++ {
				frame = gb.Frame()
			}
			copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(&frame[0])), len(buf)))
			select {
			case fb <- buf:
			default:
			}
		}
	}()

	driver := display.GetDriver(*displayDriver)

	// check to make sure the driver is valid
	if driver == nil {
		logger.Fatal("invalid display driver")
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
		logger.Fatal(err.Error())
	}

	// save after the driver has stopped
	if err := gb.Save(); err != nil {
		logger.Fatal(fmt.Sprintf("unable to save: %v", err))
	}
}

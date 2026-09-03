package gameboy

import (
	"errors"
	"fmt"
	"github.com/maestroi/gomeboy/internal/apu"
	"github.com/maestroi/gomeboy/internal/cpu"
	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/ppu"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/serial"
	"github.com/maestroi/gomeboy/internal/timer"
	"github.com/maestroi/gomeboy/internal/types"
	"github.com/maestroi/gomeboy/pkg/emulator"
	"github.com/maestroi/gomeboy/pkg/log"
	"github.com/maestroi/gomeboy/pkg/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GameBoy represents the state and components of the Game Boy emulator.
type GameBoy struct {
	CPU       *cpu.CPU
	PPU       *ppu.PPU
	APU       *apu.APU
	Timer     *timer.Controller
	Serial    *serial.Controller
	Scheduler *scheduler.Scheduler
	Bus       *io.Bus

	save            *emulator.Save
	filename        string
	model           types.Model
	frames          uint64
	dontBoot        bool
	rumbling        bool
	paused, running bool
	initialised     bool
	saveDir         string
	noSaves         bool
	cheatsPath      string
	speed           int

	// mu serialises the frame loop (audio/ticker thread) against save/load
	// (UI thread) so Snapshot/Restore can't rewrite PPU/APU state mid-frame.
	mu sync.Mutex

	ROM     []byte
	options []Opt
}

// NewGameBoy creates a new GameBoy with the provided Opt(s).
func NewGameBoy(opts ...Opt) *GameBoy { return &GameBoy{options: opts, speed: 1} }

// WithSaveDir sets the directory that save files (.sav) and quick-save state
// files (.state) are read from and written to. An empty dir keeps the
// historical behaviour of using the process' working directory.
func WithSaveDir(dir string) Opt { return func(gb *GameBoy) { gb.saveDir = dir } }

// WithoutSaves disables battery-backed save file I/O entirely: no save file
// is read at initialisation and none is written on Save or Close.
func WithoutSaves() Opt { return func(gb *GameBoy) { gb.noSaves = true } }

// WithCheats sets the exact path of the cheats file to load at
// initialisation, instead of probing the working directory for one.
func WithCheats(path string) Opt { return func(gb *GameBoy) { gb.cheatsPath = path } }

// LoadROM loads a ROM file from the specified path and initializes the Game Boy.
//
// It accepts a string representing the absolute path to the ROM file.
// If loading fails, it will return an error.
//
// Example:
//
//	gb.LoadROM("path/to/rom.gb")
func (g *GameBoy) LoadROM(romPath string) error {
	var err error
	g.ROM, err = utils.LoadFile(romPath)
	if err != nil {
		return err
	}
	g.filename = strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	g.Init()
	return nil
}

// LoadROMBytes initialises the Game Boy from an in-memory ROM image. name
// is used for save/state file naming and may be empty.
func (g *GameBoy) LoadROMBytes(rom []byte, name string) error {
	if len(rom) == 0 {
		return errors.New("gomeboy: empty ROM")
	}
	g.ROM = rom
	g.filename = name
	g.Init()
	return nil
}

// Init initializes the Game Boy and its components, including CPU, PPU, APU, Timer, and Bus.
//
// It sets up the scheduler, maps memory, and configures the system according to the loaded ROM and model.
// This function also handles loading save files for battery-backed cartridges.
func (g *GameBoy) Init() {
	sched := scheduler.NewScheduler()

	b := io.NewBus(sched, g.ROM)
	serialCtl := serial.NewController(b, sched)
	sound := apu.New(b, sched)
	timerCtl := timer.NewController(b, sched, sound)
	video := ppu.New(b, sched)
	processor := cpu.NewCPU(b, sched)

	var model = types.DMGABC
	if b.Cartridge().IsCGBCartridge() {
		model = types.CGBABC
	}

	g.CPU = processor
	g.PPU = video
	g.Bus = b
	g.Serial = serialCtl
	g.Timer = timerCtl
	g.APU = sound
	g.Scheduler = sched
	g.model = model

	for _, o := range g.options {
		o(g)
	}

	// does the cartridge have battery backed RAM? (and therefore a save file)
	if !g.noSaves && b.Cartridge().Features.Battery {
		// only load the save file from disk on the first initialisation;
		// on re-initialisation (Reset) the live cartridge RAM is preserved
		if g.save == nil {
			// try to load the save file
			var err error
			g.save, err = emulator.NewSave(filepath.Join(g.saveDir, g.filename), uint(b.Cartridge().RAMSize))

			if err != nil {
				// was there an error loading the save files?
				log.Errorf(fmt.Sprintf("error loading save files: %s", err))
			} else {
				copy(g.Bus.Cartridge().RAM, g.save.Bytes())
				var length = len(g.save.Bytes())
				if length > 0x2000 {
					length = 0x2000
				}
				g.Bus.CopyTo(0xA000, 0xC000, g.save.Bytes()[:length])
			}
		}
	}

	// try to find the cheats file, if a path was configured
	if g.cheatsPath != "" {
		cheatFile, err := os.Open(g.cheatsPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Errorf("error opening %s file: %s", g.cheatsPath, err)
		} else if err == nil {
			cheats, err := io.ParseCheats(cheatFile)
			if err != nil {
				log.Errorf("error parsing %s: %s", g.cheatsPath, err)
			} else {
				g.Bus.LoadedCheats = cheats // so that gui can easily read/modify

				// parse cheats according to type and load into bus
				for _, c := range g.Bus.LoadedCheats {
					if c.Enabled {
						for _, code := range c.Codes {
							switch len(code) {
							case 8:
								cheat, err := io.ParseGameSharkCode(code)
								if err != nil {
									log.Errorf("error parsing GameShark code %s: %s", code, err)
								} else {
									g.Bus.GameSharkCodes = append(g.Bus.GameSharkCodes, cheat)
								}
							case 11:
								cheat, err := io.ParseGameGenieCode(code)
								if err != nil {
									log.Errorf("error parsing GameGenie code %s: %s", code, err)
								} else {
									g.Bus.GameGenieCodes = append(g.Bus.GameGenieCodes, cheat)
								}
							default:
								log.Errorf("error parsing code: %s", code)
							}
						}
					}
				}
			}
		}
	}

	g.Bus.Map(g.model)
	g.Colourise()
	if !g.dontBoot {
		g.CPU.Boot(g.model)
		g.Bus.Boot()
	}

	g.Bus.Cartridge().RumbleCallback = func(b bool) {
		g.rumbling = b
	}

	// schedule the frame sequencer event for the next 8192 ticks
	g.Scheduler.ScheduleEvent(scheduler.APUFrameSequencer, uint64(8192-g.Scheduler.SysClock()&0x0fff))
	g.Scheduler.ScheduleEvent(scheduler.APUFrameSequencer2, uint64(8192-g.Scheduler.SysClock()&0x0fff)+4096)
	g.initialised = true
}

// Colourise applies color palettes to the Game Boy's PPU based on the cartridge type and system model.
//
// If the loaded cartridge is not a Game Boy Color (CGB) cartridge and the Game Boy model is set to CGB, a color palette
// is selected based on the cartridge's licensee and title. If no specific palette is found, a default palette is used.
//
// For non-CGB models, a greyscale palette is applied to the PPU.
func (g *GameBoy) Colourise() {
	if !g.Bus.Cartridge().IsCGBCartridge() && (g.model == types.CGBABC || g.model == types.CGB0) {
		var pal = ppu.ColourisationPalettes[0]
		if g.Bus.Cartridge().Licensee() == "Nintendo" {
			// compute title hash
			hash := uint8(0)
			title := []byte(g.Bus.Cartridge().Title)
			for i := 0; i < len(title); i++ {
				hash += title[i]
			}
			var ok bool
			pal, ok = ppu.ColourisationPalettes[uint16(hash)]

			if !ok {
				pal, ok = ppu.ColourisationPalettes[uint16(title[3])<<8|uint16(hash)]
				if !ok {
					pal = ppu.ColourisationPalettes[0]
				}
			}
		}
		g.PPU.BGColourisationPalette = pal.BG
		g.PPU.OBJ0ColourisationPalette = pal.OBJ0
		g.PPU.OBJ1ColourisationPalette = pal.OBJ1
	} else {
		g.PPU.BGColourisationPalette = ppu.ColourPalettes[ppu.Greyscale]
		g.PPU.OBJ0ColourisationPalette = ppu.ColourPalettes[ppu.Greyscale]
		g.PPU.OBJ1ColourisationPalette = ppu.ColourPalettes[ppu.Greyscale]
	}
}

// Frame generates the next frame of the Game Boy's display and applies any visual effects.
func (g *GameBoy) Frame() [ppu.ScreenHeight][ppu.ScreenWidth][3]uint8 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = true
	g.CPU.Frame()

	if g.rumbling {
		// utils.ShakeFrame(&g.PPU.PreparedFrame, rand.N(100))
	}
	if g.Bus.Cartridge().Features.Accelerometer {
		// utils.Rotate2DFrame(&g.PPU.PreparedFrame, -float64(g.Bus.Cartridge().AccelerometerX), float64(g.Bus.Cartridge().AccelerometerY)) // TODO make configurable
	}
	g.running = false
	g.frames++
	return g.PPU.PreparedFrame
}

// Step advances the emulator by exactly one frame. Unlike Frame, it does
// not copy the rendered frame, so it avoids allocating on the hot path.
// Use FrameBuffer to access the rendered frame.
func (g *GameBoy) Step() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = true
	g.CPU.Frame()
	g.running = false
	g.frames++
}

// StepFrames advances n frames while taking the frame/save-state mutex once.
// This preserves the same serialization guarantee as Step without paying one
// mutex lock/unlock pair per frame in batch/agent workloads.
func (g *GameBoy) StepFrames(n int) {
	if n <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = true
	for i := 0; i < n; i++ {
		g.CPU.Frame()
		g.frames++
	}
	g.running = false
}

// FrameCount returns the number of frames the emulator has advanced since the
// ROM was loaded or the emulator was last Reset.
func (g *GameBoy) FrameCount() uint64 { return g.frames }

// Cycle returns the emulator's current master clock cycle.
func (g *GameBoy) Cycle() uint64 { return g.Scheduler.Cycle() }

// FrameBuffer returns a pointer to the most recently rendered frame.
// The contents are overwritten by the next call to Step or Frame.
func (g *GameBoy) FrameBuffer() *[ppu.ScreenHeight][ppu.ScreenWidth][3]uint8 {
	return &g.PPU.PreparedFrame
}

// Reset returns the emulator to its initial boot state, reusing the ROM
// that is already loaded in memory (it is not re-read from disk).
// Battery-backed cartridge RAM is preserved across the reset.
func (g *GameBoy) Reset() error {
	if g.ROM == nil {
		return errors.New("gomeboy: cannot reset: no ROM loaded")
	}

	// preserve battery-backed cartridge RAM across the reset
	var batteryRAM []byte
	if g.Bus != nil && g.Bus.Cartridge().Features.Battery {
		batteryRAM = append([]byte(nil), g.Bus.Cartridge().RAM...)
	}

	g.Init()
	g.frames = 0

	if batteryRAM != nil {
		copy(g.Bus.Cartridge().RAM, batteryRAM)
	}
	return nil
}

// Save writes the current state of the Game Boy's RAM to the save file, if a save is present.
func (g *GameBoy) Save() error {
	// close the save file
	if g.save != nil {
		b := g.Bus.Cartridge().RAM
		if err := g.save.SetBytes(b); err != nil {
			return err
		}
		if err := g.save.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (g *GameBoy) Initialised() bool { return g.initialised } // has the Game Boy been initialised?
func (g *GameBoy) Paused() bool      { return g.paused }      // is the Game Boy paused?
func (g *GameBoy) Pause()            { g.paused = true }      // pause execution of the Game Boy
func (g *GameBoy) Resume()           { g.paused = false }     // resume execution of the Game Boy
func (g *GameBoy) Running() bool     { return g.running }     // is the emulator currently running?

// SetSpeed sets the emulation speed multiplier. 1 is normal speed. Values
// less than 1 are clamped to 1 — this does not support slow motion.
// Values above 8 are clamped to 8.
func (g *GameBoy) SetSpeed(speed int) {
	if speed < 1 {
		speed = 1
	}
	if speed > 8 {
		speed = 8
	}
	g.speed = speed
}

// Speed returns the current emulation speed multiplier. It defaults to 1.
func (g *GameBoy) Speed() int {
	if g.speed == 0 {
		return 1
	}
	return g.speed
}

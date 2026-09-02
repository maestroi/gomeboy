// Package gomeboy exposes GomeBoy as a clean, headless Go library for
// programmatic, in-process Game Boy emulation.
//
// An Emulator is fully self-contained: it has no GUI, no audio device, and no
// realtime throttling. You advance it deterministically one frame at a time
// with StepFrame/StepFrames, feed it joypad input with Press/Release, read
// its memory with Read8/Read, and retrieve the rendered frame with Frame.
//
// Multiple Emulator instances are independent and may be created and run in
// the same process. A single Emulator instance is not safe for concurrent use
// by multiple goroutines; the caller must synchronize access.
//
// An Emulator performs no disk I/O unless WithSaveDir or WithCheats is
// passed: by default, battery-backed save files are neither read nor written
// and no cheats file is read, so any number of instances may run the same ROM
// in parallel without sharing files. Pass WithSaveDir to enable persistence
// into a per-instance directory, and WithCheats to load cheats from an
// explicit file.
package gomeboy

import (
	"fmt"

	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/ppu"
	"github.com/thelolagemann/gomeboy/internal/types"
	"github.com/thelolagemann/gomeboy/pkg/utils"
	"unsafe"
)

// Button is a joypad button.
type Button uint8

// The joypad buttons, in the order a caller is most likely to reference them.
const (
	ButtonA Button = iota
	ButtonB
	ButtonStart
	ButtonSelect
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
)

// buttonMap maps a public Button to the internal io.Button it corresponds to.
var buttonMap = [8]io.Button{
	io.ButtonA,
	io.ButtonB,
	io.ButtonStart,
	io.ButtonSelect,
	io.ButtonUp,
	io.ButtonDown,
	io.ButtonLeft,
	io.ButtonRight,
}

// Model selects the Game Boy hardware the emulator emulates. The zero value
// is not a valid model; pass ModelAuto to keep the model inferred from the
// loaded cartridge (the default when WithModel is not passed).
type Model string

// The supported hardware models.
const (
	ModelAuto Model = "auto" // infer the model from the loaded cartridge
	ModelDMG0 Model = "DMG0" // early Game Boy (Japan)
	ModelDMG  Model = "DMG"  // Game Boy
	ModelCGB0 Model = "CGB0" // early Game Boy Color (Japan)
	ModelCGB  Model = "CGB"  // Game Boy Color
	ModelMGB  Model = "MGB"  // Pocket Game Boy
	ModelSGB  Model = "SGB"  // Super Game Boy
	ModelSGB2 Model = "SGB2" // Super Game Boy 2
	ModelAGB  Model = "AGB"  // Game Boy Advance
)

// modelMap maps the public models to the internal hardware models. ModelAuto
// is deliberately absent: it keeps the cartridge-based inference.
var modelMap = map[Model]types.Model{
	ModelDMG0: types.DMG0,
	ModelDMG:  types.DMGABC,
	ModelCGB0: types.CGB0,
	ModelCGB:  types.CGBABC,
	ModelMGB:  types.MGB,
	ModelSGB:  types.SGB,
	ModelSGB2: types.SGB2,
	ModelAGB:  types.AGB,
}

// publicModelMap maps the internal hardware models back to the public models.
var publicModelMap = map[types.Model]Model{
	types.DMG0:   ModelDMG0,
	types.DMGABC: ModelDMG,
	types.CGB0:   ModelCGB0,
	types.CGBABC: ModelCGB,
	types.MGB:    ModelMGB,
	types.SGB:    ModelSGB,
	types.SGB2:   ModelSGB2,
	types.AGB:    ModelAGB,
}

// Frame is the rendered output of a single frame, in 24-bit RGB.
type Frame struct {
	Width  int
	Height int
	RGB    []byte // Width*Height*3 bytes, row-major, 3 bytes (R,G,B) per pixel
}

// Emulator is a headless Game Boy emulator instance. It is not safe for
// concurrent use by multiple goroutines.
type Emulator struct {
	gb *gameboy.GameBoy
}

type config struct {
	romPath  string
	romBytes []byte
	romName  string
	bootROM  string
	headless bool
	saveDir  string
	saves    bool
	model    Model
	printer  bool
	cheats   string
}

// Option configures an Emulator at construction time.
type Option func(*config)

// WithROM sets the ROM to load when the Emulator is created. The emulator is
// ready to step immediately after New returns.
func WithROM(path string) Option {
	return func(c *config) { c.romPath = path }
}

// WithROMBytes loads an in-memory ROM image. It takes precedence over
// WithROM if both are supplied.
func WithROMBytes(rom []byte) Option {
	return func(c *config) {
		c.romBytes = rom
		if c.romName == "" {
			c.romName = "rom"
		}
	}
}

// WithBootROM sets the boot ROM (a .gbr file) to use instead of the built-in
// hardware-level-emulated boot process.
func WithBootROM(path string) Option {
	return func(c *config) { c.bootROM = path }
}

// WithSaveDir enables battery-backed save file persistence and sets the
// directory in which .sav and .state files are read from and written to.
// Without it, the Emulator performs no disk I/O at all.
func WithSaveDir(dir string) Option {
	return func(c *config) {
		c.saveDir = dir
		c.saves = true
	}
}

// Headless disables APU sample accumulation. The core is already headless
// (no GUI, no audio device); this option only prevents the APU from growing
// its sample buffer, so long-running headless emulation does not leak memory.
func Headless() Option {
	return func(c *config) { c.headless = true }
}

// WithModel selects the hardware model to emulate, overriding the model
// inferred from the cartridge. ModelAuto (the default) keeps the inference.
// The model is fixed at construction time; there is no way to change it on a
// running Emulator.
func WithModel(m Model) Option {
	return func(c *config) { c.model = m }
}

// WithPrinter attaches the serial printer accessory to the emulator. Without
// it, no device is attached (the default).
func WithPrinter() Option {
	return func(c *config) { c.printer = true }
}

// WithCheats loads cheats from the file at path when the ROM is initialised.
// The path is used exactly as given; the working directory is never probed.
// Without it, no cheats file is read.
func WithCheats(path string) Option {
	return func(c *config) { c.cheats = path }
}

// New creates a new Emulator. If a ROM was supplied via WithROM, it is loaded
// and the emulator is ready to step. Otherwise, call LoadROM before stepping.
func New(opts ...Option) (*Emulator, error) {
	cfg := &config{model: ModelAuto}
	for _, o := range opts {
		o(cfg)
	}

	var gbOpts []gameboy.Opt
	if cfg.bootROM != "" {
		bootROM, err := utils.LoadFile(cfg.bootROM)
		if err != nil {
			return nil, err
		}
		gbOpts = append(gbOpts, gameboy.WithBootROM(bootROM))
	}

	// appended after WithBootROM so an explicit model overrides the model
	// detected from the boot ROM
	if cfg.model != ModelAuto {
		internal, ok := modelMap[cfg.model]
		if !ok {
			return nil, fmt.Errorf("gomeboy: unknown model %q: use auto, DMG0, DMG, CGB0, CGB, MGB, SGB, SGB2, or AGB", cfg.model)
		}
		gbOpts = append(gbOpts, gameboy.AsModel(internal))
	}

	if cfg.printer {
		gbOpts = append(gbOpts, gameboy.WithPrinter())
	}

	if cfg.cheats != "" {
		gbOpts = append(gbOpts, gameboy.WithCheats(cfg.cheats))
	}

	if cfg.saves {
		gbOpts = append(gbOpts, gameboy.WithSaveDir(cfg.saveDir))
	} else {
		gbOpts = append(gbOpts, gameboy.WithoutSaves())
	}

	e := &Emulator{gb: gameboy.NewGameBoy(gbOpts...)}

	// romName is only ever set by WithROMBytes, so it records whether an
	// in-memory ROM was supplied (possibly empty, which is an error).
	if cfg.romName != "" {
		if err := e.gb.LoadROMBytes(cfg.romBytes, cfg.romName); err != nil {
			return nil, err
		}
	} else if cfg.romPath != "" {
		if err := e.LoadROM(cfg.romPath); err != nil {
			return nil, err
		}
	}

	if cfg.headless {
		e.gb.APU.SetHeadless(true)
	}

	return e, nil
}

// Model returns the hardware model the emulator is emulating. When ModelAuto
// is in effect (the default), this is the model inferred from the loaded
// cartridge, or ModelAuto before a ROM has been loaded.
func (e *Emulator) Model() Model {
	if e.gb.Bus == nil {
		return ModelAuto
	}
	if m, ok := publicModelMap[e.gb.Bus.Model()]; ok {
		return m
	}
	return ModelAuto
}

// LoadROM loads a ROM from disk and (re)initializes the emulator.
func (e *Emulator) LoadROM(path string) error {
	return e.gb.LoadROM(path)
}

// LoadROMBytes loads an in-memory ROM image and (re)initializes the emulator.
// name is used for save/state file naming and may be empty.
func (e *Emulator) LoadROMBytes(rom []byte, name string) error {
	return e.gb.LoadROMBytes(rom, name)
}

// Press presses a joypad button.
func (e *Emulator) Press(b Button) {
	e.gb.Bus.Press(buttonMap[b])
}

// Release releases a joypad button.
func (e *Emulator) Release(b Button) {
	e.gb.Bus.Release(buttonMap[b])
}

// StepFrame advances the emulator by exactly one frame.
func (e *Emulator) StepFrame() {
	e.gb.Step()
}

// StepFrames advances the emulator by n frames.
func (e *Emulator) StepFrames(n int) {
	e.gb.StepFrames(n)
}

// FrameCount returns the number of frames this Emulator has advanced since
// the ROM was loaded or the emulator was last Reset.
func (e *Emulator) FrameCount() uint64 {
	return e.gb.FrameCount()
}

// Cycle returns the emulator's current master clock cycle.
func (e *Emulator) Cycle() uint64 {
	return e.gb.Cycle()
}

// Read8 performs a CPU-accurate read of a single byte from the emulator's
// 16-bit address space. The result can be affected by DMA conflicts and PPU
// region locks, so it is not a pure observation of memory. Use Peek8 or
// PeekInto to observe memory without side effects.
func (e *Emulator) Read8(addr uint16) byte {
	return e.gb.Bus.Read(addr)
}

// Read performs CPU-accurate reads of length bytes starting at addr from the
// emulator's address space. Each byte can be affected by DMA conflicts and
// PPU region locks. Use Peek8 or PeekInto to observe memory without side
// effects.
func (e *Emulator) Read(addr uint16, length int) []byte {
	out := make([]byte, length)
	for i := range length {
		out[i] = e.gb.Bus.Read(addr + uint16(i))
	}
	return out
}

// Frame returns the most recently rendered frame as a zero-copy view.
//
// The returned Frame.RGB slice references internal emulator memory and is
// valid only until the next call to StepFrame or StepFrames. Copy the bytes
// if you need to keep them.
func (e *Emulator) Frame() Frame {
	fb := e.gb.FrameBuffer()
	return Frame{
		Width:  ppu.ScreenWidth,
		Height: ppu.ScreenHeight,
		RGB:    unsafe.Slice(&(*fb)[0][0][0], ppu.ScreenWidth*ppu.ScreenHeight*3),
	}
}

// Reset returns the emulator to its initial boot state, reusing the ROM that
// is already loaded (it is not re-read from disk). Battery-backed cartridge
// RAM is preserved across the reset.
func (e *Emulator) Reset() error {
	return e.gb.Reset()
}

// SaveState serializes the emulator's complete execution state into a byte
// slice. The bytes can be passed to LoadState (on this or another Emulator
// running the same ROM) to restore the exact state.
func (e *Emulator) SaveState() ([]byte, error) {
	return e.gb.SaveState()
}

// LoadState restores the emulator to a state previously produced by
// SaveState.
func (e *Emulator) LoadState(data []byte) error {
	return e.gb.LoadState(data)
}

// QuickSave writes the complete emulator state to <romname>.state, where
// romname is the loaded ROM's base name without its extension.
func (e *Emulator) QuickSave() error {
	return e.gb.QuickSave()
}

// QuickLoad restores the emulator state from <romname>.state.
func (e *Emulator) QuickLoad() error {
	return e.gb.QuickLoad()
}

// Close flushes any pending battery-backed save data to disk and releases
// resources held by the emulator.
func (e *Emulator) Close() error {
	return e.gb.Save()
}

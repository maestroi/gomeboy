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
package gomeboy

import (
	"github.com/thelolagemann/gomeboy/internal/gameboy"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/ppu"
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
	bootROM  string
	headless bool
}

// Option configures an Emulator at construction time.
type Option func(*config)

// WithROM sets the ROM to load when the Emulator is created. The emulator is
// ready to step immediately after New returns.
func WithROM(path string) Option {
	return func(c *config) { c.romPath = path }
}

// WithBootROM sets the boot ROM (a .gbr file) to use instead of the built-in
// hardware-level-emulated boot process.
func WithBootROM(path string) Option {
	return func(c *config) { c.bootROM = path }
}

// Headless disables APU sample accumulation. The core is already headless
// (no GUI, no audio device); this option only prevents the APU from growing
// its sample buffer, so long-running headless emulation does not leak memory.
func Headless() Option {
	return func(c *config) { c.headless = true }
}

// New creates a new Emulator. If a ROM was supplied via WithROM, it is loaded
// and the emulator is ready to step. Otherwise, call LoadROM before stepping.
func New(opts ...Option) (*Emulator, error) {
	cfg := &config{}
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

	e := &Emulator{gb: gameboy.NewGameBoy(gbOpts...)}

	if cfg.romPath != "" {
		if err := e.LoadROM(cfg.romPath); err != nil {
			return nil, err
		}
	}

	if cfg.headless {
		e.gb.APU.SetHeadless(true)
	}

	return e, nil
}

// LoadROM loads a ROM from disk and (re)initializes the emulator.
func (e *Emulator) LoadROM(path string) error {
	return e.gb.LoadROM(path)
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
	for i := 0; i < n; i++ {
		e.gb.Step()
	}
}

// Read8 reads a single byte from the emulator's 16-bit address space.
func (e *Emulator) Read8(addr uint16) byte {
	return e.gb.Bus.Read(addr)
}

// Read reads length bytes starting at addr from the emulator's address space.
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

// Close flushes any pending battery-backed save data to disk and releases
// resources held by the emulator.
func (e *Emulator) Close() error {
	return e.gb.Save()
}

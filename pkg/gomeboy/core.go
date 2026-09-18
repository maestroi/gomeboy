package gomeboy

import (
	"fmt"
	"unsafe"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/ppu"
)

const gameBoyAddressSpaceSize uint64 = 1 << 16

// emulationCore is the system-independent execution boundary used by Emulator.
//
// System-specific adapters may expose additional capabilities elsewhere, but
// the lifecycle, framebuffer, memory-inspection, and state operations here are
// deliberately free of Game Boy-specific public types so another core can
// implement them without importing GB/GBC internals.
type emulationCore interface {
	CoreID() string
	LoadROM(string) error
	LoadROMBytes([]byte, string) error
	StepFrame()
	StepFrames(int)
	FrameCount() uint64
	Cycle() uint64
	Frame() Frame

	AddressSpaceSize() uint64
	Read8At(uint32) (byte, error)
	ReadIntoAt(uint32, []byte) error
	Peek8At(uint32) (byte, error)
	PeekIntoAt(uint32, []byte) error

	Reset() error
	SaveState() ([]byte, error)
	LoadState([]byte) error
	QuickSave() error
	QuickLoad() error
	Close() error

	NewCheckpoint() any
	CheckpointInto(any)
	RestoreCheckpoint(any) error
}

// newEmulatorWithCore centralizes construction around the core abstraction.
// Concrete-system constructors may attach optional system-specific capabilities
// after creating the Emulator.
func newEmulatorWithCore(core emulationCore) *Emulator {
	return &Emulator{core: core}
}

type gameBoyCore struct {
	gb *gameboy.GameBoy
}

var _ emulationCore = (*gameBoyCore)(nil)

func (c *gameBoyCore) CoreID() string { return "gb" }

func (c *gameBoyCore) LoadROM(path string) error {
	return c.gb.LoadROM(path)
}

func (c *gameBoyCore) LoadROMBytes(rom []byte, name string) error {
	return c.gb.LoadROMBytes(rom, name)
}

func (c *gameBoyCore) StepFrame() {
	c.gb.Step()
}

func (c *gameBoyCore) StepFrames(n int) {
	c.gb.StepFrames(n)
}

func (c *gameBoyCore) FrameCount() uint64 {
	return c.gb.FrameCount()
}

func (c *gameBoyCore) Cycle() uint64 {
	return c.gb.Cycle()
}

func (c *gameBoyCore) Frame() Frame {
	fb := c.gb.FrameBuffer()
	return Frame{
		Width:  ppu.ScreenWidth,
		Height: ppu.ScreenHeight,
		RGB:    unsafe.Slice(&(*fb)[0][0][0], ppu.ScreenWidth*ppu.ScreenHeight*3),
	}
}

func (c *gameBoyCore) AddressSpaceSize() uint64 {
	return gameBoyAddressSpaceSize
}

func (c *gameBoyCore) Read8At(addr uint32) (byte, error) {
	if err := c.validateRange(addr, 1); err != nil {
		return 0, err
	}
	return c.gb.Bus.Read(uint16(addr)), nil
}

func (c *gameBoyCore) ReadIntoAt(addr uint32, dst []byte) error {
	if err := c.validateRange(addr, len(dst)); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = c.gb.Bus.Read(uint16(addr + uint32(i)))
	}
	return nil
}

func (c *gameBoyCore) Peek8At(addr uint32) (byte, error) {
	if err := c.validateRange(addr, 1); err != nil {
		return 0, err
	}
	return c.gb.Bus.Get(uint16(addr)), nil
}

func (c *gameBoyCore) PeekIntoAt(addr uint32, dst []byte) error {
	if err := c.validateRange(addr, len(dst)); err != nil {
		return err
	}
	if len(dst) == 0 {
		return nil
	}

	start := uint16(addr)
	end := uint64(addr) + uint64(len(dst))
	if end < gameBoyAddressSpaceSize {
		c.gb.Bus.CopyFrom(start, uint16(end), dst)
		return nil
	}
	if end == gameBoyAddressSpaceSize {
		last := len(dst) - 1
		if last > 0 {
			c.gb.Bus.CopyFrom(start, 0xffff, dst[:last])
		}
		dst[last] = c.gb.Bus.Get(0xffff)
		return nil
	}

	return fmt.Errorf("gomeboy: GB memory range 0x%X+%d exceeds 0x%X-byte address space", addr, len(dst), gameBoyAddressSpaceSize)
}

func (c *gameBoyCore) validateRange(addr uint32, length int) error {
	if length < 0 {
		return fmt.Errorf("gomeboy: negative memory length %d", length)
	}
	end := uint64(addr) + uint64(length)
	if uint64(addr) > gameBoyAddressSpaceSize || end > gameBoyAddressSpaceSize {
		return fmt.Errorf("gomeboy: GB memory range 0x%X+%d exceeds 0x%X-byte address space", addr, length, gameBoyAddressSpaceSize)
	}
	return nil
}

func (c *gameBoyCore) Reset() error {
	return c.gb.Reset()
}

func (c *gameBoyCore) SaveState() ([]byte, error) {
	return c.gb.SaveState()
}

func (c *gameBoyCore) LoadState(data []byte) error {
	return c.gb.LoadState(data)
}

func (c *gameBoyCore) QuickSave() error {
	return c.gb.QuickSave()
}

func (c *gameBoyCore) QuickLoad() error {
	return c.gb.QuickLoad()
}

func (c *gameBoyCore) Close() error {
	return c.gb.Save()
}

func (c *gameBoyCore) NewCheckpoint() any {
	return &gameboy.State{}
}

func (c *gameBoyCore) CheckpointInto(dst any) {
	state, ok := dst.(*gameboy.State)
	if !ok {
		panic(fmt.Sprintf("gomeboy: invalid checkpoint storage %T for GB core", dst))
	}
	c.gb.CheckpointInto(state)
}

func (c *gameBoyCore) RestoreCheckpoint(src any) error {
	state, ok := src.(*gameboy.State)
	if !ok {
		return fmt.Errorf("gomeboy: invalid checkpoint storage %T for GB core", src)
	}
	c.gb.Restore(*state)
	return nil
}

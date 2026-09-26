package gomeboy

import (
	"errors"
	"fmt"
	"path/filepath"

	gbaaudio "github.com/maestroi/gomeboy/internal/gba/audio"
	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/keypad"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
	"github.com/maestroi/gomeboy/internal/gba/system"
	"github.com/maestroi/gomeboy/pkg/utils"
)

const gbaAddressSpaceSize uint64 = 1 << 32

var errGBAStateUnsupported = errors.New("gomeboy: GBA save states are not implemented yet")

type gbaCore struct {
	machine   *system.Machine
	bios      []byte
	rom       []byte
	name      string
	headless  bool
	muted     bool
	lastError error
}

var _ emulationCore = (*gbaCore)(nil)

func newGBACore(bios []byte) *gbaCore {
	return &gbaCore{bios: append([]byte(nil), bios...)}
}

func (c *gbaCore) CoreID() string { return "gba" }

func (c *gbaCore) LoadROM(path string) error {
	rom, err := utils.LoadFile(path)
	if err != nil {
		return err
	}
	return c.LoadROMBytes(rom, stringsTrimExt(filepath.Base(path)))
}

func stringsTrimExt(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

func (c *gbaCore) LoadROMBytes(rom []byte, name string) error {
	if len(rom) == 0 {
		return errors.New("gomeboy: empty GBA ROM")
	}
	c.rom = append(c.rom[:0], rom...)
	c.name = name
	c.resetMachine()
	return nil
}

func (c *gbaCore) resetMachine() {
	c.machine = system.New(c.bios, c.rom)
	c.lastError = nil

	if len(c.bios) == 0 {
		// Match the useful post-reset stack layout used by GBA firmware/core
		// implementations, then enter the cartridge directly when no BIOS was
		// supplied.
		_ = c.machine.CPU.SetMode(cpu.ModeIRQ)
		c.machine.CPU.WriteRegister(13, 0x03007fa0)
		_ = c.machine.CPU.SetMode(cpu.ModeSupervisor)
		c.machine.CPU.WriteRegister(13, 0x03007fe0)
		_ = c.machine.CPU.SetMode(cpu.ModeSystem)
		c.machine.CPU.WriteRegister(13, 0x03007f00)
		c.machine.CPU.SetPC(bus.ROM0Start)
	}

	c.machine.Audio.SetHeadless(c.headless)
	c.machine.Audio.SetMute(c.muted)
}

func (c *gbaCore) StepFrame() {
	if c.machine == nil || c.lastError != nil {
		return
	}
	start := c.machine.PPU.FrameCount()
	for c.machine.PPU.FrameCount() == start {
		result, err := c.machine.Step()
		if err != nil {
			c.lastError = err
			return
		}
		if result.Stopped && result.ElapsedCycles == 0 {
			// STOP is released by asynchronous keypad/SIO/Game Pak input.
			return
		}
	}
}

func (c *gbaCore) StepFrames(n int) {
	for i := 0; i < n; i++ {
		c.StepFrame()
		if c.lastError != nil || (c.machine != nil && c.machine.Stopped()) {
			return
		}
	}
}

func (c *gbaCore) FrameCount() uint64 {
	if c.machine == nil {
		return 0
	}
	return c.machine.PPU.FrameCount()
}

func (c *gbaCore) Cycle() uint64 {
	if c.machine == nil {
		return 0
	}
	return c.machine.Cycle()
}

func (c *gbaCore) Frame() Frame {
	if c.machine == nil {
		return Frame{Width: ppu.ScreenWidth, Height: ppu.ScreenHeight}
	}
	return Frame{
		Width:  ppu.ScreenWidth,
		Height: ppu.ScreenHeight,
		RGB:    c.machine.PPU.FrameBuffer(),
	}
}

func (c *gbaCore) AddressSpaceSize() uint64 { return gbaAddressSpaceSize }

func (c *gbaCore) validateRange(addr uint32, length int) error {
	if length < 0 {
		return fmt.Errorf("gomeboy: negative memory length %d", length)
	}
	end := uint64(addr) + uint64(length)
	if end > gbaAddressSpaceSize {
		return fmt.Errorf("gomeboy: GBA memory range 0x%X+%d exceeds 32-bit address space", addr, length)
	}
	return nil
}

func (c *gbaCore) Read8At(addr uint32) (byte, error) {
	if c.machine == nil {
		return 0, errors.New("gomeboy: GBA core has no ROM loaded")
	}
	value, cycles := c.machine.Bus.Read8(addr, bus.Access{})
	c.machine.Advance(cycles)
	return value, nil
}

func (c *gbaCore) ReadIntoAt(addr uint32, dst []byte) error {
	if err := c.validateRange(addr, len(dst)); err != nil {
		return err
	}
	for i := range dst {
		value, err := c.Read8At(addr + uint32(i))
		if err != nil {
			return err
		}
		dst[i] = value
	}
	return nil
}

func (c *gbaCore) Peek8At(addr uint32) (byte, error) {
	if c.machine == nil {
		return 0, errors.New("gomeboy: GBA core has no ROM loaded")
	}
	return c.machine.Bus.Peek8(addr), nil
}

func (c *gbaCore) PeekIntoAt(addr uint32, dst []byte) error {
	if c.machine == nil {
		return errors.New("gomeboy: GBA core has no ROM loaded")
	}
	if err := c.validateRange(addr, len(dst)); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = c.machine.Bus.Peek8(addr + uint32(i))
	}
	return nil
}

func (c *gbaCore) Reset() error {
	if len(c.rom) == 0 {
		return errors.New("gomeboy: cannot reset GBA core: no ROM loaded")
	}
	c.resetMachine()
	return nil
}

func (c *gbaCore) SaveState() ([]byte, error) { return nil, errGBAStateUnsupported }
func (c *gbaCore) LoadState([]byte) error      { return errGBAStateUnsupported }
func (c *gbaCore) QuickSave() error            { return errGBAStateUnsupported }
func (c *gbaCore) QuickLoad() error            { return errGBAStateUnsupported }
func (c *gbaCore) Close() error                { return nil }

func (c *gbaCore) NewCheckpoint() any { return &struct{}{} }
func (c *gbaCore) CheckpointInto(any) {}
func (c *gbaCore) RestoreCheckpoint(any) error { return errGBAStateUnsupported }

func (c *gbaCore) Press(button Button) {
	if c.machine == nil {
		return
	}
	if mapped, ok := gbaButton(button); ok {
		c.machine.Keypad.Press(mapped)
	}
}

func (c *gbaCore) Release(button Button) {
	if c.machine == nil {
		return
	}
	if mapped, ok := gbaButton(button); ok {
		c.machine.Keypad.Release(mapped)
	}
}

func gbaButton(button Button) (keypad.Button, bool) {
	switch button {
	case ButtonA:
		return keypad.ButtonA, true
	case ButtonB:
		return keypad.ButtonB, true
	case ButtonStart:
		return keypad.ButtonStart, true
	case ButtonSelect:
		return keypad.ButtonSelect, true
	case ButtonUp:
		return keypad.ButtonUp, true
	case ButtonDown:
		return keypad.ButtonDown, true
	case ButtonLeft:
		return keypad.ButtonLeft, true
	case ButtonRight:
		return keypad.ButtonRight, true
	case ButtonL:
		return keypad.ButtonL, true
	case ButtonR:
		return keypad.ButtonR, true
	default:
		return 0, false
	}
}

func (c *gbaCore) Samples() ([]float32, uint32) {
	if c.machine == nil {
		return nil, 0
	}
	return c.machine.Audio.Samples()
}

func (c *gbaCore) SetHeadless(headless bool) {
	c.headless = headless
	if c.machine != nil {
		c.machine.Audio.SetHeadless(headless)
	}
}

func (c *gbaCore) ToggleMute() bool {
	c.muted = !c.muted
	if c.machine != nil {
		c.machine.Audio.SetMute(c.muted)
	}
	return c.muted
}

func (c *gbaCore) AudioSampleRate() uint64 { return gbaaudio.SampleRate }

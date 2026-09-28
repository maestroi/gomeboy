package gomeboy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gbaaudio "github.com/maestroi/gomeboy/internal/gba/audio"
	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
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
	saveDir    string
	saves      bool
	saveLoaded bool
}

var _ emulationCore = (*gbaCore)(nil)

func newGBACore(bios []byte, saveDir string, saves bool) *gbaCore {
	return &gbaCore{
		bios:    append([]byte(nil), bios...),
		saveDir: saveDir,
		saves:   saves,
	}
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
	if len(c.bios) != 0 && len(c.bios) != bus.BIOSSize {
		return fmt.Errorf("gomeboy: GBA BIOS must be exactly 16 KiB, got %d bytes", len(c.bios))
	}
	if len(rom) == 0 {
		return errors.New("gomeboy: empty GBA ROM")
	}
	if err := c.flushSave(); err != nil {
		return err
	}
	c.rom = append(c.rom[:0], rom...)
	c.name = gbaSaveName(name)
	c.saveLoaded = false
	c.resetMachine()
	if err := c.loadSave(); err != nil {
		return err
	}
	c.saveLoaded = true
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
		_ = c.machine.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem))
		c.machine.CPU.SetPC(bus.ROM0Start)
	}

	c.machine.Audio.SetHeadless(c.headless)
	c.machine.Audio.SetMute(c.muted)
}

func (c *gbaCore) StepFrame() {
	if c.machine == nil || c.lastError != nil {
		return
	}
	if err := c.machine.RunFrame(); err != nil {
		c.lastError = err
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

	var liveSave []byte
	if device := c.persistentDevice(); device != nil {
		liveSave = device.SaveData()
	}
	var liveRTC []byte
	if rtc := c.cartridgeRTC(); rtc != nil {
		liveRTC = rtc.SaveFooter()
	}

	c.resetMachine()
	if liveSave != nil {
		device := c.persistentDevice()
		if device == nil {
			return errors.New("gomeboy: GBA cartridge save hardware disappeared across reset")
		}
		if err := device.LoadSaveData(liveSave); err != nil {
			return fmt.Errorf("gomeboy: restore GBA cartridge save after reset: %w", err)
		}
	}
	if liveRTC != nil {
		rtc := c.cartridgeRTC()
		if rtc == nil {
			return errors.New("gomeboy: GBA cartridge RTC disappeared across reset")
		}
		if err := rtc.LoadFooter(liveRTC); err != nil {
			return fmt.Errorf("gomeboy: restore GBA cartridge RTC after reset: %w", err)
		}
	}
	return nil
}

func (c *gbaCore) SaveState() ([]byte, error) { return nil, errGBAStateUnsupported }
func (c *gbaCore) LoadState([]byte) error      { return errGBAStateUnsupported }
func (c *gbaCore) QuickSave() error            { return errGBAStateUnsupported }
func (c *gbaCore) QuickLoad() error            { return errGBAStateUnsupported }
func (c *gbaCore) Close() error                { return c.flushSave() }

func gbaSaveName(name string) string {
	if name == "" {
		return ""
	}
	return stringsTrimExt(filepath.Base(name))
}

func (c *gbaCore) persistentDevice() cartridge.PersistentDevice {
	if c.machine == nil {
		return nil
	}
	return c.machine.Cartridge.PersistentDevice()
}

func (c *gbaCore) cartridgeRTC() *cartridge.RTC {
	if c.machine == nil || c.machine.Cartridge.GPIO == nil {
		return nil
	}
	return c.machine.Cartridge.GPIO.RTC()
}

func (c *gbaCore) saveFilePath() string {
	if !c.saves || c.name == "" {
		return ""
	}
	if c.persistentDevice() == nil && c.cartridgeRTC() == nil {
		return ""
	}
	return filepath.Join(c.saveDir, c.name+".sav")
}

func (c *gbaCore) loadSave() error {
	path := c.saveFilePath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("gomeboy: read GBA save %s: %w", path, err)
	}

	device := c.persistentDevice()
	baseSize := 0
	if device != nil {
		baseSize = device.SaveSize()
	}

	var footer []byte
	switch {
	case len(data) == baseSize:
		// Raw SRAM/Flash/EEPROM save without an RTC trailer.
	case c.cartridgeRTC() != nil && len(data) == baseSize+cartridge.RTCFooterSize:
		footer = data[baseSize:]
		data = data[:baseSize]
	default:
		return fmt.Errorf("gomeboy: load GBA save %s: size %d bytes does not match selected cartridge payload %d", path, len(data), baseSize)
	}

	if device != nil {
		if err := device.LoadSaveData(data); err != nil {
			return fmt.Errorf("gomeboy: load GBA save %s: %w", path, err)
		}
	}
	if footer != nil {
		if err := c.cartridgeRTC().LoadFooter(footer); err != nil {
			return fmt.Errorf("gomeboy: load GBA RTC %s: %w", path, err)
		}
	}
	return nil
}

func (c *gbaCore) flushSave() error {
	if !c.saveLoaded {
		return nil
	}
	path := c.saveFilePath()
	if path == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("gomeboy: create GBA save directory %s: %w", dir, err)
		}
	}

	var data []byte
	if device := c.persistentDevice(); device != nil {
		data = device.SaveData()
	}
	if rtc := c.cartridgeRTC(); rtc != nil {
		data = append(data, rtc.SaveFooter()...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("gomeboy: write GBA save %s: %w", path, err)
	}
	return nil
}

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

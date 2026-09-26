package gomeboy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gbaLoopROM() []byte {
	// ARM B . -- enough to let the integrated scheduler/PPU advance forever.
	return []byte{0xfe, 0xff, 0xff, 0xea}
}

func TestGBAInMemoryCoreLaunchAndFrame(t *testing.T) {
	e, err := New()
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatalf("New GBA: %v", err)
	}
	defer e.Close()

	if got := e.Model(); got != ModelAGB {
		t.Fatalf("Model = %s, want AGB", got)
	}
	if got := e.AddressSpaceSize(); got != uint64(1)<<32 {
		t.Fatalf("AddressSpaceSize = %#x, want 2^32", got)
	}
	if width, height := e.FrameSize(); width != 240 || height != 160 {
		t.Fatalf("FrameSize = %dx%d, want 240x160", width, height)
	}

	core, ok := e.core.(*gbaCore)
	if !ok {
		t.Fatalf("core = %T, want *gbaCore", e.core)
	}
	if got := core.machine.CPU.PC(); got != 0x08000000 {
		t.Fatalf("BIOS-less PC = %#08x, want cartridge entry", got)
	}
	if got := core.machine.CPU.ReadRegister(13); got != 0x03007f00 {
		t.Fatalf("BIOS-less SP = %#08x, want 0x03007f00", got)
	}

	e.StepFrame()
	if got := e.FrameCount(); got != 1 {
		t.Fatalf("FrameCount = %d, want 1", got)
	}
	frame := e.Frame()
	if len(frame.RGB) != 240*160*3 {
		t.Fatalf("frame bytes = %d, want %d", len(frame.RGB), 240*160*3)
	}
	if _, count := e.Samples(); count == 0 {
		t.Fatal("interactive GBA core produced no host audio samples")
	}
}

func TestGBAHeadlessKeepsHardwareButDropsAudioBuffer(t *testing.T) {
	e, err := New(Headless())
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StepFrame()
	if e.FrameCount() != 1 {
		t.Fatalf("headless FrameCount = %d, want 1", e.FrameCount())
	}
	if _, count := e.Samples(); count != 0 {
		t.Fatalf("headless Samples count = %d, want 0", count)
	}
}

func TestGBAKeypadMapsAllFrontendButtons(t *testing.T) {
	e, err := New(Headless())
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	const keyInput = uint32(0x04000130)
	beforeLo, _ := e.Peek8At(keyInput)
	beforeHi, _ := e.Peek8At(keyInput + 1)
	if beforeLo != 0xff || beforeHi&0x03 != 0x03 {
		t.Fatalf("initial KEYINPUT bytes = %02x %02x", beforeLo, beforeHi)
	}

	e.Press(ButtonA)
	e.Press(ButtonL)
	lo, _ := e.Peek8At(keyInput)
	hi, _ := e.Peek8At(keyInput + 1)
	if lo&0x01 != 0 {
		t.Fatalf("A press not reflected in KEYINPUT: %02x", lo)
	}
	if hi&0x02 != 0 {
		t.Fatalf("L press not reflected in KEYINPUT: %02x", hi)
	}

	e.Release(ButtonA)
	e.Release(ButtonL)
	lo, _ = e.Peek8At(keyInput)
	hi, _ = e.Peek8At(keyInput + 1)
	if lo&0x01 == 0 || hi&0x02 == 0 {
		t.Fatalf("release not reflected in KEYINPUT: %02x %02x", lo, hi)
	}
}

func TestLoadROMSelectsGBAByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smoke.gba")
	if err := os.WriteFile(path, gbaLoopROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.core.CoreID() != "gb" {
		t.Fatalf("initial core = %q, want gb", e.core.CoreID())
	}
	if err := e.LoadROM(path); err != nil {
		t.Fatalf("LoadROM(.gba): %v", err)
	}
	if e.core.CoreID() != "gba" || e.Model() != ModelAGB {
		t.Fatalf("after .gba load core=%q model=%s", e.core.CoreID(), e.Model())
	}
}

func TestGBAStateOperationsReportUnsupported(t *testing.T) {
	e, err := New(Headless())
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if _, err := e.SaveState(); err == nil || !strings.Contains(err.Error(), "GBA save states") {
		t.Fatalf("SaveState error = %v", err)
	}
	if err := e.QuickSave(); err == nil || !strings.Contains(err.Error(), "GBA save states") {
		t.Fatalf("QuickSave error = %v", err)
	}
}

func TestGBARejectsWrongSizedBIOS(t *testing.T) {
	dir := t.TempDir()
	bios := filepath.Join(dir, "gba.bin")
	if err := os.WriteFile(bios, make([]byte, 256), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := New(WithBootROM(bios))
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if e != nil {
		defer e.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "16 KiB") {
		t.Fatalf("New with bad GBA BIOS error = %v", err)
	}
}

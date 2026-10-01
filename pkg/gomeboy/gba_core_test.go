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

func TestGBAStateRoundTripQuickSaveAndCheckpoint(t *testing.T) {
	dir := t.TempDir()
	e, err := New(Headless(), WithSaveDir(dir))
	if err == nil {
		err = e.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	core := e.core.(*gbaCore)
	core.machine.Bus.EWRAM()[0x123] = 0x5a
	e.Press(ButtonA)
	e.StepFrame()

	state, err := e.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if len(state) <= gbaStateHeader {
		t.Fatalf("SaveState returned %d bytes", len(state))
	}
	wantCycle := e.Cycle()
	wantFrame := e.FrameCount()
	wantPC := core.machine.CPU.PC()
	wantKeys := core.machine.Keypad.PressedMask()

	var checkpoint Checkpoint
	e.CheckpointInto(&checkpoint)

	e.StepFrame()
	nextCycle := e.Cycle()
	nextPC := core.machine.CPU.PC()
	nextFrame := append([]byte(nil), e.Frame().RGB...)

	core.machine.Bus.EWRAM()[0x123] = 0xa5
	e.Release(ButtonA)
	e.StepFrame()

	if err := e.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := e.Cycle(); got != wantCycle {
		t.Fatalf("Cycle after LoadState = %d, want %d", got, wantCycle)
	}
	if got := e.FrameCount(); got != wantFrame {
		t.Fatalf("FrameCount after LoadState = %d, want %d", got, wantFrame)
	}
	if got := core.machine.CPU.PC(); got != wantPC {
		t.Fatalf("PC after LoadState = %#x, want %#x", got, wantPC)
	}
	if got := core.machine.Bus.EWRAM()[0x123]; got != 0x5a {
		t.Fatalf("EWRAM after LoadState = %#02x, want 0x5a", got)
	}
	if got := core.machine.Keypad.PressedMask(); got != wantKeys {
		t.Fatalf("keypad after LoadState = %#04x, want %#04x", got, wantKeys)
	}

	e.StepFrame()
	if got := e.Cycle(); got != nextCycle {
		t.Fatalf("deterministic continuation cycle = %d, want %d", got, nextCycle)
	}
	if got := core.machine.CPU.PC(); got != nextPC {
		t.Fatalf("deterministic continuation PC = %#x, want %#x", got, nextPC)
	}
	if got := e.Frame().RGB; string(got) != string(nextFrame) {
		t.Fatal("framebuffer diverged after SaveState/LoadState continuation")
	}

	if err := e.RestoreCheckpoint(&checkpoint); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	if got := e.Cycle(); got != wantCycle {
		t.Fatalf("Cycle after RestoreCheckpoint = %d, want %d", got, wantCycle)
	}

	if err := e.QuickSave(); err != nil {
		t.Fatalf("QuickSave: %v", err)
	}
	core.machine.Bus.EWRAM()[0x123] = 0xcc
	e.StepFrame()
	if err := e.QuickLoad(); err != nil {
		t.Fatalf("QuickLoad: %v", err)
	}
	if got := core.machine.Bus.EWRAM()[0x123]; got != 0x5a {
		t.Fatalf("EWRAM after QuickLoad = %#02x, want 0x5a", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "loop.state")); err != nil {
		t.Fatalf("quick-save state file: %v", err)
	}
}

func TestGBAStateRejectsWrongROMAndPreservesHeadlessPolicy(t *testing.T) {
	source, err := New()
	if err == nil {
		err = source.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	source.StepFrame()
	state, err := source.SaveState()
	if err != nil {
		t.Fatal(err)
	}

	headless, err := New(Headless())
	if err == nil {
		err = headless.LoadROMBytes(gbaLoopROM(), "loop.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer headless.Close()
	if err := headless.LoadState(state); err != nil {
		t.Fatalf("headless LoadState: %v", err)
	}
	headless.StepFrame()
	if _, count := headless.Samples(); count != 0 {
		t.Fatalf("restoring audible state enabled headless audio buffering: %d samples", count)
	}

	otherROM := append(append([]byte(nil), gbaLoopROM()...), 0)
	other, err := New(Headless())
	if err == nil {
		err = other.LoadROMBytes(otherROM, "other.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.LoadState(state); err == nil || !strings.Contains(err.Error(), "ROM does not match") {
		t.Fatalf("wrong-ROM LoadState error = %v", err)
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

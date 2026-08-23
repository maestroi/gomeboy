package gomeboy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// testROM is a simple, deterministic ROM that renders a known image.
var testROM = filepath.Join("..", "..", "tests", "roms", "little-things-gb", "firstwhite.gb")

func newTestEmulator(t *testing.T, opts ...Option) *Emulator {
	t.Helper()
	e, err := New(append([]Option{WithROM(testROM)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

// snapshot copies the current frame so it survives the next step.
func (e *Emulator) snapshot() []byte {
	f := e.Frame()
	return append([]byte(nil), f.RGB...)
}

func TestNewWithROM(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrame()
	f := e.Frame()
	if f.Width != 160 || f.Height != 144 {
		t.Errorf("unexpected frame dimensions: %dx%d", f.Width, f.Height)
	}
	if len(f.RGB) != 160*144*3 {
		t.Errorf("unexpected RGB length: %d", len(f.RGB))
	}
	// firstwhite renders a non-black screen
	var nonZero int
	for _, b := range f.RGB {
		if b != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("expected a non-black frame")
	}
}

func TestDeterministicFrames(t *testing.T) {
	a := newTestEmulator(t)
	defer a.Close()
	b := newTestEmulator(t)
	defer b.Close()

	for i := 0; i < 30; i++ {
		a.StepFrame()
		b.StepFrame()
	}
	if !bytes.Equal(a.snapshot(), b.snapshot()) {
		t.Error("two emulators with the same ROM and no input produced different frames")
	}
}

func TestRead(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrame()

	// Read and Read8 must agree on every byte of a region.
	addr := uint16(0xC000)
	n := 256
	block := e.Read(addr, n)
	if len(block) != n {
		t.Fatalf("Read returned %d bytes, want %d", len(block), n)
	}
	for i := 0; i < n; i++ {
		if block[i] != e.Read8(addr+uint16(i)) {
			t.Fatalf("Read/Read8 disagree at offset %d: %x vs %x", i, block[i], e.Read8(addr+uint16(i)))
		}
	}
}

func TestInput(t *testing.T) {
	// A game that responds to input will produce a different frame when a
	// button is held. firstwhite does not, so we only assert that pressing and
	// releasing every button is safe and does not panic.
	e := newTestEmulator(t)
	defer e.Close()

	for b := Button(0); b <= ButtonRight; b++ {
		e.Press(b)
		e.StepFrame()
		e.Release(b)
		e.StepFrame()
	}
}

func TestReset(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	const frames = 25
	for i := 0; i < frames; i++ {
		e.StepFrame()
	}
	before := e.snapshot()

	if err := e.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for i := 0; i < frames; i++ {
		e.StepFrame()
	}
	after := e.snapshot()

	if !bytes.Equal(before, after) {
		t.Error("frame after reset+steps does not match the original frame")
	}
}

func TestSaveState(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	const warmup, branch = 20, 15
	for i := 0; i < warmup; i++ {
		e.StepFrame()
	}
	state, err := e.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Path 1: continue from the saved point.
	for i := 0; i < branch; i++ {
		e.StepFrame()
	}
	path1 := e.snapshot()

	// Restore and replay the same number of frames.
	if err := e.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	for i := 0; i < branch; i++ {
		e.StepFrame()
	}
	path2 := e.snapshot()

	if !bytes.Equal(path1, path2) {
		t.Error("frame after restore+replay does not match the original continuation")
	}
}

func TestQuickSaveLoad(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	defer os.Remove("firstwhite.state")

	const warmup, branch = 20, 15
	for i := 0; i < warmup; i++ {
		e.StepFrame()
	}
	if err := e.QuickSave(); err != nil {
		t.Fatalf("QuickSave: %v", err)
	}

	// Path 1: continue from the saved point.
	for i := 0; i < branch; i++ {
		e.StepFrame()
	}
	path1 := e.snapshot()

	// Restore and replay the same number of frames.
	if err := e.QuickLoad(); err != nil {
		t.Fatalf("QuickLoad: %v", err)
	}
	for i := 0; i < branch; i++ {
		e.StepFrame()
	}
	path2 := e.snapshot()

	if !bytes.Equal(path1, path2) {
		t.Error("frame after quick load+replay does not match the original continuation")
	}
}

func TestIndependentInstances(t *testing.T) {
	a := newTestEmulator(t)
	defer a.Close()
	b := newTestEmulator(t)
	defer b.Close()

	// Drive input on A only; B must be unaffected.
	for i := 0; i < 20; i++ {
		a.Press(ButtonA)
		a.StepFrame()
		b.StepFrame()
	}
	a.Release(ButtonA)

	// B has received no input, so it must match a fresh, un-driven emulator.
	c := newTestEmulator(t)
	defer c.Close()
	for i := 0; i < 20; i++ {
		c.StepFrame()
	}
	if !bytes.Equal(b.snapshot(), c.snapshot()) {
		t.Error("input on one instance affected another instance")
	}
}

func TestHeadlessDoesNotLeak(t *testing.T) {
	e, err := New(WithROM(testROM), Headless())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	// Step many frames; in headless mode the APU must not accumulate samples.
	for i := 0; i < 500; i++ {
		e.StepFrame()
	}
	// If the buffer were growing unboundedly this would be a large allocation;
	// we only assert the emulator still produces frames without error.
	if len(e.Frame().RGB) != 160*144*3 {
		t.Error("headless emulator produced a malformed frame")
	}
}

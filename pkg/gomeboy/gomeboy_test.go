package gomeboy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/internal/serial/accessories"
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

// absTestROM returns testROM as an absolute path so tests that change the
// working directory can still find it.
func absTestROM(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(testROM)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", testROM, err)
	}
	return p
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

// TestWithModel verifies that every documented public model maps to the
// intended internal hardware model: the effective model reported by Model()
// must round-trip the value passed to WithModel.
func TestWithModel(t *testing.T) {
	models := []Model{ModelDMG0, ModelDMG, ModelCGB0, ModelCGB, ModelMGB, ModelSGB, ModelSGB2, ModelAGB}
	for _, m := range models {
		t.Run(string(m), func(t *testing.T) {
			e, err := New(WithROM(testROM), WithModel(m))
			if err != nil {
				t.Fatalf("New(WithModel(%s)): %v", m, err)
			}
			defer e.Close()
			if got := e.Model(); got != m {
				t.Errorf("Model() = %s, want %s", got, m)
			}
		})
	}
}

// TestWithModelAuto verifies that auto (the default, and the explicit
// ModelAuto) keeps the model inferred from the cartridge. firstwhite is a
// DMG cartridge, so the inference must resolve to ModelDMG.
func TestWithModelAuto(t *testing.T) {
	e, err := New(WithROM(testROM))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()
	if got := e.Model(); got != ModelDMG {
		t.Errorf("auto Model() = %s, want %s", got, ModelDMG)
	}

	e2, err := New(WithROM(testROM), WithModel(ModelAuto))
	if err != nil {
		t.Fatalf("New(WithModel(auto)): %v", err)
	}
	defer e2.Close()
	if got := e2.Model(); got != ModelDMG {
		t.Errorf("explicit auto Model() = %s, want %s", got, ModelDMG)
	}
}

// TestWithModelUnknown verifies that unknown model values are rejected by New
// with an actionable error that names the offending value.
func TestWithModelUnknown(t *testing.T) {
	for _, m := range []Model{"", "nintendo", "DMG1", "cgb", "AUTO"} {
		name := string(m)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			e, err := New(WithROM(testROM), WithModel(m))
			if err == nil {
				t.Fatalf("New(WithModel(%q)): expected an error, got nil", m)
			}
			if e != nil {
				t.Error("expected a nil Emulator")
			}
			if !strings.Contains(err.Error(), "unknown model") {
				t.Errorf("error %q does not name the problem", err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", m)) {
				t.Errorf("error %q does not quote the offending value", err)
			}
		})
	}
}

// TestWithPrinter verifies that WithPrinter attaches the printer accessory.
func TestWithPrinter(t *testing.T) {
	e := newTestEmulator(t, WithPrinter())
	defer e.Close()

	if _, ok := e.gb.Serial.AttachedDevice.(*accessories.Printer); !ok {
		t.Errorf("AttachedDevice = %T, want *accessories.Printer", e.gb.Serial.AttachedDevice)
	}
}

// TestNoPrinterByDefault verifies that no printer is attached unless
// WithPrinter is passed.
func TestNoPrinterByDefault(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	if _, ok := e.gb.Serial.AttachedDevice.(*accessories.Printer); ok {
		t.Error("a printer is attached without WithPrinter")
	}
}

// TestWithCheatsLoadsOnlyExplicitPath verifies that WithCheats loads exactly
// the path it is given: a cheats file with the conventional name sitting in
// the working directory must not be picked up.
func TestWithCheatsLoadsOnlyExplicitPath(t *testing.T) {
	rom := absTestROM(t)
	t.Chdir(t.TempDir())

	if err := os.WriteFile("firstwhite.cheats", []byte("# cwd cheat\n11223344\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	explicit := filepath.Join(t.TempDir(), "explicit.cheats")
	if err := os.WriteFile(explicit, []byte("# explicit cheat\n11234567\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	e, err := New(WithROM(rom), WithCheats(explicit))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if len(e.gb.Bus.LoadedCheats) != 1 {
		t.Fatalf("LoadedCheats = %d entries, want 1: %+v", len(e.gb.Bus.LoadedCheats), e.gb.Bus.LoadedCheats)
	}
	if got := e.gb.Bus.LoadedCheats[0].Name; got != "explicit cheat" {
		t.Errorf("cheat name = %q, want %q", got, "explicit cheat")
	}
	if len(e.gb.Bus.GameSharkCodes) != 1 {
		t.Errorf("GameSharkCodes = %d entries, want 1", len(e.gb.Bus.GameSharkCodes))
	}
}

// TestWithCheatsMalformed verifies that malformed cheats content is reported
// through the existing observable error behavior (logged, not returned): New
// succeeds and no cheats are loaded.
func TestWithCheatsMalformed(t *testing.T) {
	rom := absTestROM(t)
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "bad.cheats")
	// a line longer than bufio.MaxScanTokenSize makes the parser fail
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 70*1024), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	e, err := New(WithROM(rom), WithCheats(path))
	if err != nil {
		t.Fatalf("New: %v (malformed cheats must not fail New)", err)
	}
	defer e.Close()

	if len(e.gb.Bus.LoadedCheats) != 0 {
		t.Errorf("LoadedCheats = %d entries, want 0", len(e.gb.Bus.LoadedCheats))
	}
}

// TestWithCheatsUnreadable verifies that an unreadable (nonexistent) cheats
// path is reported through the existing observable error behavior: New
// succeeds and no cheats are loaded.
func TestWithCheatsUnreadable(t *testing.T) {
	rom := absTestROM(t)
	t.Chdir(t.TempDir())

	e, err := New(WithROM(rom), WithCheats(filepath.Join(t.TempDir(), "missing.cheats")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if len(e.gb.Bus.LoadedCheats) != 0 {
		t.Errorf("LoadedCheats = %d entries, want 0", len(e.gb.Bus.LoadedCheats))
	}
}

// TestNoDiskIOWithModelAndPrinter verifies that the non-persistence options
// introduce no disk I/O: without WithSaveDir or WithCheats, New, stepping,
// and Close leave the working directory empty.
func TestNoDiskIOWithModelAndPrinter(t *testing.T) {
	rom := absTestROM(t)
	t.Chdir(t.TempDir())

	e, err := New(WithROM(rom), WithModel(ModelCGB), WithPrinter())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.StepFrames(10)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if names := entryNames(t, "."); len(names) != 0 {
		t.Errorf("expected no files in the working directory, found %v", names)
	}
}

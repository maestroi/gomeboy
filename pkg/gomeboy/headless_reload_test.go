package gomeboy

import (
	"os"
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
)

// TestHeadlessSurvivesROMSwapAndStateLoad guards the leak where a headless
// emulator silently started buffering audio forever after LoadROMBytes (which
// rebuilds the APU) or after loading a state saved by a non-headless emulator.
func TestHeadlessSurvivesROMSwapAndStateLoad(t *testing.T) {
	sampling := func(e *Emulator) bool { return e.gb.Scheduler.Until(scheduler.APUSample) != 0 }

	e, err := New(WithROM(testROM), Headless())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()
	rom, err := os.ReadFile(testROM)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.LoadROMBytes(rom, "swap"); err != nil {
		t.Fatalf("LoadROMBytes: %v", err)
	}
	if sampling(e) {
		t.Fatal("LoadROMBytes turned headless audio sampling back on")
	}

	audible, err := New(WithROM(testROM))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer audible.Close()
	audible.StepFrames(10)
	st, err := audible.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := e.LoadState(st); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if sampling(e) {
		t.Fatal("loading an audible state turned headless audio sampling back on")
	}
}

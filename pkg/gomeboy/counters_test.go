package gomeboy

import "testing"

func TestFrameCountAdvances(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	if got := e.FrameCount(); got != 0 {
		t.Fatalf("fresh emulator FrameCount = %d, want 0", got)
	}
	e.StepFrames(10)
	if got := e.FrameCount(); got != 10 {
		t.Fatalf("FrameCount after StepFrames(10) = %d, want 10", got)
	}
	e.StepFrame()
	if got := e.FrameCount(); got != 11 {
		t.Fatalf("FrameCount after StepFrame = %d, want 11", got)
	}
}

func TestCycleAdvances(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	before := e.Cycle()
	e.StepFrame()
	after := e.Cycle()
	if after <= before {
		t.Errorf("Cycle did not advance: before=%d, after=%d", before, after)
	}
}

func TestResetZeroesFrameCount(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrames(25)
	if err := e.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := e.FrameCount(); got != 0 {
		t.Fatalf("FrameCount after Reset = %d, want 0", got)
	}
}

func TestFrameCountSurvivesSaveState(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrames(20)
	state, err := e.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	e.StepFrames(5)
	if got := e.FrameCount(); got != 25 {
		t.Fatalf("FrameCount after 25 frames = %d, want 25", got)
	}
	if err := e.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := e.FrameCount(); got != 20 {
		t.Fatalf("FrameCount after LoadState = %d, want 20", got)
	}
}

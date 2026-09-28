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


func TestExecutionEpochMarksLifecycleDiscontinuities(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	epoch := e.ExecutionEpoch()
	if epoch == 0 {
		t.Fatal("fresh loaded emulator ExecutionEpoch = 0, want non-zero")
	}

	e.StepFrames(3)
	if got := e.ExecutionEpoch(); got != epoch {
		t.Fatalf("ExecutionEpoch changed during ordinary stepping: got %d want %d", got, epoch)
	}

	state, err := e.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	e.StepFrames(2)
	if err := e.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	epoch++
	if got := e.ExecutionEpoch(); got != epoch {
		t.Fatalf("ExecutionEpoch after LoadState = %d, want %d", got, epoch)
	}

	var cp Checkpoint
	e.CheckpointInto(&cp)
	e.StepFrame()
	if err := e.RestoreCheckpoint(&cp); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	epoch++
	if got := e.ExecutionEpoch(); got != epoch {
		t.Fatalf("ExecutionEpoch after RestoreCheckpoint = %d, want %d", got, epoch)
	}

	if err := e.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	epoch++
	if got := e.ExecutionEpoch(); got != epoch {
		t.Fatalf("ExecutionEpoch after Reset = %d, want %d", got, epoch)
	}
}

func TestExecutionEpochChangesOnlyAfterSuccessfulRestore(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	before := e.ExecutionEpoch()
	if err := e.LoadState([]byte("not a state")); err == nil {
		t.Fatal("invalid LoadState unexpectedly succeeded")
	}
	if got := e.ExecutionEpoch(); got != before {
		t.Fatalf("ExecutionEpoch changed after failed LoadState: got %d want %d", got, before)
	}
}

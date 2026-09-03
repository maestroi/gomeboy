package gomeboy

import "testing"

func TestReusableCheckpointSteadyStateAllocations(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	var cp Checkpoint
	// First capture allocates the reusable variable-sized buffers. Steady-state
	// capture/restore should not allocate after that storage has been established.
	e.CheckpointInto(&cp)
	if err := e.RestoreCheckpoint(&cp); err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		e.CheckpointInto(&cp)
		if err := e.RestoreCheckpoint(&cp); err != nil {
			panic(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("steady-state checkpoint round trip allocated %.2f objects/run, want 0", allocs)
	}
}

func TestRestoreUninitializedCheckpointRejected(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	var cp Checkpoint
	if err := e.RestoreCheckpoint(&cp); err == nil {
		t.Fatal("expected error restoring an uninitialized checkpoint")
	}
}

package gomeboy

import (
	"bytes"
	"reflect"
	"testing"
)

func newFastPathTestEmulator(t *testing.T, opts ...Option) *Emulator {
	t.Helper()
	all := []Option{WithROMBytes(perfROM())}
	all = append(all, opts...)
	e, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestWithoutVideoPreservesSimulationState(t *testing.T) {
	video := newFastPathTestEmulator(t, Headless())
	noVideo := newFastPathTestEmulator(t, Headless(), WithoutVideo())

	video.StepFrames(30)
	noVideo.StepFrames(30)

	var a, b Checkpoint
	video.CheckpointInto(&a)
	noVideo.CheckpointInto(&b)
	// The framebuffer is intentionally different when output is disabled;
	// every other captured simulation field must remain identical.
	a.state.PPU.PreparedFrame = [144][160][3]uint8{}
	b.state.PPU.PreparedFrame = [144][160][3]uint8{}
	if !reflect.DeepEqual(a.state, b.state) {
		t.Fatal("video suppression changed emulation state")
	}
}

func TestCheckpointRestoreRoundTrip(t *testing.T) {
	e := newFastPathTestEmulator(t, Headless(), WithoutVideo())
	e.StepFrames(5)

	var cp Checkpoint
	e.CheckpointInto(&cp)
	before := make([]byte, AddressSpaceSize)
	if _, err := e.SnapshotMemory(before); err != nil {
		t.Fatalf("SnapshotMemory(before): %v", err)
	}
	cycle := e.Cycle()
	frames := e.FrameCount()

	e.Press(ButtonA)
	e.StepFrames(7)
	e.Release(ButtonA)

	if err := e.RestoreCheckpoint(&cp); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	after := make([]byte, AddressSpaceSize)
	if _, err := e.SnapshotMemory(after); err != nil {
		t.Fatalf("SnapshotMemory(after): %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("memory differs after checkpoint restore")
	}
	if got := e.Cycle(); got != cycle {
		t.Fatalf("cycle=%d, want %d", got, cycle)
	}
	if got := e.FrameCount(); got != frames {
		t.Fatalf("frames=%d, want %d", got, frames)
	}
}

func TestReadIntoMatchesRead(t *testing.T) {
	e := newFastPathTestEmulator(t, Headless(), WithoutVideo())
	e.StepFrame()
	want := e.Read(0xC000, 64)
	got := make([]byte, len(want))
	e.ReadInto(0xC000, got)
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadInto mismatch: got %x want %x", got, want)
	}
}

func TestRestoreUninitializedCheckpoint(t *testing.T) {
	e := newFastPathTestEmulator(t, Headless())
	var cp Checkpoint
	if err := e.RestoreCheckpoint(&cp); err == nil {
		t.Fatal("expected error restoring uninitialized checkpoint")
	}
}

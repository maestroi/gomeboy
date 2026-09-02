package gomeboy

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWithoutVideoMatchesExecutionState(t *testing.T) {
	video, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer video.Close()

	noVideo, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer noVideo.Close()

	video.StepFrames(120)
	noVideo.StepFrames(120)

	a := video.gb.Snapshot()
	b := noVideo.gb.Snapshot()
	// PreparedFrame is deliberately output-only and therefore expected to differ.
	a.PPU.PreparedFrame = [144][160][3]uint8{}
	b.PPU.PreparedFrame = [144][160][3]uint8{}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("disabling video changed emulation state")
	}
}

func TestCheckpointRestore(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StepFrames(5)
	wantCycle := e.Cycle()
	wantFrames := e.FrameCount()
	wantMemory := make([]byte, AddressSpaceSize)
	e.PeekInto(0, wantMemory)

	var cp Checkpoint
	e.CheckpointInto(&cp)
	e.Press(ButtonA)
	e.StepFrames(3)
	e.RestoreCheckpoint(&cp)

	if e.Cycle() != wantCycle || e.FrameCount() != wantFrames {
		t.Fatalf("checkpoint restored cycle/frame = %d/%d, want %d/%d", e.Cycle(), e.FrameCount(), wantCycle, wantFrames)
	}
	gotMemory := make([]byte, AddressSpaceSize)
	e.PeekInto(0, gotMemory)
	if !bytes.Equal(gotMemory, wantMemory) {
		t.Fatal("checkpoint did not restore memory")
	}

	// Reuse the same checkpoint object at a new point in time.
	e.StepFrames(2)
	wantCycle = e.Cycle()
	e.CheckpointInto(&cp)
	e.StepFrames(1)
	e.RestoreCheckpoint(&cp)
	if e.Cycle() != wantCycle {
		t.Fatalf("reused checkpoint restored cycle = %d, want %d", e.Cycle(), wantCycle)
	}
}

func TestReadIntoMatchesRead(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	want := e.Read(0xC000, 256)
	got := make([]byte, 256)
	e.ReadInto(0xC000, got)
	if !bytes.Equal(got, want) {
		t.Fatal("ReadInto differs from Read")
	}
}

package gomeboy

import (
	"errors"
	"testing"
)

func TestStepFramesToPublishesEveryFrame(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	var frames []uint64
	var cycles []uint64
	sink := FrameSinkFunc(func(frame uint64, cycle uint64, image Frame) error {
		if image.Width != 160 || image.Height != 144 || len(image.RGB) != 160*144*3 {
			t.Fatalf("unexpected frame shape: %dx%d len=%d", image.Width, image.Height, len(image.RGB))
		}
		frames = append(frames, frame)
		cycles = append(cycles, cycle)
		return nil
	})

	if err := e.StepFramesTo(3, sink); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("published %d frames, want 3", len(frames))
	}
	for i, want := range []uint64{1, 2, 3} {
		if frames[i] != want {
			t.Fatalf("frame[%d] = %d, want %d", i, frames[i], want)
		}
		if cycles[i] == 0 {
			t.Fatalf("cycle[%d] = 0", i)
		}
	}
}

func TestPublishFrameRejectsWithoutVideo(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	err = e.PublishFrame(FrameSinkFunc(func(uint64, uint64, Frame) error { return nil }))
	if err == nil {
		t.Fatal("PublishFrame accepted emulator with video disabled")
	}
}

func TestStepFrameToPropagatesSinkError(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	want := errors.New("sink failed")
	err = e.StepFrameTo(FrameSinkFunc(func(uint64, uint64, Frame) error { return want }))
	if !errors.Is(err, want) {
		t.Fatalf("StepFrameTo error = %v, want wrapped %v", err, want)
	}
	if e.FrameCount() != 1 {
		t.Fatalf("FrameCount = %d, want 1", e.FrameCount())
	}
}

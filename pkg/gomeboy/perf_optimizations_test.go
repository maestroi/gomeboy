package gomeboy

import (
	"bytes"
	"testing"
)

func newPerfTestEmulator(t *testing.T) *Emulator {
	t.Helper()
	e, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestPerfStepFramesMatchesRepeatedStep(t *testing.T) {
	a := newPerfTestEmulator(t)
	b := newPerfTestEmulator(t)

	a.StepFrames(5)
	for i := 0; i < 5; i++ {
		b.StepFrame()
	}

	if a.FrameCount() != b.FrameCount() || a.Cycle() != b.Cycle() {
		t.Fatalf("batch stepping diverged: frames %d/%d cycles %d/%d", a.FrameCount(), b.FrameCount(), a.Cycle(), b.Cycle())
	}
	if !bytes.Equal(a.Frame().RGB, b.Frame().RGB) {
		t.Fatal("batch stepping produced a different frame")
	}
}

func TestPerfPeekIntoMatchesPeek8(t *testing.T) {
	e := newPerfTestEmulator(t)
	dst := make([]byte, 4096)
	e.PeekInto(0xC000, dst)
	for i, got := range dst {
		want := e.Peek8(0xC000 + uint16(i))
		if got != want {
			t.Fatalf("PeekInto byte %d = %02x, want %02x", i, got, want)
		}
	}

	wrap := make([]byte, 4)
	e.PeekInto(0xFFFE, wrap)
	for i, got := range wrap {
		want := e.Peek8(0xFFFE + uint16(i))
		if got != want {
			t.Fatalf("wrapped PeekInto byte %d = %02x, want %02x", i, got, want)
		}
	}
}

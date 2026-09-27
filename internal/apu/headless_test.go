package apu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
)

func TestHighPassStateIsPerAPU(t *testing.T) {
	a := &APU{}
	b := &APU{}

	if got := a.highPass(0, 1, true); got != 1 {
		t.Fatalf("first APU highPass = %v, want 1", got)
	}
	if got := b.highPass(0, 1, true); got != 1 {
		t.Fatalf("second APU inherited filter state: got %v, want 1", got)
	}
}

func TestHeadlessRemovesOutputSamplingEvent(t *testing.T) {
	s := scheduler.NewScheduler()
	b := io.NewBus(s, make([]byte, 32*1024))
	a := New(b, s)

	if got := s.Until(scheduler.APUSample); got == 0 {
		t.Fatal("APUSample was not scheduled by default")
	}
	a.SetHeadless(true)
	if got := s.Until(scheduler.APUSample); got != 0 {
		t.Fatalf("APUSample still scheduled in headless mode: %d cycles", got)
	}
	if a.buffer != nil || a.bufferPos != 0 {
		t.Fatal("headless mode retained transient audio buffer")
	}

	a.SetHeadless(false)
	if got := s.Until(scheduler.APUSample); got == 0 {
		t.Fatal("APUSample was not restored after leaving headless mode")
	}
	if len(a.buffer) != bufferSize {
		t.Fatalf("audio buffer length = %d, want %d", len(a.buffer), bufferSize)
	}
}

func TestRestoreKeepsHostHeadlessMode(t *testing.T) {
	newAPU := func() (*APU, *scheduler.Scheduler) {
		s := scheduler.NewScheduler()
		return New(io.NewBus(s, make([]byte, 32*1024)), s), s
	}
	audible, _ := newAPU()
	headless, hs := newAPU()
	headless.SetHeadless(true)

	// A snapshot from an audio-producing emulator must not re-enable
	// unbounded sample buffering in a headless one.
	headless.Restore(audible.Snapshot())
	if !headless.headless || headless.buffer != nil {
		t.Fatal("restoring an audible snapshot turned headless output back on")
	}
	if got := hs.Until(scheduler.APUSample); got != 0 {
		t.Fatalf("APUSample scheduled after headless restore: %d cycles", got)
	}

	// The reverse keeps an audible emulator sampling.
	audible2, as := newAPU()
	audible2.Restore(headless.Snapshot())
	if audible2.headless || len(audible2.buffer) < bufferSize {
		t.Fatal("restoring a headless snapshot silenced an audible emulator")
	}
	if got := as.Until(scheduler.APUSample); got == 0 {
		t.Fatal("APUSample not scheduled after audible restore of a headless snapshot")
	}
}

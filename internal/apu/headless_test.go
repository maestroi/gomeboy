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

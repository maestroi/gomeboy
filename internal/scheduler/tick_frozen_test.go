package scheduler

import "testing"

func TestTickFrozenDelaysPPUEventsAndStamps(t *testing.T) {
	s := NewScheduler()
	ppuAt, apuAt := uint64(0), uint64(0)
	s.RegisterEvent(PPUHandleVisualLine, func() { ppuAt = s.Cycle() })
	s.RegisterEvent(APUSample, func() { apuAt = s.Cycle() })
	stamp := uint64(0)
	s.RegisterDeviceStamp(&stamp)
	s.ScheduleEvent(PPUHandleVisualLine, 10)
	s.ScheduleEvent(APUSample, 6)
	stamp = 2

	s.TickFrozen(5)
	s.Tick(20)

	if ppuAt != 15 {
		t.Fatalf("PPU event ran at %d, want 15 (delayed by the freeze)", ppuAt)
	}
	if apuAt != 6 {
		t.Fatalf("non-PPU event ran at %d, want 6 (unaffected)", apuAt)
	}
	if stamp != 7 {
		t.Fatalf("stamp = %d, want 7", stamp)
	}
}

func TestChangeSpeedRebasesDeviceStamps(t *testing.T) {
	s := NewScheduler()
	stamp := uint64(0)
	s.RegisterDeviceStamp(&stamp)
	s.Tick(100)
	stamp = 90 // 10 normal-speed dots ago

	s.ChangeSpeed(true)
	if got := s.Cycle() - stamp; got != 20 {
		t.Fatalf("elapsed after entering DS = %d, want 20", got)
	}
	s.ChangeSpeed(false)
	if got := s.Cycle() - stamp; got != 10 {
		t.Fatalf("elapsed after leaving DS = %d, want 10", got)
	}
}

package scheduler

import "testing"

func TestDescheduleRootUpdatesNextEvent(t *testing.T) {
	s := NewScheduler()
	earlyRan := false
	futureRan := false
	s.RegisterEvent(APUSample, func() { earlyRan = true })
	s.RegisterEvent(APUChannel1, func() { futureRan = true })
	s.ScheduleEvent(APUSample, 4)
	s.ScheduleEvent(APUChannel1, 20)

	s.DescheduleEvent(APUSample)
	s.Tick(4)
	if earlyRan {
		t.Fatal("descheduled event ran")
	}
	if futureRan {
		t.Fatal("future event ran early after root deschedule")
	}
	if got := s.Until(APUChannel1); got != 16 {
		t.Fatalf("Until(APUChannel1) = %d, want 16", got)
	}

	s.Tick(16)
	if !futureRan {
		t.Fatal("future event did not run at its scheduled cycle")
	}
}

func TestDescheduleDoesNotRemoveSentinel(t *testing.T) {
	s := NewScheduler()
	s.DescheduleEvent(EventType(0))
	s.Tick(1)
	if got := s.Cycle(); got != 1 {
		t.Fatalf("Cycle() = %d, want 1", got)
	}
}

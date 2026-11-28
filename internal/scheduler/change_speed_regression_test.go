package scheduler

import "testing"

func TestChangeSpeedReschedulesAllAffectedEvents(t *testing.T) {
	s := NewScheduler()
	for _, typ := range []EventType{APUFrameSequencer, APUChannel1, APUChannel2} {
		s.RegisterEvent(typ, func() {})
	}
	s.ScheduleEvent(APUFrameSequencer, 10)
	s.ScheduleEvent(APUChannel1, 40)
	s.ScheduleEvent(APUChannel2, 80)

	// Entering double speed keeps the same device-dot lengths but each
	// dot now costs two CPU cycles (ScheduleEvent ×2).
	s.ChangeSpeed(true)
	for typ, want := range map[EventType]uint64{
		APUFrameSequencer: 20,
		APUChannel1:       80,
		APUChannel2:       160,
	} {
		if got := s.Until(typ); got != want {
			t.Fatalf("Until(%v) after enter double speed = %d, want %d", typ, got, want)
		}
	}

	// Leaving double speed restores one CPU cycle per device dot.
	s.ChangeSpeed(false)
	for typ, want := range map[EventType]uint64{
		APUFrameSequencer: 10,
		APUChannel1:       40,
		APUChannel2:       80,
	} {
		if got := s.Until(typ); got != want {
			t.Fatalf("Until(%v) after leave double speed = %d, want %d", typ, got, want)
		}
	}

	empty := NewScheduler()
	empty.ChangeSpeed(true)
	empty.Tick(1)
	if got := empty.Cycle(); got != 1 {
		t.Fatalf("empty scheduler cycle = %d, want 1", got)
	}
}

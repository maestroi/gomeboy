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

	s.ChangeSpeed(true)
	for typ, want := range map[EventType]uint64{
		APUFrameSequencer: 5,
		APUChannel1:       20,
		APUChannel2:       40,
	} {
		if got := s.Until(typ); got != want {
			t.Fatalf("Until(%v) after speed change = %d, want %d", typ, got, want)
		}
	}

	empty := NewScheduler()
	empty.ChangeSpeed(true)
	empty.Tick(1)
	if got := empty.Cycle(); got != 1 {
		t.Fatalf("empty scheduler cycle = %d, want 1", got)
	}
}

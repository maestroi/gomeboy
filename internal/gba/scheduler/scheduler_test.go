package scheduler

import (
	"reflect"
	"testing"
)

func TestOrdersEventsByTimestampPriorityAndInsertion(t *testing.T) {
	s := New()
	var got []string
	s.Schedule(4, PriorityNormal, func() { got = append(got, "later") })
	s.Schedule(2, PriorityLate, func() { got = append(got, "late") })
	s.Schedule(2, PriorityEarly, func() { got = append(got, "early") })
	s.Schedule(2, PriorityLate, func() { got = append(got, "late-2") })

	s.Advance(4)

	want := []string{"early", "late", "late-2", "later"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event order = %v, want %v", got, want)
	}
	if s.Now() != 4 {
		t.Fatalf("timestamp = %d, want 4", s.Now())
	}
}

func TestSameTimestampEventScheduledByCallbackRunsBeforeAdvancing(t *testing.T) {
	s := New()
	var got []string
	s.Schedule(1, PriorityNormal, func() {
		got = append(got, "parent")
		s.Schedule(0, PriorityLate, func() { got = append(got, "child") })
	})
	s.Advance(2)
	if want := []string{"parent", "child"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event order = %v, want %v", got, want)
	}
}

func TestCancel(t *testing.T) {
	s := New()
	fired := false
	h := s.Schedule(1, PriorityNormal, func() { fired = true })
	if !s.Cancel(h) {
		t.Fatal("Cancel returned false for pending event")
	}
	s.Advance(2)
	if fired {
		t.Fatal("canceled event fired")
	}
	if s.Cancel(h) {
		t.Fatal("Cancel returned true twice")
	}
}

func TestNextSkipsCanceledEvents(t *testing.T) {
	s := New()
	h := s.Schedule(1, PriorityNormal, func() {})
	s.Schedule(3, PriorityNormal, func() {})
	s.Cancel(h)
	if next, ok := s.Next(); !ok || next != 3 {
		t.Fatalf("Next = %d, %v, want 3, true", next, ok)
	}
}

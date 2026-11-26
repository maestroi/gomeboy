package timer

// TimerState captures one GBA hardware timer. Scheduler handles are deliberately
// excluded; pending overflow callbacks are rebuilt from logical timestamps.
type TimerState struct {
	Reload         uint16
	Counter        uint16
	Control        uint16
	LastEvent      uint64
	LastTickAt     uint64
	LastOverflowAt uint64
}

// PendingWriteState captures a deferred CPU timer control write.
type PendingWriteState struct {
	Index int
	Value uint16
}

// State captures timer counters, timestamp phase, deferred control writes and
// the timer scheduler's master timestamp.
type State struct {
	Timers           [4]TimerState
	Active           uint8
	DeferWrites      bool
	PendingWrites    []PendingWriteState
	PendingBusWrites []PendingWriteState
	SchedulerNow     uint64
}

func snapshotWrites(dst []PendingWriteState, src []pendingWrite) []PendingWriteState {
	if cap(dst) < len(src) {
		dst = make([]PendingWriteState, len(src))
	} else {
		dst = dst[:len(src)]
	}
	for i, w := range src {
		dst[i] = PendingWriteState{Index: w.index, Value: w.value}
	}
	return dst
}

func restoreWrites(dst []pendingWrite, src []PendingWriteState) []pendingWrite {
	if cap(dst) < len(src) {
		dst = make([]pendingWrite, len(src))
	} else {
		dst = dst[:len(src)]
	}
	for i, w := range src {
		dst[i] = pendingWrite{index: w.Index, value: w.Value}
	}
	return dst
}

func (t *Timers) Snapshot() State {
	var s State
	t.SnapshotInto(&s)
	return s
}

func (t *Timers) SnapshotInto(s *State) {
	if s == nil {
		return
	}
	for i, v := range t.timer {
		s.Timers[i] = TimerState{
			Reload: v.reload, Counter: v.counter, Control: v.control,
			LastEvent: v.lastEvent, LastTickAt: v.lastTickAt, LastOverflowAt: v.lastOverflowAt,
		}
	}
	s.Active = t.active
	s.DeferWrites = t.deferWrites
	s.PendingWrites = snapshotWrites(s.PendingWrites, t.pendingWrites)
	s.PendingBusWrites = snapshotWrites(s.PendingBusWrites, t.pendingBusWrites)
	s.SchedulerNow = t.scheduler.Now()
}

func (t *Timers) Restore(s State) {
	for i := range t.timer {
		t.cancelOverflow(i)
	}
	t.scheduler.Reset(s.SchedulerNow)
	for i, v := range s.Timers {
		t.timer[i] = state{
			reload: v.Reload, counter: v.Counter, control: v.Control,
			lastEvent: v.LastEvent, lastTickAt: v.LastTickAt, lastOverflowAt: v.LastOverflowAt,
		}
	}
	t.active = s.Active & 0x0f
	t.deferWrites = s.DeferWrites
	t.pendingWrites = restoreWrites(t.pendingWrites, s.PendingWrites)
	t.pendingBusWrites = restoreWrites(t.pendingBusWrites, s.PendingBusWrites)
	for i := range t.timer {
		t.scheduleOverflow(i)
	}
}

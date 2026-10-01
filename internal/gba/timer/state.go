package timer

// TimerState captures one GBA hardware timer.
type TimerState struct {
	Reload           uint16
	Counter          uint16
	Control          uint16
	Phase            uint32
	LastTickOverflow bool
}

// PendingWriteState captures a deferred CPU timer-register write.
type PendingWriteState struct {
	Index int
	Kind  uint8
	Value uint16
}

// State captures timer counters, phase, and deferred write queues.
type State struct {
	Timers           [4]TimerState
	Active           uint8
	DeferWrites      bool
	PendingWrites    []PendingWriteState
	PendingBusWrites []PendingWriteState
}

func snapshotWrites(dst []PendingWriteState, src []pendingWrite) []PendingWriteState {
	if cap(dst) < len(src) {
		dst = make([]PendingWriteState, len(src))
	} else {
		dst = dst[:len(src)]
	}
	for i, w := range src {
		dst[i] = PendingWriteState{Index: w.index, Kind: uint8(w.kind), Value: w.value}
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
		dst[i] = pendingWrite{index: w.Index, kind: pendingWriteKind(w.Kind), value: w.Value}
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
			Phase: v.phase, LastTickOverflow: v.lastTickOverflow,
		}
	}
	s.Active = t.active
	s.DeferWrites = t.deferWrites
	s.PendingWrites = snapshotWrites(s.PendingWrites, t.pendingWrites)
	s.PendingBusWrites = snapshotWrites(s.PendingBusWrites, t.pendingBusWrites)
}

func (t *Timers) Restore(s State) {
	for i, v := range s.Timers {
		t.timer[i] = state{
			reload: v.Reload, counter: v.Counter, control: v.Control,
			phase: v.Phase, lastTickOverflow: v.LastTickOverflow,
		}
	}
	t.active = s.Active & 0x0f
	t.deferWrites = s.DeferWrites
	t.pendingWrites = restoreWrites(t.pendingWrites, s.PendingWrites)
	t.pendingBusWrites = restoreWrites(t.pendingBusWrites, s.PendingBusWrites)
}

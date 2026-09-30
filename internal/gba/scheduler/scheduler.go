// Package scheduler provides the timestamped event queue used by GBA hardware.
//
// The scheduler deliberately knows nothing about individual peripherals. Event
// ordering is expressed by timestamp and a small same-cycle priority, allowing
// timer/IRQ/DMA hardware to model edges without depending on Advance chunking.
package scheduler

import "container/heap"

// Priority orders events that share a timestamp. Lower values run first.
type Priority uint8

const (
	PriorityEarly Priority = iota
	PriorityNormal
	PriorityLate
	PriorityLatest
)

// Handle identifies a scheduled event. The zero handle is invalid.
type Handle uint64

type event struct {
	at       uint64
	priority Priority
	seq      uint64
	handle   Handle
	callback func()
	canceled bool
}

type eventHeap []*event

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	if h[i].priority != h[j].priority {
		return h[i].priority < h[j].priority
	}
	return h[i].seq < h[j].seq
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any) { *h = append(*h, x.(*event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Scheduler owns the GBA master timestamp and deferred hardware events.
type Scheduler struct {
	now        uint64
	nextHandle Handle
	nextSeq    uint64
	events     eventHeap
	byHandle   map[Handle]*event
}

// New creates an empty scheduler at timestamp zero.
func New() *Scheduler {
	s := &Scheduler{byHandle: make(map[Handle]*event)}
	heap.Init(&s.events)
	return s
}

// Now returns the current master-clock timestamp.
func (s *Scheduler) Now() uint64 { return s.now }

// Schedule adds callback delay cycles from the current timestamp.
func (s *Scheduler) Schedule(delay uint64, priority Priority, callback func()) Handle {
	if callback == nil {
		panic("gba scheduler: nil callback")
	}
	s.nextHandle++
	if s.nextHandle == 0 {
		s.nextHandle++
	}
	s.nextSeq++
	e := &event{at: s.now + delay, priority: priority, seq: s.nextSeq, handle: s.nextHandle, callback: callback}
	heap.Push(&s.events, e)
	s.byHandle[e.handle] = e
	return e.handle
}

// Cancel prevents a pending event from firing.
func (s *Scheduler) Cancel(handle Handle) bool {
	e, ok := s.byHandle[handle]
	if !ok {
		return false
	}
	delete(s.byHandle, handle)
	e.canceled = true
	return true
}

// Next returns the timestamp of the next live event.
func (s *Scheduler) Next() (uint64, bool) {
	s.discardCanceled()
	if len(s.events) == 0 {
		return 0, false
	}
	return s.events[0].at, true
}

// AdvanceTo runs every event due through target and leaves Now at target.
// Callbacks may schedule additional same-timestamp events; those are processed
// before time advances beyond that timestamp.
func (s *Scheduler) AdvanceTo(target uint64) {
	if target < s.now {
		panic("gba scheduler: time moved backwards")
	}
	for {
		s.discardCanceled()
		if len(s.events) == 0 || s.events[0].at > target {
			break
		}
		e := heap.Pop(&s.events).(*event)
		delete(s.byHandle, e.handle)
		s.now = e.at
		e.callback()
	}
	s.now = target
}

// Advance moves forward by cycles and dispatches due events.
func (s *Scheduler) Advance(cycles uint64) { s.AdvanceTo(s.now + cycles) }

func (s *Scheduler) discardCanceled() {
	for len(s.events) != 0 && s.events[0].canceled {
		heap.Pop(&s.events)
	}
}

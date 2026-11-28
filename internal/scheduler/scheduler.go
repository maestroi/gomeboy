package scheduler

import (
	"fmt"
	"math"
)

// Scheduler is a simple event scheduler that can be used to schedule events
// to be executed at a specific cycle.
//
// The scheduler is a linked list of events, sorted by the cycle at which
// they should be executed. When an event is scheduled, it is inserted into
// the list in the correct position, and when the scheduler is ticked, the
// next event is executed and removed from the list, if the event is scheduled
// for the current cycle.
type Scheduler struct {
	cycles   uint64
	divTimer uint64 // the cycle at which the DIV register was last reset
	root     *Event

	events      [len(_EventType_index) - 1]*Event // only one event of each type can be scheduled at a time
	nextEventAt uint64                            // the cycle at which the next event should be executed

	doubleSpeed bool // whether the scheduler is running at double speed

	Halted bool
}

func (s *Scheduler) OverrideDiv(div uint16) {
	s.divTimer = s.cycles - uint64(div)
}

func NewScheduler() *Scheduler {
	s := &Scheduler{ // 0x = DMG magic value,
		cycles:      0,
		nextEventAt: math.MaxUint64,
		root: &Event{
			next:  nil,
			cycle: math.MaxUint64,
			handler: func() {

			},
		},
	}

	// initialize the events with the number of event types
	// to avoid the cost of allocating a new event for each
	// scheduled event
	for i := 0; i < len(_EventType_index)-1; i++ {
		s.events[i] = &Event{}
	}

	return s
}

// State is a snapshot of the scheduler's execution state. It captures the
// current cycle, the DIV timer, the speed mode, and the set of pending
// events (by type and absolute cycle). Restoring a State rebuilds the
// pending event list exactly.
type State struct {
	Cycles      uint64
	DivTimer    uint64
	DoubleSpeed bool
	Halted      bool
	// Events holds the absolute cycle at which each event type is
	// pending. A value of 0 (or MaxUint64) means the event is not scheduled.
	Events [len(_EventType_index) - 1]uint64
}

// Snapshot captures the scheduler's execution state.
func (s *Scheduler) Snapshot() State {
	st := State{
		Cycles:      s.cycles,
		DivTimer:    s.divTimer,
		DoubleSpeed: s.doubleSpeed,
		Halted:      s.Halted,
	}
	for e := s.root; e != nil; e = e.next {
		// skip the empty-list sentinel
		if e.cycle == math.MaxUint64 {
			break
		}
		st.Events[e.eventType] = e.cycle
	}
	return st
}

// Restore rebuilds the scheduler's execution state from a snapshot.
func (s *Scheduler) Restore(st State) {
	s.cycles = st.Cycles
	s.divTimer = st.DivTimer
	s.doubleSpeed = st.DoubleSpeed
	s.Halted = st.Halted

	// reset the event list to the empty sentinel. The sentinel is the tail
	// node of the current (valid) list, so walk to it and re-root there.
	sentinel := s.root
	for sentinel.next != nil {
		sentinel = sentinel.next
	}
	s.root = sentinel
	sentinel.next = nil
	s.nextEventAt = math.MaxUint64

	// re-insert the pending events in sorted order
	for i, cycle := range st.Events {
		if cycle == 0 || cycle == math.MaxUint64 {
			continue
		}
		s.insertAbsolute(EventType(i), cycle)
	}
}

// insertAbsolute inserts the pooled event node for eventType into the list
// at the given absolute cycle, maintaining sort order. It bypasses the
// double-speed cycle scaling that ScheduleEvent applies.
func (s *Scheduler) insertAbsolute(eventType EventType, atCycle uint64) {
	event := s.events[eventType]
	event.cycle = atCycle

	if atCycle <= s.nextEventAt {
		event.next = s.root
		s.root = event
		s.nextEventAt = atCycle
		return
	}

	var prev *Event = s.root
	for prev.next != nil && prev.next.cycle <= atCycle {
		prev = prev.next
	}
	event.next = prev.next
	prev.next = event
}

func (s *Scheduler) Cycle() uint64 {
	return s.cycles
}

// RegisterEvent registers a function of the EventType to be called when
// the event is scheduled for execution. This is to avoid the cost of
// having to allocate a function for each event, which would frequently
// invoke the garbage collector, despite the functions always performing
// the same task.
func (s *Scheduler) RegisterEvent(eventType EventType, fn func()) {
	s.events[eventType].handler = fn
	s.events[eventType].eventType = eventType
}

// Tick advances the scheduler by the given number of cycles. This will
// execute all scheduled events up to the current cycle. If an event is
// scheduled for the current cycle, it will be executed and removed from
// the list. If an event is scheduled for a cycle in the future, it will
// be executed when the scheduler is ticked with the cycle at which it
// should be executed.
func (s *Scheduler) Tick(c uint64) {
	// what cycle are we advancing to
	cycleToAdvance := s.cycles + c

	// if the next event occurs in the future, simply increment
	// the cycle count and return
	if s.nextEventAt > cycleToAdvance {
		s.cycles = cycleToAdvance
		return
	}

	// get the cycle at which the next event occurs
	nextEvent := s.nextEventAt

	// otherwise perform accurate loop, jumping from each event
	// (mostly to pass blarggs wave read/write/trigger - maybe make
	// this optional in the future?)
	for nextEvent <= cycleToAdvance {
		s.cycles = nextEvent

		nextEvent = s.doEvents(s.cycles)
	}

	// update cycles and next event
	s.cycles = cycleToAdvance
	s.nextEventAt = nextEvent
}

// SysClock returns the internal divider clock of the Game Boy.
func (s *Scheduler) SysClock() uint16 {
	return uint16((s.cycles - s.divTimer) & 0xFFFF)
}

// SysClockReset resets the internal divider clock. TODO notify
// timer of reset.
func (s *Scheduler) SysClockReset() {
	s.divTimer = s.cycles
}

// doEvents executes all events scheduled in the list up to the given
// cycle. It returns the cycle at which the next event should be executed.
func (s *Scheduler) doEvents(until uint64) uint64 {
	nextEvent := s.nextEventAt

	for nextEvent <= until {
		// we need to copy the event to a local variable
		// as the handler may schedule a new event, which
		// could modify the event in the list
		event := s.root

		// set the next event to be executed
		s.root = event.next

		// execute the event
		event.handler()

		nextEvent = s.root.cycle
	}

	return nextEvent
}

// ScheduleEvent schedules an event to be executed at the given cycle.
func (s *Scheduler) ScheduleEvent(eventType EventType, cycle uint64) {
	if s.doubleSpeed && eventType <= PPUHandleOffscreenLine {
		cycle = cycle * 2
	}
	// when the event is scheduled, it is scheduled for the current cycle + the cycle
	// at which it should be executed
	atCycle := s.cycles + cycle

	// get the event to insert from the event pool and set the cycle
	// this is to avoid the cost of allocating a new event for each
	// scheduled event
	eventToInsert := s.events[eventType]
	eventToInsert.cycle = atCycle

	// if the event is scheduled for a cycle before the next scheduled
	// event, then we can just prepend it to the list by setting it as
	// the root and updating the next event to be the current root
	if atCycle <= s.nextEventAt {
		eventToInsert.next = s.root
		s.root = eventToInsert
		s.nextEventAt = atCycle

		// early return to avoid iterating over the list
		return
	}

	// iterate over the list of events to find the correct position
	// to insert the event
	var currentRoot = s.root // start at the root
	var nextCycle = currentRoot.cycle
	var didLoop = currentRoot.cycle <= atCycle
	var prev *Event
eventLoop:
	// typically you would use a for loop here, but in this case,
	// the performance critical nature of the scheduler means
	// that we need to avoid as much overhead as possible
	if nextCycle <= atCycle {
		// when the event is scheduled for a cycle after the current
		// root, then we need to insert the event after the current
		// root, so we need to continue iterating over the list
		// until we find the correct position
		prev = currentRoot
		currentRoot = currentRoot.next
		if currentRoot != nil {
			nextCycle = currentRoot.cycle
			goto eventLoop
		}
	}

	// TODO use positions instead of nil checks

	if !didLoop {
		eventToInsert.next = currentRoot
		s.root = eventToInsert
		s.nextEventAt = atCycle
	} else {
		prev.next = eventToInsert
		prev.next.next = currentRoot
	}

}

func (s *Scheduler) DescheduleEvent(eventType EventType) {
	if s.root == nil {
		return
	}

	var prev *Event
	event := s.root

	// Never remove the MaxUint64 sentinel. Its zero-value event type may
	// otherwise alias a real EventType and leave the scheduler without a tail.
	for event != nil && event.cycle != math.MaxUint64 {
		if event.eventType == eventType {
			if prev == nil {
				s.root = event.next
				if s.root != nil {
					s.nextEventAt = s.root.cycle
				} else {
					s.nextEventAt = math.MaxUint64
				}
			} else {
				prev.next = event.next
			}
			event.next = nil
			break
		}
		prev = event
		event = event.next
	}
}

// TODO pass in event type to avoid BCE in the loop
// TODO
func (s *Scheduler) DoEvent() uint64 {
	event := s.root

	s.root = event.next
	event.handler()

	return s.root.cycle
}

// ChangeSpeed informs the scheduler that the speed of the CPU has either gone
// from normal speed to double speed, or from double speed to normal speed.
//
// Device events (APU/PPU/…) are scheduled in device dots. ScheduleEvent
// converts those to CPU cycles with a ×2 when doubleSpeed is set. On a
// KEY1 flip we therefore: (1) recover each queued event's remaining
// length in dots, (2) flip doubleSpeed, (3) reschedule with that same
// dot length so ScheduleEvent applies the new conversion. The old
// implementation halved remaining cycles when entering double speed,
// which made the PPU run 2× fast after KEY1 (AGE lcd-align-ly DS rows).
func (s *Scheduler) ChangeSpeed(speed bool) {
	if s.doubleSpeed == speed {
		return
	}

	type pending struct {
		typ  EventType
		dots uint64
	}
	var list []pending
	seen := [len(_EventType_index) - 1]bool{}

	for event := s.root; event != nil && event.cycle != math.MaxUint64; event = event.next {
		if event.eventType > PPUHandleOffscreenLine || seen[event.eventType] {
			continue
		}
		seen[event.eventType] = true
		cyclesFromNow := event.cycle - s.cycles
		dots := cyclesFromNow
		if s.doubleSpeed {
			dots = cyclesFromNow / 2
		}
		list = append(list, pending{event.eventType, dots})
	}
	for _, p := range list {
		s.DescheduleEvent(p.typ)
	}

	s.doubleSpeed = speed

	for _, p := range list {
		s.ScheduleEvent(p.typ, p.dots)
	}
}

// Skip invokes the scheduler to execute the next event, by setting the
// current cycle to the cycle at which the next event is scheduled to be
// executed. This is useful when the CPU is halted, and the scheduler
// should be invoked to execute until the CPU is un-halted by an interrupt.
func (s *Scheduler) Skip() {
	s.cycles = s.nextEventAt
	s.nextEventAt = s.DoEvent()
}

func (s *Scheduler) String() string {
	result := ""
	event := s.root
	for event != nil {
		//fmt.Println(event)
		result += fmt.Sprintf("%s:%d->", event.eventType, event.cycle)
		event = event.next
	}
	return result
}

func (s *Scheduler) Until(increment EventType) uint64 {
	event := s.root
	for event != nil {
		if event.eventType == increment {
			return event.cycle - s.cycles
		}
		event = event.next
	}
	return 0
}

func (s *Scheduler) DoubleSpeed() bool {
	return s.doubleSpeed
}

// DivAPUBit returns the current bitmask for scheduling
// the APU frame sequencer. In CGB double speed bit 5 of
// types.DIV is used, otherwise it is bit 4 (note bit 12/13
// of the SysClock)
func (s *Scheduler) DivAPUBit() uint16 {
	if s.doubleSpeed {
		return 0x2000
	}
	return 0x1000
}

func (s *Scheduler) Events() [][3]string {
	events := make([][3]string, 0)

	event := s.root
	for event != nil {
		if event.cycle == math.MaxUint64 {
			break // last event in list
		}
		events = append(events, [3]string{
			fmt.Sprintf("%-8d", event.cycle-s.Cycle()),
			fmt.Sprintf("%-16d", event.cycle),
			fmt.Sprintf("%s", event.eventType),
		})
		event = event.next
	}

	return events
}

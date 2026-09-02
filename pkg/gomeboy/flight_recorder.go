package gomeboy

import "fmt"

// FlightEventKind identifies a flight-recorder event.
type FlightEventKind string

const (
	FlightFrame        FlightEventKind = "frame"
	FlightInput        FlightEventKind = "input"
	FlightMemoryChange FlightEventKind = "memory_change"
)

// FlightEvent is one bounded diagnostic event. Fields not relevant to Kind
// are zero-valued.
type FlightEvent struct {
	Kind    FlightEventKind
	Frame   uint64
	Cycle   uint64
	PC      uint16
	Address uint16
	Before  byte
	After   byte
	Button  Button
	Pressed bool
}

// FlightRecorderOptions configures an in-memory bounded flight recorder.
// Memory ranges are sampled after every StepFrame; traced StepFrames therefore
// takes the per-frame path intentionally. Capacity defaults to 4096 events.
type FlightRecorderOptions struct {
	Capacity     int
	Memory       []MemoryRange
	RecordFrames bool
}

type flightRange struct {
	range_  MemoryRange
	before  []byte
	current []byte
}

// FlightRecorder keeps only the newest Capacity events. It is owned by one
// Emulator and, like Emulator itself, is not safe for concurrent use.
type FlightRecorder struct {
	capacity     int
	recordFrames bool
	events       []FlightEvent
	next         int
	full         bool
	ranges       []flightRange
}

// EnableFlightRecorder starts a new recorder, replacing any existing one.
// It snapshots watched memory immediately, so the first memory-change event
// means "changed after enabling" rather than "different from zero".
func (e *Emulator) EnableFlightRecorder(opts FlightRecorderOptions) error {
	capacity := opts.Capacity
	if capacity == 0 {
		capacity = 4096
	}
	if capacity < 0 {
		return fmt.Errorf("gomeboy: flight recorder capacity must be >= 0")
	}
	r := &FlightRecorder{
		capacity:     capacity,
		recordFrames: opts.RecordFrames,
		events:       make([]FlightEvent, 0, capacity),
	}
	for _, mr := range opts.Memory {
		if err := validateMemoryRange(mr); err != nil {
			return fmt.Errorf("gomeboy: flight recorder: %w", err)
		}
		fr := flightRange{
			range_:  mr,
			before:  make([]byte, mr.Length),
			current: make([]byte, mr.Length),
		}
		e.PeekInto(mr.Start, fr.before)
		r.ranges = append(r.ranges, fr)
	}
	e.flight = r
	return nil
}

// DisableFlightRecorder detaches the recorder and returns its events in
// chronological order.
func (e *Emulator) DisableFlightRecorder() []FlightEvent {
	if e.flight == nil {
		return nil
	}
	out := e.flight.Events()
	e.flight = nil
	return out
}

// FlightEvents returns the currently attached recorder's events in
// chronological order without disabling it.
func (e *Emulator) FlightEvents() []FlightEvent {
	if e.flight == nil {
		return nil
	}
	return e.flight.Events()
}

// Events returns a chronological copy of the recorder's bounded ring.
func (r *FlightRecorder) Events() []FlightEvent {
	if r == nil || len(r.events) == 0 {
		return nil
	}
	if !r.full {
		out := make([]FlightEvent, len(r.events))
		copy(out, r.events)
		return out
	}
	out := make([]FlightEvent, 0, len(r.events))
	out = append(out, r.events[r.next:]...)
	out = append(out, r.events[:r.next]...)
	return out
}

func (r *FlightRecorder) append(event FlightEvent) {
	if r == nil || r.capacity == 0 {
		return
	}
	if len(r.events) < r.capacity {
		r.events = append(r.events, event)
		if len(r.events) == r.capacity {
			r.full = true
			r.next = 0
		}
		return
	}
	r.events[r.next] = event
	r.next++
	if r.next == r.capacity {
		r.next = 0
	}
}

func (r *FlightRecorder) recordInput(event InputEvent) {
	r.append(FlightEvent{
		Kind:    FlightInput,
		Frame:   event.Frame,
		Cycle:   event.Cycle,
		Button:  event.Button,
		Pressed: event.Pressed,
	})
}

func (e *Emulator) recordFlightFrame() {
	e.recordFlightSample(true)
}

func (e *Emulator) recordFlightInstruction() {
	e.recordFlightSample(false)
}

func (e *Emulator) recordFlightSample(includeFrame bool) {
	r := e.flight
	if r == nil {
		return
	}
	frame := e.FrameCount()
	cycle := e.Cycle()
	pc := e.gb.CPU.PC
	for i := range r.ranges {
		fr := &r.ranges[i]
		e.PeekInto(fr.range_.Start, fr.current)
		for off, after := range fr.current {
			before := fr.before[off]
			if before == after {
				continue
			}
			r.append(FlightEvent{
				Kind:    FlightMemoryChange,
				Frame:   frame,
				Cycle:   cycle,
				PC:      pc,
				Address: fr.range_.Start + uint16(off),
				Before:  before,
				After:   after,
			})
		}
		fr.before, fr.current = fr.current, fr.before
	}
	if includeFrame && r.recordFrames {
		r.append(FlightEvent{
			Kind:  FlightFrame,
			Frame: frame,
			Cycle: cycle,
			PC:    pc,
		})
	}
}

func (r *FlightRecorder) needsFrameSampling() bool {
	return r != nil && (r.recordFrames || len(r.ranges) != 0)
}

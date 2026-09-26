package gomeboy

import (
	"fmt"

	"github.com/maestroi/gomeboy/internal/serial"
)

// LinkEventKind names one externally-sourced serial link fact in a session
// recording.
type LinkEventKind string

const (
	// LinkAttach marks AttachLink at a frame boundary.
	LinkAttach LinkEventKind = "attach"
	// LinkExchange is the incoming bit of one internally-clocked transfer bit.
	LinkExchange LinkEventKind = "exchange"
	// LinkClock is one external clock pulse the peer delivered to a poll.
	LinkClock LinkEventKind = "clock"
)

// LinkEvent is one non-deterministic result a network link fed into emulation.
// Joypad input alone cannot reproduce a linked session: the peer's bits and
// the cycle at which each external pulse became ready depend on the network.
// Recording every result at its emulated cycle lets replay substitute the
// peer exactly. Not-ready external polls are implied by the absence of a
// LinkClock event at that cycle.
type LinkEvent struct {
	Kind  LinkEventKind `json:"kind"`
	Frame uint64        `json:"frame,omitempty"` // LinkAttach only
	Cycle uint64        `json:"cycle"`
	Bit   bool          `json:"bit,omitempty"`
}

// linkTap wraps an attached network device and records what it returns to the
// serial controller while a session recording is active.
type linkTap struct {
	inner *serial.NetworkDevice
	e     *Emulator
}

func (t *linkTap) Send() bool     { return t.inner.Send() }
func (t *linkTap) Receive(b bool) { t.inner.Receive(b) }

func (t *linkTap) ExchangeBit(out bool) (bool, error) {
	in, err := t.inner.ExchangeBit(out)
	if err != nil {
		// The controller reads a failed exchange as an unplugged (high) line.
		in = true
	}
	t.e.recordLinkEvent(LinkEvent{Kind: LinkExchange, Cycle: t.e.Cycle(), Bit: in})
	return in, err
}

func (t *linkTap) PollClock() (serial.ClockPulse, bool) {
	pulse, ok := t.inner.PollClock()
	if ok {
		t.e.recordLinkEvent(LinkEvent{Kind: LinkClock, Cycle: t.e.Cycle(), Bit: pulse.Incoming})
	}
	return pulse, ok
}

func (t *linkTap) ReplyClock(pulse serial.ClockPulse, out bool) error {
	return t.inner.ReplyClock(pulse, out)
}

func (e *Emulator) recordLinkEvent(event LinkEvent) {
	if e.linkRecording {
		e.linkLog = append(e.linkLog, event)
	}
}

// linkReplay stands in for the recorded peer. It returns each recorded result
// only at the exact cycle it was recorded, so a replay that drifts fails with
// a named cycle instead of silently playing a different game.
type linkReplay struct {
	e      *Emulator
	events []LinkEvent
	next   int
	err    error
}

func (r *linkReplay) Send() bool   { return true }
func (r *linkReplay) Receive(bool) {}

func (r *linkReplay) ExchangeBit(bool) (bool, error) {
	if event, ok := r.take(LinkExchange); ok {
		return event.Bit, nil
	}
	return true, nil
}

func (r *linkReplay) PollClock() (serial.ClockPulse, bool) {
	if r.err != nil || r.next >= len(r.events) || r.events[r.next].Kind != LinkClock || r.events[r.next].Cycle > r.e.Cycle() {
		return serial.ClockPulse{}, false
	}
	// A due pulse at an earlier cycle means replay polled at a different
	// cycle than the live session did; take reports that desync.
	event, ok := r.take(LinkClock)
	return serial.ClockPulse{Incoming: event.Bit}, ok
}

func (r *linkReplay) ReplyClock(serial.ClockPulse, bool) error { return nil }

func (r *linkReplay) take(kind LinkEventKind) (LinkEvent, bool) {
	cycle := r.e.Cycle()
	if r.err != nil {
		return LinkEvent{}, false
	}
	if r.next >= len(r.events) || r.events[r.next].Kind != kind || r.events[r.next].Cycle != cycle {
		want := "end of link log"
		if r.next < len(r.events) {
			want = fmt.Sprintf("%s at cycle %d", r.events[r.next].Kind, r.events[r.next].Cycle)
		}
		r.err = fmt.Errorf("gomeboy: ReplayRecording: link %s at cycle %d, recording has %s", kind, cycle, want)
		return LinkEvent{}, false
	}
	event := r.events[r.next]
	r.next++
	return event, true
}

// attachDue attaches the replay peer for every LinkAttach recorded at frame.
func (r *linkReplay) attachDue(frame uint64) error {
	for r.next < len(r.events) && r.events[r.next].Kind == LinkAttach && r.events[r.next].Frame == frame {
		if cycle := r.e.Cycle(); cycle != r.events[r.next].Cycle {
			return fmt.Errorf("gomeboy: ReplayRecording: link attached at frame %d cycle %d, replay is at cycle %d", frame, r.events[r.next].Cycle, cycle)
		}
		r.e.gb.Serial.Attach(r)
		r.next++
	}
	return r.err
}

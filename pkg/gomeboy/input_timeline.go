package gomeboy

import "fmt"

// InputEvent is one joypad transition at a frame boundary. Frame is the
// emulator FrameCount before the frame that sees the transition. Cycle is
// recorded for diagnostics; ReplayInputs is intentionally frame-granular and
// uses Frame as the replay clock.
type InputEvent struct {
	Frame   uint64
	Cycle   uint64
	Button  Button
	Pressed bool
}

// StartInputRecording clears the previous input log and starts recording
// subsequent Press and Release calls.
func (e *Emulator) StartInputRecording() {
	e.inputLog = e.inputLog[:0]
	e.inputRecording = true
}

// StopInputRecording stops recording and returns a copy of the recorded input
// timeline. Calling it when recording is already stopped is harmless.
func (e *Emulator) StopInputRecording() []InputEvent {
	e.inputRecording = false
	return e.InputLog()
}

// InputLog returns a copy of the currently recorded input timeline.
func (e *Emulator) InputLog() []InputEvent {
	out := make([]InputEvent, len(e.inputLog))
	copy(out, e.inputLog)
	return out
}

// ClearInputLog discards recorded input events without changing whether
// recording is enabled.
func (e *Emulator) ClearInputLog() {
	e.inputLog = e.inputLog[:0]
}

func (e *Emulator) recordInputEvent(button Button, pressed bool) {
	event := InputEvent{
		Frame:   e.FrameCount(),
		Cycle:   e.Cycle(),
		Button:  button,
		Pressed: pressed,
	}
	if e.inputRecording {
		e.inputLog = append(e.inputLog, event)
	}
	if e.flight != nil {
		e.flight.recordInput(event)
	}
}

// ReplayInputs replays frame-boundary input events from the emulator's
// current FrameCount up to, but not including, untilFrame. An event at frame F
// is applied immediately before frame F is stepped. Events must be sorted by
// Frame and may not refer to a frame that has already passed.
//
// For exact reproduction, restore/reset the emulator to the same starting
// state used when the events were recorded, then pass the same final frame.
func (e *Emulator) ReplayInputs(events []InputEvent, untilFrame uint64) error {
	current := e.FrameCount()
	if untilFrame < current {
		return fmt.Errorf("gomeboy: ReplayInputs: untilFrame %d is before current frame %d", untilFrame, current)
	}
	for i := range events {
		if events[i].Frame < current {
			return fmt.Errorf("gomeboy: ReplayInputs: event %d is in past frame %d (current %d)", i, events[i].Frame, current)
		}
		if i > 0 && events[i].Frame < events[i-1].Frame {
			return fmt.Errorf("gomeboy: ReplayInputs: events are not sorted at index %d", i)
		}
		if events[i].Frame >= untilFrame {
			return fmt.Errorf("gomeboy: ReplayInputs: event %d at frame %d is not before untilFrame %d", i, events[i].Frame, untilFrame)
		}
	}

	i := 0
	for e.FrameCount() < untilFrame {
		frame := e.FrameCount()
		for i < len(events) && events[i].Frame == frame {
			if events[i].Pressed {
				e.Press(events[i].Button)
			} else {
				e.Release(events[i].Button)
			}
			i++
		}
		e.StepFrame()
	}
	return nil
}

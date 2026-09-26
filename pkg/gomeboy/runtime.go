package gomeboy

import (
	"github.com/maestroi/gomeboy/pkg/emulator"
)

const defaultAudioSampleRate uint64 = 96000

// Pause marks realtime frontend execution paused. Explicit StepFrame/StepFrames
// calls remain deterministic and continue to step, matching the existing
// library semantics.
func (e *Emulator) Pause() {
	if e != nil {
		e.paused = true
	}
}

func (e *Emulator) Resume() {
	if e != nil {
		e.paused = false
	}
}

func (e *Emulator) Paused() bool {
	return e != nil && e.paused
}

func (e *Emulator) Initialised() bool {
	return e != nil && e.initialised
}

func (e *Emulator) SetSpeed(speed int) {
	if e == nil {
		return
	}
	if speed < 1 {
		speed = 1
	}
	if speed > 8 {
		speed = 8
	}
	e.speed = speed
}

func (e *Emulator) Speed() int {
	if e == nil || e.speed < 1 {
		return 1
	}
	return e.speed
}

// Samples drains interleaved stereo float32 samples from the active core.
func (e *Emulator) Samples() ([]float32, uint32) {
	if e == nil || e.core == nil {
		return nil, 0
	}
	if audio, ok := e.core.(interface{ Samples() ([]float32, uint32) }); ok {
		return audio.Samples()
	}
	return nil, 0
}

// AudioSampleRate returns the active core's host output rate.
func (e *Emulator) AudioSampleRate() uint64 {
	if e == nil || e.core == nil {
		return defaultAudioSampleRate
	}
	if audio, ok := e.core.(interface{ AudioSampleRate() uint64 }); ok {
		return audio.AudioSampleRate()
	}
	return defaultAudioSampleRate
}

// ToggleMute toggles host output without stopping emulated audio hardware.
func (e *Emulator) ToggleMute() bool {
	if e == nil || e.core == nil {
		return false
	}
	if audio, ok := e.core.(interface{ ToggleMute() bool }); ok {
		return audio.ToggleMute()
	}
	return false
}

// FrameSize reports the active core's framebuffer dimensions.
func (e *Emulator) FrameSize() (width, height int) {
	if e != nil && e.core != nil && e.core.CoreID() == "gba" {
		return 240, 160
	}
	return 160, 144
}

// PressButton implements emulator.InputController for desktop frontends.
func (e *Emulator) PressButton(button emulator.Button) {
	if mapped, ok := publicButton(button); ok {
		e.Press(mapped)
	}
}

// ReleaseButton implements emulator.InputController for desktop frontends.
func (e *Emulator) ReleaseButton(button emulator.Button) {
	if mapped, ok := publicButton(button); ok {
		e.Release(mapped)
	}
}

func publicButton(button emulator.Button) (Button, bool) {
	switch button {
	case emulator.ButtonA:
		return ButtonA, true
	case emulator.ButtonB:
		return ButtonB, true
	case emulator.ButtonStart:
		return ButtonStart, true
	case emulator.ButtonSelect:
		return ButtonSelect, true
	case emulator.ButtonUp:
		return ButtonUp, true
	case emulator.ButtonDown:
		return ButtonDown, true
	case emulator.ButtonLeft:
		return ButtonLeft, true
	case emulator.ButtonRight:
		return ButtonRight, true
	case emulator.ButtonL:
		return ButtonL, true
	case emulator.ButtonR:
		return ButtonR, true
	default:
		return 0, false
	}
}

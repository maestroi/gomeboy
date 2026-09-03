// Package webbridge bridges a step-driven gomeboy.Emulator into the
// display layer's push/channel model. The Adapter satisfies
// emulator.Controller but never advances the emulator itself: the agent
// loop owns timing and calls PublishFrame after it has stepped.
package webbridge

import (
	"sync/atomic"

	"github.com/maestroi/gomeboy/pkg/emulator"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Emulator is the minimal surface webbridge needs. pkg/gomeboy.Emulator
// satisfies it; tests use a fake instead of a real ROM.
type Emulator interface {
	Frame() gomeboy.Frame
	LoadROM(string) error
	QuickSave() error
	QuickLoad() error
}

// Adapter satisfies emulator.Controller for an Emulator. It never calls
// StepFrame/StepFrames; Pause/Resume are advisory only (the agent loop
// must check Paused() itself before advancing the emulator).
type Adapter struct {
	emu         Emulator
	fb          chan<- []byte
	paused      atomic.Bool
	initialised atomic.Bool
	speed       atomic.Int64
}

// NewAdapter creates an Adapter bridging emu's frames onto fb.
func NewAdapter(emu Emulator, fb chan<- []byte) *Adapter {
	a := &Adapter{emu: emu, fb: fb}
	a.speed.Store(1)
	return a
}

var _ emulator.Controller = (*Adapter)(nil)

// LoadROM delegates to the emulator and tracks initialisation. A failed
// load leaves the adapter reporting not initialised.
func (a *Adapter) LoadROM(path string) error {
	err := a.emu.LoadROM(path)
	a.initialised.Store(err == nil)
	return err
}

// Pause marks the adapter as paused. Advisory only: it flips a flag and
// does not sleep or block, and does not by itself prevent the emulator
// from advancing.
func (a *Adapter) Pause() {
	a.paused.Store(true)
}

// Resume clears the pause flag.
func (a *Adapter) Resume() {
	a.paused.Store(false)
}

// Paused reports whether the adapter is paused.
func (a *Adapter) Paused() bool {
	return a.paused.Load()
}

// Initialised reports whether a ROM has been loaded successfully.
func (a *Adapter) Initialised() bool {
	return a.initialised.Load()
}

// QuickSave delegates to the emulator.
func (a *Adapter) QuickSave() error {
	return a.emu.QuickSave()
}

// QuickLoad delegates to the emulator.
func (a *Adapter) QuickLoad() error {
	return a.emu.QuickLoad()
}

// SetSpeed sets the adapter's speed factor.
func (a *Adapter) SetSpeed(n int) {
	a.speed.Store(int64(n))
}

// Speed returns the adapter's speed factor.
func (a *Adapter) Speed() int {
	return int(a.speed.Load())
}

// PublishFrame pushes a copy of the emulator's current frame onto the
// display hub's frame buffer channel. It is a no-op (frame dropped) if the
// adapter is paused or the channel is full; it never blocks the caller.
func (a *Adapter) PublishFrame() {
	if a.paused.Load() {
		return
	}
	f := a.emu.Frame()
	// Frame().RGB is a zero-copy view invalidated by the next step, so
	// copy it before the (asynchronous) channel send.
	data := make([]byte, len(f.RGB))
	copy(data, f.RGB)
	select {
	case a.fb <- data:
	default:
		// channel full: drop the frame rather than block the agent loop
	}
}

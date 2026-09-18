package gomeboy

import (
	"errors"
	"fmt"
)

// Checkpoint is an opaque reusable in-memory emulator checkpoint. Its zero
// value is ready to use as a CheckpointInto destination. Unlike SaveState, it
// is not a stable or portable serialized format.
//
// Checkpoint storage belongs to the concrete emulator core that created it.
// Keeping the payload opaque lets GB/GBC and future cores use different native
// state types without leaking those types through the public API.
type Checkpoint struct {
	coreID string
	state  any
	valid  bool
}

// CheckpointInto captures the emulator into dst. Reusing the same Checkpoint
// avoids serialization and reuses variable-sized snapshot buffers after their
// first allocation.
func (e *Emulator) CheckpointInto(dst *Checkpoint) {
	if dst == nil {
		panic("gomeboy: nil checkpoint destination")
	}
	if e == nil || e.core == nil {
		panic("gomeboy: emulator core is not initialized")
	}

	coreID := e.core.CoreID()
	if dst.state == nil || dst.coreID != coreID {
		dst.state = e.core.NewCheckpoint()
		dst.coreID = coreID
	}
	e.core.CheckpointInto(dst.state)
	dst.valid = true
}

// RestoreCheckpoint restores a checkpoint captured from an emulator running
// the same core and ROM.
func (e *Emulator) RestoreCheckpoint(src *Checkpoint) error {
	if src == nil || !src.valid {
		return errors.New("gomeboy: checkpoint is not initialized")
	}
	if e == nil || e.core == nil {
		return errors.New("gomeboy: emulator core is not initialized")
	}
	if src.coreID != e.core.CoreID() {
		return fmt.Errorf("gomeboy: checkpoint core %q does not match emulator core %q", src.coreID, e.core.CoreID())
	}
	return e.core.RestoreCheckpoint(src.state)
}

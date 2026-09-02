package gomeboy

import (
	"errors"

	"github.com/thelolagemann/gomeboy/internal/gameboy"
)

// Checkpoint is an opaque reusable in-memory emulator checkpoint. Its zero
// value is ready to use as a CheckpointInto destination. Unlike SaveState, it
// is not a stable or portable serialized format.
type Checkpoint struct {
	state gameboy.State
	valid bool
}

// CheckpointInto captures the emulator into dst. Reusing the same Checkpoint
// avoids serialization and reuses variable-sized snapshot buffers after their
// first allocation.
func (e *Emulator) CheckpointInto(dst *Checkpoint) {
	if dst == nil {
		panic("gomeboy: nil checkpoint destination")
	}
	e.gb.CheckpointInto(&dst.state)
	dst.valid = true
}

// RestoreCheckpoint restores a checkpoint captured from an emulator running
// the same ROM.
func (e *Emulator) RestoreCheckpoint(src *Checkpoint) error {
	if src == nil || !src.valid {
		return errors.New("gomeboy: checkpoint is not initialized")
	}
	e.gb.Restore(src.state)
	return nil
}

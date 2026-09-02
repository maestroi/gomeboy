package gomeboy

import "github.com/thelolagemann/gomeboy/internal/gameboy"

// Checkpoint is an opaque in-memory emulator snapshot intended for fast
// branching/search workloads. It is process-local: use SaveState for portable
// serialized state.
type Checkpoint struct {
	state gameboy.State
}

// CheckpointInto captures the current execution state into cp. Reusing the same
// Checkpoint avoids serialized save-state encoding and its byte-buffer churn.
func (e *Emulator) CheckpointInto(cp *Checkpoint) {
	if cp == nil {
		return
	}
	cp.state = e.gb.Snapshot()
}

// RestoreCheckpoint restores a checkpoint previously captured from an emulator
// running the same ROM.
func (e *Emulator) RestoreCheckpoint(cp *Checkpoint) {
	if cp == nil {
		return
	}
	e.gb.Restore(cp.state)
}

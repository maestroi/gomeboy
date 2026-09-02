package gameboy

// CheckpointInto captures the complete execution state into dst while reusing
// mutable snapshot storage where possible. It is intended for high-frequency
// in-process branching; SaveState remains the portable serialized format.
func (g *GameBoy) CheckpointInto(dst *State) {
	g.mu.Lock()
	defer g.mu.Unlock()
	dst.CPU = g.CPU.Snapshot()
	dst.Scheduler = g.Scheduler.Snapshot()
	g.Bus.SnapshotInto(&dst.Bus)
	g.PPU.SnapshotInto(&dst.PPU)
	dst.APU = g.APU.Snapshot()
	dst.Timer = g.Timer.Snapshot()
	dst.Serial = g.Serial.Snapshot()
	dst.Model = g.model
	dst.Frames = g.frames
}

package ppu

// cpuOAMStartupWriteOpen models the CPU-visible OAM write port on the
// shortened first LCD-enabled scanline. This special scanline is not
// equivalent to normal Mode 2: its PPU write lock is acquired at dot 76,
// but the double-speed CPU can still finish a write on the first dot of
// that handover.
//
// This does not advance the PPU's own OAM lock, move scheduler events, or
// relax DMA restrictions. It applies only to colour CGB double speed
// during the transition from the startup OAM write lock to Mode 3.
func (p *PPU) cpuOAMStartupWriteOpen(defaultOpen bool) bool {
	if defaultOpen || !p.enabled || !p.cgbMode || !p.b.Model().IsCGB() ||
		!p.s.DoubleSpeed() || p.glitchedLineState != GlitchedLineEndOAM {
		return defaultOpen
	}
	if p.s.Cycle() < p.lineDot {
		return defaultOpen
	}
	elapsed := p.s.Cycle() - p.lineDot
	// At double speed the PPU reaches startup dot 76 after 152 CPU
	// clocks. Allow the CPU write for one additional PPU dot (2 clocks).
	if elapsed >= 152 && elapsed < 154 {
		return true
	}
	return defaultOpen
}

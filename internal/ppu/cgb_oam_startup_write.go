package ppu

// cpuOAMWriteOpen models the CPU-facing OAM write port at the physical
// CGB's hardware handovers, independent of CPU reads and PPU scanning.
//
// The native-colour CPU may write after the last pixel before the coarse
// HBlank unlock; at double speed it then closes its write port early at
// HBlankEnd before Mode 2 acquires the PPU's OAM bus. The abbreviated
// first LCD-on line has its own dot-76 startup write grace.
//
// These gates do not shift PPU/STAT events or change DMA arbitration.
// DMG and CGB compatibility mode preserve their previous write phases.
func (p *PPU) cpuOAMWriteOpen(defaultOpen bool) bool {
	if !p.enabled {
		return defaultOpen
	}
	if defaultOpen {
		// Physical CGB at double speed reserves the CPU OAM write
		// port at the end-of-HBlank / next-Mode-2 handover, before
		// the PPU's next scheduled OAM scan acquires its coarse lock.
		// This is a CPU write-port phase: CPU reads, VRAM, DMA and the
		// PPU's scanning schedule remain independent.
		if p.cgbMode && p.b.Model().IsCGB() && p.s.DoubleSpeed() &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			(p.lineState == HBlankEnd ||
				p.lineState == StartOAMScan && p.oamScanIndex == 0) {
			return false
		}
		return true
	}

	// On physical CGB in native-colour mode, the completed final
	// visible pixel can release CPU OAM writes before the scheduled
	// HBlank event unlocks the shared PPU OAM bus. At normal speed
	// this is observable at SCX residues 2,3,6,7; the double-speed
	// CPU samples even fine-scroll phases. CPU OAM reads, DMA and
	// compatibility-mode fetchers follow their own established phases.
	if p.cgbMode && p.b.Model().IsCGB() &&
		p.lineState == EnterHBlank && p.lx == 168 &&
		(!p.s.DoubleSpeed() && p.scx&2 != 0 ||
			p.s.DoubleSpeed() && p.scx&1 == 0) {
		return true
	}

	if !p.cgbMode || !p.b.Model().IsCGB() ||
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

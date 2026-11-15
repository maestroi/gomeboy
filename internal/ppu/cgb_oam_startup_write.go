package ppu

import "github.com/maestroi/gomeboy/internal/types"

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
		// On the abbreviated first LCD-on line, the physical CGB
		// compatibility-mode CPU OAM write port may remain locked
		// after the coarse HBlank bus releases OAM and VRAM. The
		// absolute release clock is captured from the real event.
		if !p.cgbMode && p.b.Model().IsCGB() && !p.s.DoubleSpeed() &&
			p.s.Cycle() < p.startupOAMWriteOpenAt {
			return false
		}
		// The physical CGB write port is reserved for the upcoming
		// Mode-2 OAM scan before the PPU takes its coarse bus lock.
		// Native CGB colour does this in double speed; a DMG-only
		// cartridge uses the CGB CPU's write timing at normal speed.
		// CPU OAM reads, VRAM, DMA, and PPU scan timing are unchanged.
		if (p.b.Model().IsCGB() &&
			(p.cgbMode && p.s.DoubleSpeed() ||
				!p.cgbMode && !p.s.DoubleSpeed()) ||
			p.b.Model() == types.DMGABC && !p.s.DoubleSpeed()) &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			(p.lineState == HBlankEnd ||
				p.lineState == StartOAMScan && p.oamScanIndex == 0) {
			return false
		}
		return true
	}

	// A completed final visible pixel can release CPU OAM writes
	// before the scheduled HBlank event unlocks the coarse OAM bus.
	// Native CGB colour normal speed uses SCX 2/3/6/7; double speed
	// uses even SCX phases. A DMG-compatible cartridge on physical
	// CGB instead uses its own SCX 3/7 write-only handover phase.
	// CPU OAM reads, DMA and the PPU pixel schedule remain separate.
	if p.b.Model().IsCGB() && p.lineState == EnterHBlank && p.lx == 168 &&
		(p.cgbMode && (!p.s.DoubleSpeed() && p.scx&2 != 0 ||
			p.s.DoubleSpeed() && p.scx&1 == 0) ||
			!p.cgbMode && !p.s.DoubleSpeed() && p.scx&3 == 3) {
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

package ppu

import "github.com/maestroi/gomeboy/internal/types"

// cpuOAMReadOpen refines the CPU-facing OAM read port at two distinct
// hardware handovers. CPU writes, PPU OAM scanning and DMA remain governed
// by their separate coarse bus locks.
//
// On CGB D/E in monochrome compatibility mode, the *first* LCD-enabled
// scanline holds the OAM read port for two PPU dots after the HBlank event
// releases VRAM and OAM writes. B/C hardware does not share this delay.
// The release cycle is captured at HBlank, not derived from a later SCX
// register value (which can be changed independently).
//
// DMG-C has a different short Mode-3 -> HBlank read handover: when SCX is
// congruent to 3 modulo four, CPU OAM reads become accessible after the
// final pixel is output but before the PPU's scheduled HBlank bus unlock.
func (p *PPU) cpuOAMReadOpen(defaultOpen bool) bool {
	if !p.enabled {
		return defaultOpen
	}
	if defaultOpen {
		if p.b.Model() == types.CGBDE && !p.cgbMode && !p.s.DoubleSpeed() &&
			p.s.Cycle() < p.startupOAMReadOpenAt {
			return false
		}
		return true
	}
	return p.b.Model() == types.DMGABC &&
		p.scx&3 == 3 && p.lineState == EnterHBlank && p.lx == 168
}

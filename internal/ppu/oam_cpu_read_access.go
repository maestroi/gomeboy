package ppu

import "github.com/maestroi/gomeboy/internal/types"

// cpuOAMReadOpen models CPU OAM read ownership independently of OAM writes,
// PPU OAM scanning and DMA. Physical CGB revisions differ even in DMG
// compatibility mode.
//
// On the abbreviated first LCD-enabled scanline, CGB B/C keeps the CPU
// OAM read port locked one PPU dot past the normal HBlank bus release;
// CGB D/E keeps it locked for two. The release cycle is captured at the
// actual HBlank event and is not affected by later SCX writes.
//
// At the end of an ordinary scanline, the CPU OAM read port on DMG-C and
// CGB B/C in DMG compatibility mode reopens just after the final visible
// pixel at SCX residues 3 and 7, before the general OAM write unlock.
// CGB D/E, CGB colour mode and other monochrome models retain their
// independent measured access timing.
func (p *PPU) cpuOAMReadOpen(defaultOpen bool) bool {
	if !p.enabled {
		return defaultOpen
	}
	if defaultOpen {
		model := p.b.Model()
		if (model == types.CGBBC || model == types.CGBDE) &&
			!p.cgbMode && !p.s.DoubleSpeed() &&
			p.s.Cycle() < p.startupOAMReadOpenAt {
			return false
		}
		return true
	}

	model := p.b.Model()
	lastPixelRead := model == types.DMGABC ||
		model == types.CGBBC && !p.cgbMode && !p.s.DoubleSpeed()
	return lastPixelRead && p.scx&3 == 3 &&
		p.lineState == EnterHBlank && p.lx == 168
}

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
// CGB B/C colour mode shares the completed-pixel read-only handover at
// normal speed. At double speed the CPU also sees the buffered final pixel
// during an earlier phase and can read the startup Mode-3 edge before the
// PPU's coarse OAM read lock catches up. CGB D/E and other monochrome
// models retain their independent measured access timing.
func (p *PPU) cpuOAMReadOpen(defaultOpen bool) bool {
	if !p.enabled {
		return defaultOpen
	}
	model := p.b.Model()
	if defaultOpen {
		if (model == types.CGBBC || model == types.CGBDE) &&
			!p.cgbMode && !p.s.DoubleSpeed() &&
			p.s.Cycle() < p.startupOAMReadOpenAt {
			return false
		}
		return true
	}

	// The colour CGB B/C CPU samples the initial LCD-on Mode-3 read
	// bus a PPU dot after its coarse read lock activates at dot 78.
	// Keep the OAM write lock and all later Mode-3 reads untouched.
	if model == types.CGBBC && p.cgbMode && p.s.DoubleSpeed() &&
		p.glitchedLineState == GlitchedLineVRAMReadLock &&
		p.s.Cycle() >= p.lineDot {
		elapsed := p.s.Cycle() - p.lineDot
		if elapsed >= 156 && elapsed < 158 {
			return true
		}
	}

	if p.scx&3 == 3 && p.lineState == EnterHBlank && p.lx == 168 {
		// Normal-speed B/C reopens the OAM read bus after its last
		// pixel, regardless of whether the cartridge is CGB-capable.
		if model == types.DMGABC || model == types.CGBBC && !p.s.DoubleSpeed() {
			return true
		}
	}

	// Double-speed B/C has an earlier pixel-buffer handover than the
	// completed-pixel phase above; no OAM writes or other models open.
	return model == types.CGBBC && p.cgbMode && p.s.DoubleSpeed() &&
		p.scx&1 == 0 && p.lineState == PixelTransferLX &&
		p.lx == 167 && p.bgFIFO.Size > 0 && !p.fetcherObj
}

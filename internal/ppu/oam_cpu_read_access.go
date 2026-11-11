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
// Native CGB colour mode has a separate double-speed CPU-facing read
// phase at the first LCD-on Mode-3 edge; B/C and D/E use the same early
// read phase but D/E keeps the first-line read lock two dots into HBlank.
// The double-speed CPU can also sample the buffered and completed final
// pixel before the OAM write lock opens. Other monochrome models retain
// their independent measured access timing.
func (p *PPU) cpuOAMReadOpen(defaultOpen bool) bool {
	if !p.enabled {
		return defaultOpen
	}
	model := p.b.Model()
	if defaultOpen {
		if !p.s.DoubleSpeed() && p.s.Cycle() < p.startupOAMReadOpenAt &&
			(model == types.CGBDE || model == types.CGBBC && !p.cgbMode) {
			return false
		}
		// On native-colour CGB D/E at double speed, the CPU read
		// port is reserved for Mode 2 a clock before the scheduled
		// scan acquires the coarse OAM bus. The write port and PPU
		// scan retain their own existing lock phases. This state
		// is entered at HBlankEnd on real lines (and frame rollover),
		// never on the abbreviated LCD-enable line.
		if model == types.CGBDE && p.cgbMode && p.s.DoubleSpeed() &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			p.lineState == StartOAMScan && p.oamScanIndex == 0 {
			return false
		}
		return true
	}

	// Physical CGB colour CPU reads can complete during the first
	// two double-speed CPU clocks at the dot-78 OAM lock handover.
	// Keep OAM writes and later Mode-3 reads under their original lock.
	if (model == types.CGBBC || model == types.CGBDE) &&
		p.cgbMode && p.s.DoubleSpeed() &&
		p.glitchedLineState == GlitchedLineVRAMReadLock &&
		p.s.Cycle() >= p.lineDot {
		elapsed := p.s.Cycle() - p.lineDot
		if elapsed >= 156 && elapsed < 158 {
			return true
		}
	}

	if p.lineState == EnterHBlank && p.lx == 168 {
		// At normal speed, B/C reopens the CPU OAM read port once
		// the last pixel is pushed on SCX=3/7. The same handover is
		// sampled at even fine-scroll offsets in native double speed.
		if model == types.DMGABC && p.scx&3 == 3 ||
			model == types.CGBBC && !p.s.DoubleSpeed() && p.scx&3 == 3 ||
			(model == types.CGBBC || model == types.CGBDE) &&
				p.cgbMode && p.s.DoubleSpeed() && p.scx&1 == 0 {
			return true
		}
	}

	// Native CGB at double speed also has a buffered-pixel read
	// handover; neither it nor the completed-pixel edge unlocks writes.
	return (model == types.CGBBC || model == types.CGBDE) &&
		p.cgbMode && p.s.DoubleSpeed() && p.scx&1 == 0 &&
		p.lineState == PixelTransferLX &&
		p.lx == 167 && p.bgFIFO.Size > 0 && !p.fetcherObj
}

package ppu

import "github.com/maestroi/gomeboy/internal/types"

// cpuVRAMReadOpenAtLastPixel refines the CPU read side of CGB VRAM ownership
// during the very short handover from pixel transfer to HBlank.
//
// The renderer can finish pushing its 168th pixel before the scheduled
// EnterHBlank event changes STAT and releases the shared PPU bus locks.
// CPU reads can see the completed transfer at this boundary, while the
// PPU's internal bus, CPU VRAM writes, and OAM stay on their own timelines.
//
// The shortened LCD-enable scanline skips pixel generation and never
// reaches lx=168, so it retains its dedicated startup access events.
func (p *PPU) cpuVRAMReadOpenAtLastPixel(defaultOpen bool) bool {
	// The CPU VRAM port belongs to the physical CGB hardware even
	// when a DMG-only cartridge is running in compatibility mode.
	// DMG/MGB hardware retains its original HBlank ownership edge.
	if defaultOpen || !p.enabled {
		return defaultOpen
	}
	// DMG-C's read port can become available once its last visible
	// pixel has entered the BG FIFO, even before the PPU updates the
	// ordinary HBlank bus locks. Do not generalize this measurement to
	// MGB/SGB hardware, whose access-phase conformance is independent.
	if p.b.Model() == types.DMGABC {
		// DMG-C has a different CPU/PPU read phase for fine-scroll
		// residues 3 and 7. Earlier access at other SCX offsets
		// breaks the LCD-enable ownership window (Mooneye).
		return p.scx&3 == 3 && p.lineState == PixelTransferLX &&
			p.lx == 167 && p.bgFIFO.Size > 0 && !p.fetcherObj
	}
	if !p.b.Model().IsCGB() {
		return defaultOpen
	}
	if p.lineState == EnterHBlank && p.lx == 168 {
		return true
	}
	// The monochrome fetcher used on CGB hardware can have its final
	// visible pixel already buffered while LX still points at 167.
	// Once a BG pixel is ready and no OBJ fetch is in flight, the CPU
	// read port may be released without unlocking VRAM writes or the
	// PPU's internal VRAM bus. CGB-colour keeps its measured last-pixel
	// release edge.
	return !p.cgbMode && p.lineState == PixelTransferLX &&
		p.lx == 167 && p.bgFIFO.Size > 0 && !p.fetcherObj
}

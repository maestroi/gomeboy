package ppu

// cpuVRAMReadOpen refines only the leading edge of a *normal* CGB Mode-3
// transfer. The physical CGB revision matters even when a DMG-only cartridge
// is running in compatibility mode. AGE's pinned vram-read timing matrix
// observes the CPU read bus remaining free briefly after STAT enters Mode 3.
//
// Keep the startup (glitched) scanline and every other bus-access boundary on
// the existing coarse-lock path. In particular, HBlank release, CPU VRAM
// writes, and all OAM accesses are deliberately outside this change.
func (p *PPU) cpuVRAMReadOpen(defaultOpen bool) bool {
	if !p.enabled || !p.b.Model().IsCGB() || p.mode != ModeVRAM {
		return defaultOpen
	}

	// The first line after enabling LCDC is shortened and uses special
	// startup ownership transitions. Even if a normal pixel-transfer state
	// becomes visible briefly, don't apply the ordinary Mode-3 grace to it.
	// frameDot is reset on every LCD enable, so later frame-0 lines (LY=0)
	// are not accidentally classified as startup.
	firstLineCycles := uint64(452)
	if p.s.DoubleSpeed() {
		firstLineCycles *= 2
	}
	if p.ly == 0 && p.s.Cycle() >= p.frameDot &&
		p.s.Cycle()-p.frameDot < firstLineCycles {
		return defaultOpen
	}
	if p.lineState != PixelTransferDummy && p.lineState != PixelTransferLX {
		return defaultOpen
	}

	if p.s.Cycle() < p.lineDot {
		return defaultOpen
	}
	dots := p.s.Cycle() - p.lineDot
	if p.s.DoubleSpeed() {
		dots >>= 1
	}

	// Normal lines enter Mode 3 at dot 80. A CGB read still completes
	// during the Mode-3 entry grace at normal speed or two PPU dots at double
	// speed; it subsequently sees the existing VRAM read lock.
	closeDot := uint64(85)
	if p.s.DoubleSpeed() {
		closeDot = 82
	}
	if dots >= 80 && dots < closeDot {
		return true
	}
	return defaultOpen
}

package ppu

// cpuOAMWriteHBlankOpen refines the CPU-facing OAM *write* port after
// Mode 3 has finished. The internal PPU bus, the OAM CPU read port, and
// STAT mode transitions continue to use the existing HBlank event.
//
// On CGB-colour hardware at double speed, a write completing on the
// first CPU T-state after HBlank begins still sees OAM as occupied.
// This short settle delay does not shift the full 456-dot scanline.
func (p *PPU) cpuOAMWriteHBlankOpen(defaultOpen bool) bool {
	if !defaultOpen || !p.enabled || !p.cgbMode || !p.s.DoubleSpeed() ||
		p.mode != ModeHBlank || p.lineState != HBlankUpdateLY {
		return defaultOpen
	}
	return p.s.Cycle() >= p.oamCPUWriteReadyAt
}

func (p *PPU) startOAMWriteHBlankRelease() {
	if p.cgbMode && p.s.DoubleSpeed() {
		// One PPU dot corresponds to two CPU cycles in double speed.
		p.oamCPUWriteReadyAt = p.s.Cycle() + 2
	} else {
		p.oamCPUWriteReadyAt = p.s.Cycle()
	}
}

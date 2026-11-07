package ppu

// cpuOAMReadOpenAtLastPixel models the CPU read-side OAM bus handover
// after the final visible pixel has entered the output FIFO.
//
// The PPU may still hold the coarse OAM lock until the scheduled HBlank
// event. A CPU OAM read can see the completed transfer before that event,
// while OAM writes, VRAM writes, DMA arbitration, and STAT mode transitions
// retain their existing separate ownership.
func (p *PPU) cpuOAMReadOpenAtLastPixel(defaultOpen bool) bool {
	if defaultOpen || !p.enabled {
		return defaultOpen
	}
	// Opening OAM at LX=167 would expose the bus during the final
	// object/pixel fetch and breaks existing OAM lock regression tests.
	// The CPU read port may reopen only after that last pixel is pushed,
	// while the scheduled PPU HBlank event still owns coarse bus locks.
	return p.lineState == EnterHBlank && p.lx == 168
}

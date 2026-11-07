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
	return p.lineState == PixelTransferLX && p.lx == 167 &&
		p.bgFIFO.Size > 0 && !p.fetcherObj
}

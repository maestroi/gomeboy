package ppu

// SetVideoOutput controls RGB framebuffer composition. Disabling it does not
// skip PPU timing, FIFO work, interrupts, bus locks, or DMA.
func (p *PPU) SetVideoOutput(enabled bool) {
	p.videoOutput = enabled
}

// VideoOutputEnabled reports whether RGB framebuffer composition is enabled.
func (p *PPU) VideoOutputEnabled() bool {
	return p.videoOutput
}

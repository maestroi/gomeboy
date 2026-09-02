package ppu

// SetVideoOutput controls RGB framebuffer generation. Disabling output keeps
// the complete PPU timing/fetch pipeline running, but skips palette composition
// and writes to PreparedFrame.
func (p *PPU) SetVideoOutput(enabled bool) {
	p.videoOutput = enabled
}

// VideoOutputEnabled reports whether RGB framebuffer generation is enabled.
func (p *PPU) VideoOutputEnabled() bool {
	return p.videoOutput
}

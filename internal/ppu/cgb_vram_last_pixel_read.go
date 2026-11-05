package ppu

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
	if defaultOpen || !p.enabled || !p.cgbMode {
		return defaultOpen
	}
	return p.lineState == EnterHBlank && p.lx == 168
}

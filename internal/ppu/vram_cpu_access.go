package ppu

// cpuVRAMHBlankReadOpen models the short delay between releasing the PPU's
// internal VRAM bus at HBlank entry and the CPU being able to read it. This
// does not affect VRAM writes, DMA arbitration, or the PPU's own fetches.
//
// Only the normal visible-line HBlank release is refined. The shortened
// LCD-enable line and VBlank retain their pre-existing read ownership.
func (p *PPU) cpuVRAMHBlankReadOpen(defaultOpen bool) bool {
	if !defaultOpen || !p.enabled || !p.b.Model().IsCGB() ||
		p.mode != ModeHBlank || p.lineState != HBlankUpdateLY {
		return defaultOpen
	}
	return p.s.Cycle() >= p.vramCPUReadReadyAt
}

// At normal speed, the read port becomes available one PPU dot after the
// HBlank entry event. Double-speed mode still runs the PPU at its original
// dot rate, so that delay occupies two CPU clock cycles.
func (p *PPU) scheduleVRAMCPUReadRelease() {
	delay := uint64(1)
	if p.s.DoubleSpeed() {
		delay = 2
	}
	p.vramCPUReadReadyAt = p.s.Cycle() + delay
}

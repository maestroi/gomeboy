package gomeboy

// Peek8 reads a byte without any hardware side effects. Unlike Read8 it
// ignores PPU region locks, DMA conflicts and IO lazy readers, so it always
// returns the byte actually held in the mapped address space. Use Peek for
// observation; use Read for CPU-accurate reads.
func (e *Emulator) Peek8(addr uint16) byte {
	return e.gb.Bus.Get(addr)
}

// Peek16 reads a little-endian 16-bit value without side effects.
func (e *Emulator) Peek16(addr uint16) uint16 {
	return uint16(e.Peek8(addr)) | uint16(e.Peek8(addr+1))<<8
}

// PeekInto fills dst with len(dst) bytes starting at addr, without side
// effects and without allocating.
func (e *Emulator) PeekInto(addr uint16, dst []byte) {
	if len(dst) == 0 {
		return
	}

	// Most agent observations are contiguous WRAM/HRAM ranges. Use the
	// runtime-optimized bulk copy when the range does not wrap the 16-bit bus.
	if end := int(addr) + len(dst); end <= 0xffff {
		e.gb.Bus.CopyFrom(addr, uint16(end), dst)
		return
	}

	// Preserve the historical uint16 wraparound semantics for boundary-crossing
	// or unusually large reads.
	for i := range dst {
		dst[i] = e.gb.Bus.Get(addr + uint16(i))
	}
}

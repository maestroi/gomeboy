package ppu

func (p *PPU) installRegisters() {
	io := p.bus.IO()

	io.Register16(dispCNTOffset,
		func() uint16 { return p.dispcnt },
		func(value uint16) { p.writeDISPCNT(value) },
	)
	io.Register16(dispSTATOffset,
		func() uint16 { return p.readDISPSTAT() },
		func(value uint16) { p.writeDISPSTAT(value) },
	)
	io.Register16(vcountOffset,
		func() uint16 { return p.vcount },
		nil,
	)
}

func (p *PPU) writeDISPCNT(value uint16) {
	// Bit 3 is the BIOS-only CGB-mode bit and is not writable as an ordinary
	// native-GBA display-control bit.
	p.dispcnt = value &^ (1 << 3)

	// Bitmap modes reserve more VRAM for BG2, moving OBJ tile data upward.
	switch p.dispcnt & 0x7 {
	case 3, 4, 5:
		p.bus.SetOBJVRAMStart(0x14000)
	default:
		p.bus.SetOBJVRAMStart(0x10000)
	}
}

func (p *PPU) readDISPSTAT() uint16 {
	value := p.dispstat & 0xff38
	if p.vblank {
		value |= 1 << 0
	}
	if p.hblank {
		value |= 1 << 1
	}
	if p.vcounter {
		value |= 1 << 2
	}
	return value
}

func (p *PPU) writeDISPSTAT(value uint16) {
	// Status bits 0-2 and unused bits 6-7 are read-only/zero on GBA.
	p.dispstat = value & 0xff38
	p.updateVCountMatch(true)
}

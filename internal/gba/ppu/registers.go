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

	for bg := 0; bg < 4; bg++ {
		bg := bg
		controlOffset := uint32(0x008 + bg*2)
		hofsOffset := uint32(0x010 + bg*4)
		vofsOffset := hofsOffset + 2

		controlMask := uint16(0xffcf)
		if bg < 2 {
			// BG0/BG1 bit 13 is unused on GBA. BG2/BG3 use it as the
			// affine display-area overflow bit.
			controlMask &^= 1 << 13
		}
		io.Register16(controlOffset,
			func() uint16 { return p.bgcnt[bg] },
			func(value uint16) { p.bgcnt[bg] = value & controlMask },
		)
		io.Register16(hofsOffset,
			func() uint16 { return p.openBusHalfword(hofsOffset) },
			func(value uint16) { p.bghofs[bg] = value & 0x01ff },
		)
		io.Register16(vofsOffset,
			func() uint16 { return p.openBusHalfword(vofsOffset) },
			func(value uint16) { p.bgvofs[bg] = value & 0x01ff },
		)
	}

	for window := 0; window < 2; window++ {
		window := window
		hOffset := uint32(0x040 + window*2)
		vOffset := uint32(0x044 + window*2)
		io.Register16(hOffset,
			func() uint16 { return p.openBusHalfword(hOffset) },
			func(value uint16) { p.winH[window] = value },
		)
		io.Register16(vOffset,
			func() uint16 { return p.openBusHalfword(vOffset) },
			func(value uint16) { p.winV[window] = value },
		)
	}
	io.Register16(0x048,
		func() uint16 { return p.winIn },
		func(value uint16) { p.winIn = value & 0x3f3f },
	)
	io.Register16(0x04a,
		func() uint16 { return p.winOut },
		func(value uint16) { p.winOut = value & 0x3f3f },
	)

	io.Register16(0x050,
		func() uint16 { return p.bldcnt },
		func(value uint16) { p.bldcnt = value & 0x3fff },
	)
	io.Register16(0x052,
		func() uint16 { return p.bldalpha },
		func(value uint16) { p.bldalpha = value & 0x1f1f },
	)
	io.Register16(0x054,
		func() uint16 { return p.openBusHalfword(0x054) },
		func(value uint16) { p.bldy = value & 0x001f },
	)

	for affine := 0; affine < 2; affine++ {
		affine := affine
		base := uint32(0x020 + affine*0x10)

		for param := 0; param < 4; param++ {
			param := param
			offset := base + uint32(param*2)
			io.Register16(offset,
				func() uint16 { return p.affineOpenBusHalfword(offset) },
				func(value uint16) { p.writeAffineParam(affine, param, value) },
			)
		}

		for axis := 0; axis < 2; axis++ {
			axis := axis
			lowOffset := base + 0x08 + uint32(axis*4)
			highOffset := lowOffset + 2
			io.Register16(lowOffset,
				func() uint16 { return p.affineOpenBusHalfword(lowOffset) },
				func(value uint16) { p.writeAffineReferenceHalf(affine, axis, false, value) },
			)
			io.Register16(highOffset,
				func() uint16 { return p.affineOpenBusHalfword(highOffset) },
				func(value uint16) { p.writeAffineReferenceHalf(affine, axis, true, value) },
			)
		}
	}
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

func (p *PPU) openBusHalfword(ioOffset uint32) uint16 {
	value := p.bus.OpenBus()
	if ioOffset&2 != 0 {
		return uint16(value >> 16)
	}
	return uint16(value)
}

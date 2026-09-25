package ppu

const (
	dispOBJMapping1D = 1 << 6
	dispOBJEnable    = 1 << 12
)

type objSample struct {
	color           [3]byte
	priority        uint8
	oamIndex        int
	semiTransparent bool
	opaque          bool
}

func (p *PPU) objPixel(screenX, screenY int, mode uint16) objSample {
	oam := p.bus.OAM()

	// GBA OBJ-to-OBJ ordering is OAM-index order: the first non-transparent
	// pixel from OBJ0..OBJ127 wins. Attr2 priority only decides OBJ vs BG.
	for index := 0; index < 128; index++ {
		base := index * 8
		attr0 := readOAM16(oam, base)
		attr1 := readOAM16(oam, base+2)
		attr2 := readOAM16(oam, base+4)

		affine := attr0&(1<<8) != 0
		// For regular OBJs attr0 bit 9 disables the object. For affine OBJs
		// the same bit expands the display box to twice the source dimensions.
		if !affine && attr0&(1<<9) != 0 {
			continue
		}

		objMode := (attr0 >> 10) & 0x3
		// OBJ-window pixels are masks rather than visible pixels; mode 3 is
		// prohibited on GBA. Both remain non-visible in this slice.
		if objMode >= 2 {
			continue
		}

		width, height, ok := objDimensions((attr0>>14)&0x3, (attr1>>14)&0x3)
		if !ok {
			continue
		}

		objX := int(attr1 & 0x01ff)
		objY := int(attr0 & 0x00ff)

		var localX, localY int
		if affine {
			var visible bool
			localX, localY, visible = p.affineOBJSource(attr0, attr1, width, height, objX, objY, screenX, screenY)
			if !visible {
				continue
			}
		} else {
			localX = (screenX - objX) & 0x01ff
			localY = (screenY - objY) & 0x00ff
			if localX >= width || localY >= height {
				continue
			}

			if attr1&(1<<12) != 0 {
				localX = width - 1 - localX
			}
			if attr1&(1<<13) != 0 {
				localY = height - 1 - localY
			}
		}

		color, opaque := p.objTilePixel(mode, attr0, attr2, width, localX, localY)
		if !opaque {
			continue
		}

		return objSample{
			color:           color,
			priority:        uint8((attr2 >> 10) & 0x3),
			oamIndex:        index,
			semiTransparent: objMode == 1,
			opaque:          true,
		}
	}

	return objSample{}
}

func (p *PPU) affineOBJSource(attr0, attr1 uint16, sourceWidth, sourceHeight, objX, objY, screenX, screenY int) (int, int, bool) {
	displayWidth, displayHeight := sourceWidth, sourceHeight
	if attr0&(1<<9) != 0 {
		displayWidth *= 2
		displayHeight *= 2
	}

	localX := (screenX - objX) & 0x01ff
	localY := (screenY - objY) & 0x00ff
	if localX >= displayWidth || localY >= displayHeight {
		return 0, 0, false
	}

	group := int((attr1 >> 9) & 0x1f)
	pa, pb, pc, pd := p.objAffineParams(group)

	dx := int32(localX - displayWidth/2)
	dy := int32(localY - displayHeight/2)
	centerX := int32(sourceWidth / 2)
	centerY := int32(sourceHeight / 2)

	sourceX := (int32(pa)*dx + int32(pb)*dy) >> 8
	sourceY := (int32(pc)*dx + int32(pd)*dy) >> 8
	sourceX += centerX
	sourceY += centerY

	if sourceX < 0 || sourceY < 0 || sourceX >= int32(sourceWidth) || sourceY >= int32(sourceHeight) {
		return 0, 0, false
	}
	return int(sourceX), int(sourceY), true
}

func (p *PPU) objAffineParams(group int) (pa, pb, pc, pd int16) {
	oam := p.bus.OAM()
	base := group * 32
	return int16(readOAM16(oam, base+6)),
		int16(readOAM16(oam, base+14)),
		int16(readOAM16(oam, base+22)),
		int16(readOAM16(oam, base+30))
}

func (p *PPU) objTilePixel(mode uint16, attr0, attr2 uint16, width, x, y int) ([3]byte, bool) {
	color256 := attr0&(1<<13) != 0
	baseTile := int(attr2 & 0x03ff)
	if color256 {
		baseTile &^= 1
	}

	tileX := x >> 3
	tileY := y >> 3
	widthTiles := width >> 3

	var tile int
	if p.dispcnt&dispOBJMapping1D != 0 {
		logical := tileY*widthTiles + tileX
		if color256 {
			tile = baseTile + logical*2
		} else {
			tile = baseTile + logical
		}
	} else {
		if color256 {
			tile = baseTile + tileY*32 + tileX*2
		} else {
			tile = baseTile + tileY*32 + tileX
		}
	}
	tile &= 0x03ff

	// In bitmap modes the lower half of OBJ character VRAM is occupied by the
	// bitmap framebuffer; only OBJ tile numbers 512..1023 are displayable.
	if mode >= 3 && mode <= 5 && tile < 512 {
		return [3]byte{}, false
	}

	vram := p.bus.VRAM()
	addr := 0x10000 + tile*32
	localX, localY := x&7, y&7

	var paletteIndex int
	if color256 {
		addr += localY*8 + localX
		if addr < 0 || addr >= len(vram) {
			return [3]byte{}, false
		}
		paletteIndex = int(vram[addr])
		if paletteIndex == 0 {
			return [3]byte{}, false
		}
		paletteIndex += 256
	} else {
		addr += localY*4 + localX/2
		if addr < 0 || addr >= len(vram) {
			return [3]byte{}, false
		}
		packed := vram[addr]
		color := int(packed & 0x0f)
		if localX&1 != 0 {
			color = int(packed >> 4)
		}
		if color == 0 {
			return [3]byte{}, false
		}
		paletteBank := int((attr2 >> 12) & 0x0f)
		paletteIndex = 256 + paletteBank*16 + color
	}

	return p.paletteColor(paletteIndex), true
}

func objDimensions(shape, size uint16) (width, height int, ok bool) {
	switch shape {
	case 0: // square
		d := [4]int{8, 16, 32, 64}[size]
		return d, d, true
	case 1: // horizontal
		widths := [4]int{16, 32, 32, 64}
		heights := [4]int{8, 8, 16, 32}
		return widths[size], heights[size], true
	case 2: // vertical
		widths := [4]int{8, 8, 16, 32}
		heights := [4]int{16, 32, 32, 64}
		return widths[size], heights[size], true
	default:
		return 0, 0, false
	}
}

func readOAM16(oam []byte, offset int) uint16 {
	return uint16(oam[offset]) | uint16(oam[offset+1])<<8
}

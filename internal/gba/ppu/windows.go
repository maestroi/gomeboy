package ppu

const (
	dispWIN0Enable   = 1 << 13
	dispWIN1Enable   = 1 << 14
	dispOBJWINEnable = 1 << 15
)

const (
	windowBG0    = 1 << 0
	windowBG1    = 1 << 1
	windowBG2    = 1 << 2
	windowBG3    = 1 << 3
	windowOBJ    = 1 << 4
	windowEffect = 1 << 5
	windowAll    = 0x3f
)

func (p *PPU) windowMaskAt(mode uint16, x, y int) uint8 {
	if p.dispcnt&(dispWIN0Enable|dispWIN1Enable|dispOBJWINEnable) == 0 {
		return windowAll
	}

	// Hardware window priority is WIN0 > WIN1 > OBJ window > outside.
	if p.dispcnt&dispWIN0Enable != 0 && windowContains(p.winH[0], p.winV[0], x, y) {
		return uint8(p.winIn)
	}
	if p.dispcnt&dispWIN1Enable != 0 && windowContains(p.winH[1], p.winV[1], x, y) {
		return uint8(p.winIn >> 8)
	}
	if p.dispcnt&dispOBJWINEnable != 0 &&
		p.dispcnt&dispOBJEnable != 0 &&
		p.objWindowPixel(x, y, mode) {
		return uint8(p.winOut >> 8)
	}

	return uint8(p.winOut)
}

func windowContains(horizontal, vertical uint16, x, y int) bool {
	left, right := int(horizontal>>8), int(horizontal&0xff)
	top, bottom := int(vertical>>8), int(vertical&0xff)
	return windowAxisContains(x, left, right) && windowAxisContains(y, top, bottom)
}

func windowAxisContains(coord, start, end int) bool {
	switch {
	case start < end:
		return coord >= start && coord < end
	case start > end:
		// Reversed bounds wrap around the 8-bit coordinate space.
		return coord >= start || coord < end
	default:
		// Equal bounds describe an empty window; the pixel falls through to
		// lower-priority windows or the outside region.
		return false
	}
}

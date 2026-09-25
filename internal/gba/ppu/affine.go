package ppu

type affineBGConfig struct {
	priority   uint8
	charBase   int
	screenBase int
	size       int
	wrap       bool
}

func decodeAffineBG(value uint16) affineBGConfig {
	size := 128 << ((value >> 14) & 0x3)
	return affineBGConfig{
		priority:   uint8(value & 0x3),
		charBase:   int((value>>2)&0x3) * 0x4000,
		screenBase: int((value>>8)&0x1f) * 0x800,
		size:       int(size),
		wrap:       value&(1<<13) != 0,
	}
}

func (p *PPU) affineBGPixel(bg int, screenX int) ([3]byte, bool) {
	index := bg - 2
	cfg := decodeAffineBG(p.bgcnt[bg])

	baseX := p.affineCurrent[index][0]
	baseY := p.affineCurrent[index][1]
	sourceX := (baseX + int32(p.affineParam[index][0])*int32(screenX)) >> 8
	sourceY := (baseY + int32(p.affineParam[index][2])*int32(screenX)) >> 8

	x, y := int(sourceX), int(sourceY)
	if cfg.wrap {
		x &= cfg.size - 1
		y &= cfg.size - 1
	} else if x < 0 || y < 0 || x >= cfg.size || y >= cfg.size {
		return [3]byte{}, false
	}

	tilesAcross := cfg.size >> 3
	tileX, tileY := x>>3, y>>3
	mapIndex := tileY*tilesAcross + tileX
	mapOffset := (cfg.screenBase + mapIndex) & 0xffff

	vram := p.bus.VRAM()
	tile := int(vram[mapOffset])
	tileOffset := (cfg.charBase + tile*64 + (y&7)*8 + (x & 7)) & 0xffff
	paletteIndex := int(vram[tileOffset])
	if paletteIndex == 0 {
		return [3]byte{}, false
	}
	return p.paletteColor(paletteIndex), true
}

func (p *PPU) writeAffineParam(bgIndex, param int, value uint16) {
	p.affineParam[bgIndex][param] = int16(value)
}

func (p *PPU) writeAffineReferenceHalf(bgIndex, axis int, high bool, value uint16) {
	raw := p.affineRefRaw[bgIndex][axis]
	if high {
		raw = (raw & 0x0000ffff) | (uint32(value&0x0fff) << 16)
	} else {
		raw = (raw & 0x0fff0000) | uint32(value)
	}
	raw &= 0x0fffffff
	p.affineRefRaw[bgIndex][axis] = raw
	p.affineCurrent[bgIndex][axis] = signExtend28(raw)
}

func signExtend28(value uint32) int32 {
	value &= 0x0fffffff
	if value&(1<<27) != 0 {
		value |= 0xf0000000
	}
	return int32(value)
}

func (p *PPU) reloadAffineReferences() {
	for bg := range p.affineRefRaw {
		for axis := 0; axis < 2; axis++ {
			p.affineCurrent[bg][axis] = signExtend28(p.affineRefRaw[bg][axis])
		}
	}
}

func (p *PPU) advanceAffineLine() {
	for bg := range p.affineCurrent {
		p.affineCurrent[bg][0] += int32(p.affineParam[bg][1]) // PB / dmx
		p.affineCurrent[bg][1] += int32(p.affineParam[bg][3]) // PD / dmy
	}
}

func (p *PPU) affineOpenBusHalfword(offset uint32) uint16 {
	return p.openBusHalfword(offset)
}

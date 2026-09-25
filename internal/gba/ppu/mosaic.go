package ppu

const (
	bgMosaicEnable  = 1 << 6
	objMosaicEnable = 1 << 12
)

func (p *PPU) bgMosaicSize() (horizontal, vertical int) {
	return int(p.mosaic&0x0f) + 1, int((p.mosaic>>4)&0x0f) + 1
}

func (p *PPU) objMosaicSize() (horizontal, vertical int) {
	return int((p.mosaic>>8)&0x0f) + 1, int((p.mosaic>>12)&0x0f) + 1
}

func mosaicCoordinate(coord, size int) int {
	if size <= 1 {
		return coord
	}
	return coord - coord%size
}

func (p *PPU) bgMosaicCoordinates(bg, x, y int) (sampleX, sampleY int) {
	if p.bgcnt[bg]&bgMosaicEnable == 0 {
		return x, y
	}
	horizontal, vertical := p.bgMosaicSize()
	return mosaicCoordinate(x, horizontal), mosaicCoordinate(y, vertical)
}

func (p *PPU) objMosaicCoordinates(attr0 uint16, x, y int) (sampleX, sampleY int) {
	if attr0&objMosaicEnable == 0 {
		return x, y
	}
	horizontal, vertical := p.objMosaicSize()
	return mosaicCoordinate(x, horizontal), mosaicCoordinate(y, vertical)
}

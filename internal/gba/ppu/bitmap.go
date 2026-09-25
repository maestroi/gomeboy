package ppu

import "encoding/binary"

const (
	dispFrameSelect = 1 << 4
	dispForcedBlank = 1 << 7
	dispBG2Enable   = 1 << 10
)

func (p *PPU) bitmapBGPixel(mode uint16, screenX int) ([3]byte, bool) {
	vram := p.bus.VRAM()
	sourceX, sourceY := p.affineSource(0, screenX)
	sx, sy := int(sourceX), int(sourceY)

	switch mode {
	case 3:
		if sx < 0 || sy < 0 || sx >= 240 || sy >= 160 {
			return [3]byte{}, false
		}
		offset := (sy*240 + sx) * 2
		color := binary.LittleEndian.Uint16(vram[offset : offset+2])
		return bgr555(color), true

	case 4:
		if sx < 0 || sy < 0 || sx >= 240 || sy >= 160 {
			return [3]byte{}, false
		}
		page := 0
		if p.dispcnt&dispFrameSelect != 0 {
			page = 0xa000
		}
		index := vram[page+sy*240+sx]
		if index == 0 {
			return [3]byte{}, false
		}
		palette := p.bus.PaletteRAM()
		offset := int(index) * 2
		color := binary.LittleEndian.Uint16(palette[offset : offset+2])
		return bgr555(color), true

	case 5:
		if sx < 0 || sy < 0 || sx >= 160 || sy >= 128 {
			return [3]byte{}, false
		}
		page := 0
		if p.dispcnt&dispFrameSelect != 0 {
			page = 0xa000
		}
		offset := page + (sy*160+sx)*2
		color := binary.LittleEndian.Uint16(vram[offset : offset+2])
		return bgr555(color), true
	}

	return [3]byte{}, false
}

func (p *PPU) paletteColor(index int) [3]byte {
	palette := p.bus.PaletteRAM()
	offset := index * 2
	return bgr555(binary.LittleEndian.Uint16(palette[offset : offset+2]))
}

func bgr555(color uint16) [3]byte {
	r := byte(color & 0x1f)
	g := byte((color >> 5) & 0x1f)
	b := byte((color >> 10) & 0x1f)
	return [3]byte{
		(r << 3) | (r >> 2),
		(g << 3) | (g >> 2),
		(b << 3) | (b >> 2),
	}
}

func (p *PPU) setPixel(x, y int, color [3]byte) {
	offset := (y*ScreenWidth + x) * 3
	copy(p.frame[offset:offset+3], color[:])
}

func (p *PPU) fillLineColor(y int, color [3]byte) {
	p.fillLine(y, color[0], color[1], color[2])
}

func (p *PPU) fillLine(y int, r, g, b byte) {
	offset := y * ScreenWidth * 3
	for x := 0; x < ScreenWidth; x++ {
		p.frame[offset+0] = r
		p.frame[offset+1] = g
		p.frame[offset+2] = b
		offset += 3
	}
}

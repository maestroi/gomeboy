package ppu

import "encoding/binary"

const (
	dispFrameSelect = 1 << 4
	dispForcedBlank = 1 << 7
	dispBG2Enable   = 1 << 10
)

func (p *PPU) renderLine(y int) {
	if p.dispcnt&dispForcedBlank != 0 {
		p.fillLine(y, 0xff, 0xff, 0xff)
		return
	}

	backdrop := p.paletteColor(0)
	mode := p.dispcnt & 0x7
	if p.dispcnt&dispBG2Enable == 0 || mode < 3 || mode > 5 {
		p.fillLineColor(y, backdrop)
		return
	}

	switch mode {
	case 3:
		p.renderMode3(y)
	case 4:
		p.renderMode4(y, backdrop)
	case 5:
		p.renderMode5(y, backdrop)
	}
}

func (p *PPU) renderMode3(y int) {
	vram := p.bus.VRAM()
	for x := 0; x < ScreenWidth; x++ {
		offset := (y*ScreenWidth + x) * 2
		color := binary.LittleEndian.Uint16(vram[offset : offset+2])
		p.setPixel(x, y, bgr555(color))
	}
}

func (p *PPU) renderMode4(y int, backdrop [3]byte) {
	vram := p.bus.VRAM()
	page := 0
	if p.dispcnt&dispFrameSelect != 0 {
		page = 0xa000
	}
	palette := p.bus.PaletteRAM()

	for x := 0; x < ScreenWidth; x++ {
		index := vram[page+y*ScreenWidth+x]
		if index == 0 {
			p.setPixel(x, y, backdrop)
			continue
		}
		offset := int(index) * 2
		color := binary.LittleEndian.Uint16(palette[offset : offset+2])
		p.setPixel(x, y, bgr555(color))
	}
}

func (p *PPU) renderMode5(y int, backdrop [3]byte) {
	if y >= 128 {
		p.fillLineColor(y, backdrop)
		return
	}

	vram := p.bus.VRAM()
	page := 0
	if p.dispcnt&dispFrameSelect != 0 {
		page = 0xa000
	}

	for x := 0; x < ScreenWidth; x++ {
		if x >= 160 {
			p.setPixel(x, y, backdrop)
			continue
		}
		offset := page + (y*160+x)*2
		color := binary.LittleEndian.Uint16(vram[offset : offset+2])
		p.setPixel(x, y, bgr555(color))
	}
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

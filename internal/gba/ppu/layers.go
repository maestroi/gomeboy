package ppu

type layerPixel struct {
	color    [3]byte
	priority uint8
	index    int
	opaque   bool
}

func (p *PPU) renderLine(y int) {
	if p.dispcnt&dispForcedBlank != 0 {
		p.fillLine(y, 0xff, 0xff, 0xff)
		return
	}

	backdrop := p.paletteColor(0)
	mode := p.dispcnt & 0x7

	for x := 0; x < ScreenWidth; x++ {
		color := backdrop
		bgPriority := uint8(4)
		windowMask := p.windowMaskAt(mode, x, y)

		if bg := p.backgroundPixel(mode, x, y, windowMask); bg.opaque {
			color = bg.color
			bgPriority = bg.priority
		}

		if p.dispcnt&dispOBJEnable != 0 && windowMask&windowOBJ != 0 {
			if obj := p.objPixel(x, y, mode); obj.opaque && obj.priority <= bgPriority {
				// On equal numeric priority OBJ is above BG.
				color = obj.color
			}
		}

		p.setPixel(x, y, color)
	}
}

func (p *PPU) backgroundPixel(mode uint16, x, y int, windowMask uint8) layerPixel {
	switch mode {
	case 0, 1, 2:
		return p.tileBackgroundPixel(mode, x, y, windowMask)
	case 3, 4, 5:
		if p.dispcnt&dispBG2Enable == 0 || windowMask&windowBG2 == 0 {
			return layerPixel{}
		}
		color, opaque := p.bitmapBGPixel(mode, x)
		return layerPixel{
			color:    color,
			priority: uint8(p.bgcnt[2] & 0x3),
			index:    2,
			opaque:   opaque,
		}
	default:
		return layerPixel{}
	}
}

func (p *PPU) tileBackgroundPixel(mode uint16, x, y int, windowMask uint8) layerPixel {
	best := layerPixel{priority: 4, index: 4}

	for bg := 0; bg < 4; bg++ {
		kind := bgKindForMode(mode, bg)
		if kind == bgUnavailable ||
			p.dispcnt&(1<<(8+bg)) == 0 ||
			windowMask&(1<<bg) == 0 {
			continue
		}

		priority := uint8(p.bgcnt[bg] & 0x3)
		if best.opaque && (priority > best.priority || (priority == best.priority && bg > best.index)) {
			continue
		}

		var color [3]byte
		var opaque bool
		switch kind {
		case bgText:
			color, opaque = p.textBGPixel(bg, decodeTextBG(p.bgcnt[bg]), x, y)
		case bgAffine:
			color, opaque = p.affineBGPixel(bg, x)
		}
		if !opaque {
			continue
		}

		best = layerPixel{
			color:    color,
			priority: priority,
			index:    bg,
			opaque:   true,
		}
	}

	return best
}

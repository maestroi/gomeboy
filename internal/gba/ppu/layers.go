package ppu

const (
	layerBG0 uint8 = iota
	layerBG1
	layerBG2
	layerBG3
	layerOBJ
	layerBackdrop
)

type layerPixel struct {
	color           [3]byte
	layer           uint8
	priority        uint8
	index           int
	semiTransparent bool
}

func (p *PPU) renderLine(y int) {
	if p.dispcnt&dispForcedBlank != 0 {
		p.fillLine(y, 0xff, 0xff, 0xff)
		return
	}

	mode := p.dispcnt & 0x7
	for x := 0; x < ScreenWidth; x++ {
		windowMask := p.windowMaskAt(mode, x, y)
		stack, count := p.visibleLayerStack(mode, x, y, windowMask)
		top := stack[0]

		var second layerPixel
		hasSecond := count > 1
		if hasSecond {
			second = stack[1]
		}

		p.setPixel(x, y, p.applyColorEffect(top, second, hasSecond, windowMask))
	}
}

func (p *PPU) visibleLayerStack(mode uint16, x, y int, windowMask uint8) ([6]layerPixel, int) {
	var stack [6]layerPixel
	count := 0

	add := func(pixel layerPixel) {
		stack[count] = pixel
		count++
		for i := count - 1; i > 0 && layerAbove(stack[i], stack[i-1]); i-- {
			stack[i], stack[i-1] = stack[i-1], stack[i]
		}
	}

	switch mode {
	case 0, 1, 2:
		for bg := 0; bg < 4; bg++ {
			kind := bgKindForMode(mode, bg)
			if kind == bgUnavailable ||
				p.dispcnt&(1<<(8+bg)) == 0 ||
				windowMask&(1<<bg) == 0 {
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

			add(layerPixel{
				color:    color,
				layer:    uint8(bg),
				priority: uint8(p.bgcnt[bg] & 0x3),
				index:    bg,
			})
		}

	case 3, 4, 5:
		if p.dispcnt&dispBG2Enable != 0 && windowMask&windowBG2 != 0 {
			if color, opaque := p.bitmapBGPixel(mode, x); opaque {
				add(layerPixel{
					color:    color,
					layer:    layerBG2,
					priority: uint8(p.bgcnt[2] & 0x3),
					index:    2,
				})
			}
		}
	}

	if p.dispcnt&dispOBJEnable != 0 && windowMask&windowOBJ != 0 {
		if obj := p.objPixel(x, y, mode); obj.opaque {
			add(layerPixel{
				color:           obj.color,
				layer:           layerOBJ,
				priority:        obj.priority,
				index:           obj.oamIndex,
				semiTransparent: obj.semiTransparent,
			})
		}
	}

	add(layerPixel{
		color:    p.paletteColor(0),
		layer:    layerBackdrop,
		priority: 4,
		index:    0,
	})

	return stack, count
}

func layerAbove(a, b layerPixel) bool {
	if a.layer == layerBackdrop {
		return false
	}
	if b.layer == layerBackdrop {
		return true
	}
	if a.priority != b.priority {
		return a.priority < b.priority
	}

	// At equal BG-relative priority, OBJ is above BG. There is only one OBJ
	// candidate here: objPixel already resolved OBJ-vs-OBJ by OAM index.
	if a.layer == layerOBJ && b.layer != layerOBJ {
		return true
	}
	if b.layer == layerOBJ && a.layer != layerOBJ {
		return false
	}

	// Equal-priority BGs are ordered by lower BG number.
	return a.index < b.index
}

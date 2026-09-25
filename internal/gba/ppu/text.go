package ppu

type textBGConfig struct {
	priority   uint8
	charBase   int
	color256   bool
	screenBase int
	width      int
	height     int
}

func (p *PPU) renderTextMode(y int, mode uint16, backdrop [3]byte) {
	for x := 0; x < ScreenWidth; x++ {
		color := backdrop
		bestPriority := uint8(4)
		found := false

		// Lower BG number wins ties at equal priority, so scan BG0 -> BG3 and
		// replace only for a strictly better priority.
		for bg := 0; bg < 4; bg++ {
			kind := bgKindForMode(mode, bg)
			if kind == bgUnavailable || p.dispcnt&(1<<(8+bg)) == 0 {
				continue
			}

			priority := uint8(p.bgcnt[bg] & 0x3)
			if found && priority >= bestPriority {
				continue
			}

			var pixel [3]byte
			var opaque bool
			switch kind {
			case bgText:
				pixel, opaque = p.textBGPixel(bg, decodeTextBG(p.bgcnt[bg]), x, y)
			case bgAffine:
				pixel, opaque = p.affineBGPixel(bg, x)
			}
			if !opaque {
				continue
			}

			color = pixel
			bestPriority = priority
			found = true
		}

		p.setPixel(x, y, color)
	}
}

type bgKind uint8

const (
	bgUnavailable bgKind = iota
	bgText
	bgAffine
)

func bgKindForMode(mode uint16, bg int) bgKind {
	switch mode {
	case 0:
		return bgText
	case 1:
		switch bg {
		case 0, 1:
			return bgText
		case 2:
			return bgAffine
		}
	case 2:
		if bg == 2 || bg == 3 {
			return bgAffine
		}
	}
	return bgUnavailable
}

func decodeTextBG(value uint16) textBGConfig {
	size := (value >> 14) & 0x3
	width, height := 256, 256
	switch size {
	case 1:
		width = 512
	case 2:
		height = 512
	case 3:
		width, height = 512, 512
	}

	return textBGConfig{
		priority:   uint8(value & 0x3),
		charBase:   int((value>>2)&0x3) * 0x4000,
		color256:   value&(1<<7) != 0,
		screenBase: int((value>>8)&0x1f) * 0x800,
		width:      width,
		height:     height,
	}
}

func (p *PPU) textBGPixel(bg int, cfg textBGConfig, screenX, screenY int) ([3]byte, bool) {
	worldX := (screenX + int(p.bghofs[bg])) & (cfg.width - 1)
	worldY := (screenY + int(p.bgvofs[bg])) & (cfg.height - 1)

	tileX := worldX >> 3
	tileY := worldY >> 3
	screensAcross := cfg.width >> 8

	blockX := tileX >> 5
	blockY := tileY >> 5
	screenBlock := blockY*screensAcross + blockX
	mapBase := (cfg.screenBase + screenBlock*0x800) & 0xffff
	mapIndex := (tileY&31)*32 + (tileX & 31)
	entryOffset := (mapBase + mapIndex*2) & 0xffff

	vram := p.bus.VRAM()
	entry := readBGVRAM16(vram, entryOffset)

	tileNumber := int(entry & 0x03ff)
	localX := worldX & 7
	localY := worldY & 7
	if entry&(1<<10) != 0 {
		localX = 7 - localX
	}
	if entry&(1<<11) != 0 {
		localY = 7 - localY
	}

	var paletteIndex int
	if cfg.color256 {
		tileOffset := (cfg.charBase + tileNumber*64 + localY*8 + localX) & 0xffff
		paletteIndex = int(vram[tileOffset])
	} else {
		tileOffset := (cfg.charBase + tileNumber*32 + localY*4 + localX/2) & 0xffff
		packed := vram[tileOffset]
		if localX&1 == 0 {
			paletteIndex = int(packed & 0x0f)
		} else {
			paletteIndex = int(packed >> 4)
		}
		if paletteIndex != 0 {
			paletteIndex += int((entry>>12)&0x0f) * 16
		}
	}

	// Palette index zero is transparent for text backgrounds.
	if paletteIndex == 0 {
		return [3]byte{}, false
	}
	return p.paletteColor(paletteIndex), true
}

func readBGVRAM16(vram []byte, offset int) uint16 {
	lo := vram[offset&0xffff]
	hi := vram[(offset+1)&0xffff]
	return uint16(lo) | uint16(hi)<<8
}

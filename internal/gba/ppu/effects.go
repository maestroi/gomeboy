package ppu

const (
	effectNone uint16 = iota
	effectAlpha
	effectBrighten
	effectDarken
)

func (p *PPU) applyColorEffect(top layerPixel, second layerPixel, hasSecond bool, windowMask uint8) [3]byte {
	if windowMask&windowEffect == 0 {
		return top.color
	}

	// Semi-transparent OBJ forces alpha blending whenever the immediately
	// underlying visible pixel is selected as a second target. This overrides
	// BLDCNT's effect mode and OBJ first-target bit.
	if top.semiTransparent && hasSecond && p.isSecondTarget(second.layer) {
		return alphaBlend(top.color, second.color, p.alphaA(), p.alphaB())
	}

	mode := (p.bldcnt >> 6) & 0x3
	if mode == effectNone || !p.isFirstTarget(top.layer) {
		return top.color
	}

	switch mode {
	case effectAlpha:
		if hasSecond && p.isSecondTarget(second.layer) {
			return alphaBlend(top.color, second.color, p.alphaA(), p.alphaB())
		}
	case effectBrighten:
		return brighten(top.color, p.brightnessY())
	case effectDarken:
		return darken(top.color, p.brightnessY())
	}
	return top.color
}

func (p *PPU) isFirstTarget(layer uint8) bool {
	return p.bldcnt&(1<<layer) != 0
}

func (p *PPU) isSecondTarget(layer uint8) bool {
	return p.bldcnt&(1<<(8+layer)) != 0
}

func (p *PPU) alphaA() int {
	return clampEffectCoefficient(int(p.bldalpha & 0x1f))
}

func (p *PPU) alphaB() int {
	return clampEffectCoefficient(int((p.bldalpha >> 8) & 0x1f))
}

func (p *PPU) brightnessY() int {
	return clampEffectCoefficient(int(p.bldy & 0x1f))
}

func clampEffectCoefficient(value int) int {
	if value > 16 {
		return 16
	}
	return value
}

func alphaBlend(first, second [3]byte, eva, evb int) [3]byte {
	var out [3]byte
	for i := range out {
		a := int(first[i] >> 3)
		b := int(second[i] >> 3)
		value := (a*eva + b*evb) >> 4
		if value > 31 {
			value = 31
		}
		out[i] = expand5(value)
	}
	return out
}

func brighten(color [3]byte, evy int) [3]byte {
	var out [3]byte
	for i := range out {
		value := int(color[i] >> 3)
		value += ((31 - value) * evy) >> 4
		if value > 31 {
			value = 31
		}
		out[i] = expand5(value)
	}
	return out
}

func darken(color [3]byte, evy int) [3]byte {
	var out [3]byte
	for i := range out {
		value := int(color[i] >> 3)
		value -= (value * evy) >> 4
		if value < 0 {
			value = 0
		}
		out[i] = expand5(value)
	}
	return out
}

func expand5(value int) byte {
	return byte((value << 3) | (value >> 2))
}

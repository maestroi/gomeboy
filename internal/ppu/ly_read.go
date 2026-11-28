package ppu

import "github.com/maestroi/gomeboy/internal/types"

// setLCDLine starts LCD line clock line `line` at startDots from now.
func (p *PPU) setLCDLine(line uint8, startDots int) {
	k := 1
	if p.s.DoubleSpeed() {
		k = 2
	}
	p.lcdLine = line
	p.lcdLineStart = uint64(int64(p.s.Cycle()) + int64(startDots*k))
}

// lcdLineClock returns the LCD scanline and its dot (0-455).
func (p *PPU) lcdLineClock() (line, dot int) {
	k := int64(1)
	if p.s.DoubleSpeed() {
		k = 2
	}
	line = int(p.lcdLine)
	d := (int64(p.s.Cycle()) - int64(p.lcdLineStart)) / k
	if d < 0 {
		d += 456
		line--
		if line < 0 {
			line = 153
		}
	}
	return line, int(d)
}

// cpuVisibleLY derives the CPU-facing LY from the LCD line clock,
// following AGE's gb_lcd::read_ly. DMG and DMG-compatibility keep the
// event-driven LY register.
func (p *PPU) cpuVisibleLY() byte {
	if !p.enabled || !p.cgbMode {
		return p.b.Get(types.LY)
	}
	line, clk := p.lcdLineClock()
	ds := p.s.DoubleSpeed()
	if !ds {
		if clk--; clk < 0 {
			clk += 456
			if line--; line < 0 {
				line = 153
			}
		}
	}
	cgbENormal := p.b.Model() == types.CGBDE && !ds

	ly := line
	switch {
	case line == 153 && clk > 2+b2i(cgbENormal):
		ly = 0
	case clk >= 456-1-b2i(ds):
		ly++
	case clk >= 456-3+b2i(cgbENormal):
		ly &= ly + 1
	}
	return byte(ly)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

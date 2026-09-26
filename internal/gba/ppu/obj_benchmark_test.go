package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func BenchmarkOBJScanlineEvaluation(b *testing.B) {
	p, mem := newTestPPU(b, Hooks{})
	disableAllOBJ(mem)
	setOBJPaletteColor(mem, 1, 0x001f)

	// Keep most OAM entries disabled, as real games commonly do, and place a
	// handful of live sprites late in OAM. The legacy per-pixel lookup still
	// walks the disabled entries repeatedly; the scanline path decodes each
	// entry once and only samples covered pixels.
	for index := 120; index < 128; index++ {
		tile := index - 120
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				setOBJ4bppPixel(mem, tile, x, y, 1)
			}
		}
		setOBJAttrs(mem, index,
			20,
			uint16((index-120)*24)|(1<<14), // 16x16 square spread across line.
			uint16(tile),
		)
	}
	mem.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})

	b.Run("legacy-per-pixel-oam-scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for x := 0; x < ScreenWidth; x++ {
				_ = p.objPixel(x, 20, 0)
			}
		}
	})

	b.Run("scanline-rasterizer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p.prepareOBJLine(20, 0)
			p.objLineValid = false
		}
	})
}

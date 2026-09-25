package ppu

import (
	"encoding/binary"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func disableAllOBJ(b *bus.Bus) {
	oam := b.OAM()
	for i := 0; i < 128; i++ {
		putOAM16(oam, i*8, 1<<9)
		putOAM16(oam, i*8+2, 0)
		putOAM16(oam, i*8+4, 0)
	}
}

func putOAM16(oam []byte, offset int, value uint16) {
	binary.LittleEndian.PutUint16(oam[offset:offset+2], value)
}

func setOBJAttrs(b *bus.Bus, index int, attr0, attr1, attr2 uint16) {
	oam := b.OAM()
	base := index * 8
	putOAM16(oam, base, attr0)
	putOAM16(oam, base+2, attr1)
	putOAM16(oam, base+4, attr2)
}

func setOBJPaletteColor(b *bus.Bus, index int, color uint16) {
	offset := 0x200 + index*2
	binary.LittleEndian.PutUint16(b.PaletteRAM()[offset:offset+2], color)
}

func setOBJ4bppPixel(b *bus.Bus, tile, x, y int, color byte) {
	addr := 0x10000 + tile*32 + y*4 + x/2
	if x&1 == 0 {
		b.VRAM()[addr] = (b.VRAM()[addr] & 0xf0) | (color & 0x0f)
	} else {
		b.VRAM()[addr] = (b.VRAM()[addr] & 0x0f) | (color << 4)
	}
}

func setOBJ8bppPixel(b *bus.Bus, tile, x, y int, color byte) {
	addr := 0x10000 + (tile&^1)*32 + y*8 + x
	b.VRAM()[addr] = color
}

func TestOBJ4bppPaletteBank(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 3*16+1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0,
		0,
		0,
		0|(0<<10)|(3<<12),
	)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("4bpp OBJ pixel = %v, want red", got)
	}
}

func TestOBJ8bppUsesOBJPaletteAndEvenTileNumber(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 5, 0x03e0)
	setOBJ8bppPixel(b, 2, 0, 0, 5)

	// Tile 3 aliases tile 2 in 256-color OBJ mode.
	setOBJAttrs(b, 0,
		1<<13,
		0,
		3,
	)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("8bpp OBJ pixel = %v, want green", got)
	}
}

func TestOBJHorizontalAndVerticalFlip(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x7c00)
	setOBJ4bppPixel(b, 0, 7, 7, 1)
	setOBJAttrs(b, 0,
		0,
		(1<<12)|(1<<13),
		0,
	)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("flipped OBJ pixel = %v, want blue", got)
	}
}

func TestOBJCoordinateWrapping(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 4, 4, 1)

	// X=508 means -4 on the 240px screen; Y=252 means -4.
	setOBJAttrs(b, 0,
		252,
		508,
		0,
	)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("wrapped OBJ pixel = %v, want red", got)
	}
}

func TestOBJShapeAndSizeDimensions(t *testing.T) {
	cases := []struct {
		shape, size uint16
		w, h        int
	}{
		{0, 0, 8, 8}, {0, 3, 64, 64},
		{1, 0, 16, 8}, {1, 3, 64, 32},
		{2, 0, 8, 16}, {2, 3, 32, 64},
	}
	for _, tc := range cases {
		w, h, ok := objDimensions(tc.shape, tc.size)
		if !ok || w != tc.w || h != tc.h {
			t.Fatalf("shape=%d size=%d -> %dx%d ok=%v, want %dx%d", tc.shape, tc.size, w, h, ok, tc.w, tc.h)
		}
	}
	if _, _, ok := objDimensions(3, 0); ok {
		t.Fatal("prohibited OBJ shape accepted")
	}
}

func TestOBJ1DAnd2DMapping(t *testing.T) {
	render := func(oneD bool) [3]byte {
		p, b := newTestPPU(t, Hooks{})
		disableAllOBJ(b)
		setOBJPaletteColor(b, 1, 0x001f)
		setOBJPaletteColor(b, 2, 0x03e0)

		// 16x16 square, base tile 4. Pixel on the second tile row comes from
		// tile 6 in 1D mapping and tile 36 in 2D mapping.
		setOBJ4bppPixel(b, 6, 0, 0, 1)
		setOBJ4bppPixel(b, 36, 0, 0, 2)
		setOBJAttrs(b, 0,
			0,
			1<<14, // square size 1 = 16x16
			4,
		)

		dispcnt := uint16(dispOBJEnable)
		if oneD {
			dispcnt |= dispOBJMapping1D
		}
		b.Write16(bus.IOStart+dispCNTOffset, dispcnt, bus.Access{})
		p.Advance(8*CyclesPerLine + VisibleCycles)
		return rgbAt(p.FrameBuffer(), 0, 8)
	}

	if got := render(true); got != [3]byte{255, 0, 0} {
		t.Fatalf("1D mapped second row = %v, want red", got)
	}
	if got := render(false); got != [3]byte{0, 255, 0} {
		t.Fatalf("2D mapped second row = %v, want green", got)
	}
}

func TestOBJPriorityAgainstBG(t *testing.T) {
	render := func(objPriority uint16) [3]byte {
		p, b := newTestPPU(t, Hooks{})
		disableAllOBJ(b)

		setBGPaletteColor(b, 1, 0x001f)
		set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
		setScreenEntry(b.VRAM(), 8, 0, 1)
		// BG0 priority 1.
		b.Write16(bus.IOStart+0x008, 1|(8<<8), bus.Access{})

		setOBJPaletteColor(b, 1, 0x03e0)
		setOBJ4bppPixel(b, 0, 0, 0, 1)
		setOBJAttrs(b, 0, 0, 0, objPriority<<10)

		b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|dispOBJEnable, bus.Access{})
		p.Advance(VisibleCycles)
		return rgbAt(p.FrameBuffer(), 0, 0)
	}

	// Equal numeric priority: OBJ is in front of BG.
	if got := render(1); got != [3]byte{0, 255, 0} {
		t.Fatalf("equal OBJ/BG priority = %v, want OBJ green", got)
	}
	// Numerically lower BG priority is in front.
	if got := render(2); got != [3]byte{255, 0, 0} {
		t.Fatalf("BG priority 1 vs OBJ 2 = %v, want BG red", got)
	}
	// Numerically lower OBJ priority is in front.
	if got := render(0); got != [3]byte{0, 255, 0} {
		t.Fatalf("OBJ priority 0 vs BG 1 = %v, want OBJ green", got)
	}
}

func TestLowerOAMIndexWinsBetweenOBJRegardlessOfBGPriorityField(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJPaletteColor(b, 2, 0x03e0)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJ4bppPixel(b, 1, 0, 0, 2)

	// OBJ0 has the "worse" BG-relative priority but still wins OBJ-vs-OBJ.
	setOBJAttrs(b, 0, 0, 0, (3<<10)|0)
	setOBJAttrs(b, 1, 0, 0, (0<<10)|1)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("overlapping OBJ winner = %v, want OBJ0 red", got)
	}
}

func TestOBJDisableAffineAndWindowModesAreNotVisibleInBaseSlice(t *testing.T) {
	cases := []struct {
		name  string
		attr0 uint16
	}{
		{"disabled regular", 1 << 9},
		{"affine deferred", 1 << 8},
		{"OBJ window", 2 << 10},
		{"prohibited mode", 3 << 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, b := newTestPPU(t, Hooks{})
			disableAllOBJ(b)
			setBGPaletteColor(b, 0, 0x7c00)
			setOBJPaletteColor(b, 1, 0x001f)
			setOBJ4bppPixel(b, 0, 0, 0, 1)
			setOBJAttrs(b, 0, tc.attr0, 0, 0)

			b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
			p.Advance(VisibleCycles)

			if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
				t.Fatalf("hidden/deferred OBJ rendered as %v, want blue backdrop", got)
			}
		})
	}
}

func TestSemiTransparentOBJStillRendersBeforeBlendingSlice(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0, 1<<10, 0, 0)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("semi-transparent OBJ base color = %v, want red", got)
	}
}

func TestBitmapModesRequireUpperOBJTileHalf(t *testing.T) {
	render := func(tile uint16) [3]byte {
		p, b := newTestPPU(t, Hooks{})
		disableAllOBJ(b)
		setBGPaletteColor(b, 0, 0x7c00)
		setOBJPaletteColor(b, 1, 0x001f)
		setOBJ4bppPixel(b, int(tile), 0, 0, 1)
		setOBJAttrs(b, 0, 0, 0, tile)

		b.Write16(bus.IOStart+dispCNTOffset, 3|dispOBJEnable, bus.Access{})
		p.Advance(VisibleCycles)
		return rgbAt(p.FrameBuffer(), 0, 0)
	}

	if got := render(511); got != [3]byte{0, 0, 255} {
		t.Fatalf("bitmap mode tile511 = %v, want backdrop blue", got)
	}
	if got := render(512); got != [3]byte{255, 0, 0} {
		t.Fatalf("bitmap mode tile512 = %v, want OBJ red", got)
	}
}

func TestOBJDisplayEnableBit(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)
	setBGPaletteColor(b, 0, 0x7c00)
	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0, 0, 0, 0)

	// OBJ enable bit is clear.
	b.Write16(bus.IOStart+dispCNTOffset, 0, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("disabled OBJ display = %v, want backdrop blue", got)
	}
}

package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestMosaicRegisterIsWriteOnlyAndDecodesSizes(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	b.Write16(bus.IOStart+0x04c, 0x3210, bus.Access{})
	if p.mosaic != 0x3210 {
		t.Fatalf("MOSAIC stored = %04x, want 3210", p.mosaic)
	}
	bgH, bgV := p.bgMosaicSize()
	objH, objV := p.objMosaicSize()
	if bgH != 1 || bgV != 2 || objH != 3 || objV != 4 {
		t.Fatalf("mosaic sizes BG=%dx%d OBJ=%dx%d, want BG=1x2 OBJ=3x4", bgH, bgV, objH, objV)
	}

	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x04c, bus.Access{}); got != 0x2211 {
		t.Fatalf("MOSAIC read = %04x, want open-bus low lane 2211", got)
	}
}

func TestTextBGMosaicRepeatsUpperLeftPixel(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f) // red
	setBGPaletteColor(b, 2, 0x03e0) // green
	setBGPaletteColor(b, 3, 0x7c00) // blue

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	set4bppPixel(vram, 0, 1, 1, 0, 2)
	set4bppPixel(vram, 0, 1, 0, 1, 3)
	setScreenEntry(vram, 8, 0, 1)

	// BG mosaic 2x2: register values are size minus one.
	b.Write16(bus.IOStart+0x04c, 0x0011, bus.Access{})
	b.Write16(bus.IOStart+0x008, bgMosaicEnable|(8<<8), bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})

	p.Advance(CyclesPerLine + VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("horizontal text mosaic = %v, want repeated red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 0, 1); got != [3]byte{255, 0, 0} {
		t.Fatalf("vertical text mosaic = %v, want repeated red", got)
	}
}

func TestBGWithoutMosaicEnableIgnoresMosaicRegister(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)
	set4bppPixel(vram, 0, 1, 0, 0, 1)
	set4bppPixel(vram, 0, 1, 1, 0, 2)
	setScreenEntry(vram, 8, 0, 1)

	b.Write16(bus.IOStart+0x04c, 0x000f, bus.Access{}) // BG width 16
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})   // mosaic flag clear
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mosaic-disabled BG = %v, want original green x1", got)
	}
}

func TestAffineBGMosaicUsesTopRowReferencePoint(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)
	setAffineMapEntry(vram, 8, 0, 1)
	set8bppPixel(vram, 0, 1, 0, 0, 1)
	set8bppPixel(vram, 0, 1, 0, 1, 2)

	setAffineIdentity(b, 2)
	b.Write16(bus.IOStart+0x04c, 1<<4, bus.Access{}) // BG vertical size 2
	b.Write16(bus.IOStart+0x00c, bgMosaicEnable|(8<<8), bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10), bus.Access{})

	p.Advance(CyclesPerLine + VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 1); got != [3]byte{255, 0, 0} {
		t.Fatalf("affine vertical mosaic = %v, want line0 red repeated on line1", got)
	}
}

func TestBitmapBGMosaicUsesAffineSampling(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setAffineIdentity(b, 2)

	put16(b.VRAM(), 0, 0x001f)
	put16(b.VRAM(), 2, 0x03e0)
	put16(b.VRAM(), 240*2, 0x7c00)

	b.Write16(bus.IOStart+0x04c, 0x0011, bus.Access{}) // BG 2x2
	b.Write16(bus.IOStart+0x00c, bgMosaicEnable, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 3|dispBG2Enable, bus.Access{})

	p.Advance(CyclesPerLine + VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("bitmap horizontal mosaic = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 0, 1); got != [3]byte{255, 0, 0} {
		t.Fatalf("bitmap vertical mosaic = %v, want red", got)
	}
}

func TestRegularOBJMosaicRepeatsSampledPixel(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJPaletteColor(b, 2, 0x03e0)
	setOBJPaletteColor(b, 3, 0x7c00)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJ4bppPixel(b, 0, 1, 0, 2)
	setOBJ4bppPixel(b, 0, 0, 1, 3)
	setOBJAttrs(b, 0, objMosaicEnable, 0, 0)

	// OBJ 2x2.
	b.Write16(bus.IOStart+0x04c, (1<<8)|(1<<12), bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(CyclesPerLine + VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("OBJ horizontal mosaic = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 0, 1); got != [3]byte{255, 0, 0} {
		t.Fatalf("OBJ vertical mosaic = %v, want red", got)
	}
}

func TestOBJWithoutMosaicFlagIgnoresObjectMosaicSize(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJPaletteColor(b, 2, 0x03e0)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJ4bppPixel(b, 0, 1, 0, 2)
	setOBJAttrs(b, 0, 0, 0, 0)

	b.Write16(bus.IOStart+0x04c, 0x0f00, bus.Access{}) // OBJ width 16
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mosaic-disabled OBJ = %v, want original green x1", got)
	}
}

func TestAffineOBJMosaicUsesSameDisplayGrid(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJPaletteColor(b, 2, 0x03e0)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJ4bppPixel(b, 0, 1, 0, 2)
	setOBJAffineParams(b, 0, 0x0100, 0, 0, 0x0100)
	setOBJAttrs(b, 0, (1<<8)|objMosaicEnable, 0, 0)

	b.Write16(bus.IOStart+0x04c, 1<<8, bus.Access{}) // OBJ width 2
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("affine OBJ mosaic = %v, want repeated red", got)
	}
}

func TestOBJMosaicDoesNotExtendDisplayRectangle(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setBGPaletteColor(b, 0, 0x7c00)
	setOBJPaletteColor(b, 1, 0x001f)
	for x := 0; x < 8; x++ {
		setOBJ4bppPixel(b, 0, x, 0, 1)
	}
	setOBJAttrs(b, 0, objMosaicEnable, 0, 0)

	b.Write16(bus.IOStart+0x04c, 0x0f00, bus.Access{}) // OBJ width 16
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 8, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("OBJ mosaic extended past 8px display box: %v", got)
	}
}

func TestOBJWindowUsesMosaicedOBJCoverage(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	// x1 is transparent but should inherit x0 with 2px horizontal mosaic.
	setOBJAttrs(b, 0, (2<<10)|objMosaicEnable, 0, 0)

	b.Write16(bus.IOStart+0x04c, 1<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable|dispOBJWINEnable, bus.Access{})

	if !p.objWindowPixel(1, 0, 0) {
		t.Fatal("mosaiced OBJ-window coverage did not repeat opaque x0 into x1")
	}
}

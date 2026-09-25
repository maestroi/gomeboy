package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func setAffineIdentity(b *bus.Bus, bg int) {
	base := uint32(0x020)
	if bg == 3 {
		base = 0x030
	}
	b.Write16(bus.IOStart+base+0x0, 0x0100, bus.Access{}) // PA
	b.Write16(bus.IOStart+base+0x2, 0x0000, bus.Access{}) // PB
	b.Write16(bus.IOStart+base+0x4, 0x0000, bus.Access{}) // PC
	b.Write16(bus.IOStart+base+0x6, 0x0100, bus.Access{}) // PD
	b.Write32(bus.IOStart+base+0x8, 0, bus.Access{})      // X
	b.Write32(bus.IOStart+base+0xc, 0, bus.Access{})      // Y
}

func setAffineMapEntry(vram []byte, screenBaseBlock, index int, tile byte) {
	offset := (screenBaseBlock*0x800 + index) & 0xffff
	vram[offset] = tile
}

func TestAffineRegistersAreWriteOnlyAndSigned(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	b.Write16(bus.IOStart+0x020, 0xff80, bus.Access{}) // PA = -0.5
	if p.affineParam[0][0] != -128 {
		t.Fatalf("BG2PA = %d, want -128", p.affineParam[0][0])
	}

	// -1.0 in signed 20.8 is 0x0fffff00 in the 28-bit register.
	b.Write32(bus.IOStart+0x028, 0x0fffff00, bus.Access{})
	if got := p.affineCurrent[0][0]; got != -256 {
		t.Fatalf("BG2X internal = %d, want -256", got)
	}
	if got := p.affineRefRaw[0][0]; got != 0x0fffff00 {
		t.Fatalf("BG2X raw = %08x, want 0fffff00", got)
	}

	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x020, bus.Access{}); got != 0x2211 {
		t.Fatalf("BG2PA read = %04x, want open-bus low lane", got)
	}
	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x022, bus.Access{}); got != 0x4433 {
		t.Fatalf("BG2PB read = %04x, want open-bus high lane", got)
	}
}

func TestAffineTileIdentityAndScaling(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)

	// One 128x128 affine map at screen block 8, tile 1 in the top-left.
	setAffineMapEntry(vram, 8, 0, 1)
	set8bppPixel(vram, 0, 1, 0, 0, 1)
	set8bppPixel(vram, 0, 1, 2, 0, 2)

	// Mode 2, BG2 enabled, 128x128, char block 0, screen block 8.
	b.Write16(bus.IOStart+0x00c, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10), bus.Access{})

	// PA=2.0 makes screen x=1 sample source x=2.
	b.Write16(bus.IOStart+0x020, 0x0200, bus.Access{})
	b.Write16(bus.IOStart+0x024, 0x0000, bus.Access{})
	b.Write16(bus.IOStart+0x026, 0x0100, bus.Access{})
	b.Write32(bus.IOStart+0x028, 0, bus.Access{})
	b.Write32(bus.IOStart+0x02c, 0, bus.Access{})

	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("affine x0 = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("affine scaled x1 = %v, want green from source x2", got)
	}
}

func TestAffineAreaOverflowTransparentOrWrap(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)

	// Source x=127 is the final pixel of the 128px map.
	lastTile := 15
	setAffineMapEntry(vram, 8, lastTile, 1)
	set8bppPixel(vram, 0, 1, 7, 0, 1)

	b.Write16(bus.IOStart+0x00c, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10), bus.Access{})
	setAffineIdentity(b, 2)

	// Start at x=127: screen x0 is red, x1 is out of bounds and transparent.
	b.Write32(bus.IOStart+0x028, 127<<8, bus.Access{})
	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("edge pixel = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("overflow without wrap = %v, want backdrop blue", got)
	}

	// Fresh PPU with area overflow enabled: x=128 wraps to x=0.
	p2, b2 := newTestPPU(t, Hooks{})
	setBGPaletteColor(b2, 0, 0x7c00)
	setBGPaletteColor(b2, 1, 0x001f)
	setAffineMapEntry(b2.VRAM(), 8, 0, 1)
	set8bppPixel(b2.VRAM(), 0, 1, 0, 0, 1)
	b2.Write16(bus.IOStart+0x00c, (8<<8)|(1<<13), bus.Access{})
	b2.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10), bus.Access{})
	setAffineIdentity(b2, 2)
	b2.Write32(bus.IOStart+0x028, 128<<8, bus.Access{})
	p2.Advance(VisibleCycles)
	if got := rgbAt(p2.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("wrapped pixel = %v, want red", got)
	}
}

func TestAffineLineAdvanceAndVBlankReload(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	// Start X at 10.0; PB advances X by 1.0 each visible scanline.
	b.Write16(bus.IOStart+0x020, 0x0100, bus.Access{})
	b.Write16(bus.IOStart+0x022, 0x0100, bus.Access{})
	b.Write16(bus.IOStart+0x024, 0x0000, bus.Access{})
	b.Write16(bus.IOStart+0x026, 0x0100, bus.Access{})
	b.Write32(bus.IOStart+0x028, 10<<8, bus.Access{})

	if got := p.affineCurrent[0][0]; got != 10<<8 {
		t.Fatalf("initial internal X = %d, want %d", got, 10<<8)
	}

	p.Advance(VisibleCycles)
	if got := p.affineCurrent[0][0]; got != 11<<8 {
		t.Fatalf("internal X after line0 = %d, want %d", got, 11<<8)
	}

	// Advance to the start of VBlank. The internal reference reloads to BG2X.
	p.Advance((CyclesPerLine-VisibleCycles) + uint32(VisibleLines-1)*CyclesPerLine)
	if p.VCount() != VBlankStartLine {
		t.Fatalf("VCOUNT = %d, want %d", p.VCount(), VBlankStartLine)
	}
	if got := p.affineCurrent[0][0]; got != 10<<8 {
		t.Fatalf("VBlank-reloaded X = %d, want %d", got, 10<<8)
	}
}

func TestAffineReferenceWriteUpdatesCurrentImmediately(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setAffineIdentity(b, 2)

	p.Advance(CyclesPerLine + 100)
	b.Write32(bus.IOStart+0x028, 42<<8, bus.Access{})
	if got := p.affineCurrent[0][0]; got != 42<<8 {
		t.Fatalf("mid-frame BG2X write current=%d, want %d", got, 42<<8)
	}
}

func TestMode1ComposesTextAndAffineBackgrounds(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)

	// BG0 text red, priority 2.
	set4bppPixel(vram, 0, 1, 0, 0, 1)
	setScreenEntry(vram, 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 2|(8<<8), bus.Access{})

	// BG2 affine green, priority 1.
	setAffineMapEntry(vram, 10, 0, 2)
	set8bppPixel(vram, 1, 2, 0, 0, 2)
	b.Write16(bus.IOStart+0x00c, 1|(1<<2)|(10<<8), bus.Access{})
	setAffineIdentity(b, 2)

	b.Write16(bus.IOStart+dispCNTOffset, 1|(1<<8)|(1<<10), bus.Access{})
	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mode1 composition = %v, want affine BG2 green", got)
	}
}

func TestMode2ComposesBG2AndBG3ByPriority(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)

	// BG2 red priority 2.
	setAffineMapEntry(vram, 8, 0, 1)
	set8bppPixel(vram, 0, 1, 0, 0, 1)
	b.Write16(bus.IOStart+0x00c, 2|(8<<8), bus.Access{})
	setAffineIdentity(b, 2)

	// BG3 green priority 1, separate char/map blocks.
	setAffineMapEntry(vram, 10, 0, 2)
	set8bppPixel(vram, 1, 2, 0, 0, 2)
	b.Write16(bus.IOStart+0x00e, 1|(1<<2)|(10<<8), bus.Access{})
	setAffineIdentity(b, 3)

	b.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10)|(1<<11), bus.Access{})
	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mode2 composition = %v, want BG3 green", got)
	}
}

func TestBitmapMode3UsesAffineReferenceAndScale(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	put16(b.PaletteRAM(), 0, 0x7c00)
	put16(b.VRAM(), (0*240+2)*2, 0x001f)
	put16(b.VRAM(), (0*240+4)*2, 0x03e0)

	b.Write16(bus.IOStart+0x020, 0x0200, bus.Access{}) // PA=2
	b.Write16(bus.IOStart+0x026, 0x0100, bus.Access{}) // PD=1
	b.Write32(bus.IOStart+0x028, 2<<8, bus.Access{})
	b.Write32(bus.IOStart+0x02c, 0, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 3|dispBG2Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("mode3 affine x0 = %v, want source x2 red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mode3 affine x1 = %v, want source x4 green", got)
	}
}

func TestBitmapOverflowAlwaysTransparent(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBGPaletteColor(b, 0, 0x7c00)
	put16(b.VRAM(), 0, 0x001f)

	// Set BG2CNT overflow bit even though bitmap modes ignore it.
	b.Write16(bus.IOStart+0x00c, 1<<13, bus.Access{})
	setBG2Identity(b)
	b.Write32(bus.IOStart+0x028, 240<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 3|dispBG2Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("bitmap overflow wrapped unexpectedly: %v", got)
	}
}

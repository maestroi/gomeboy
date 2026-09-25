package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func setWindowRect(b *bus.Bus, index int, left, right, top, bottom byte) {
	hOffset := uint32(0x040 + index*2)
	vOffset := uint32(0x044 + index*2)
	b.Write16(bus.IOStart+hOffset, uint16(left)<<8|uint16(right), bus.Access{})
	b.Write16(bus.IOStart+vOffset, uint16(top)<<8|uint16(bottom), bus.Access{})
}

func TestWindowRegistersAndWriteOnlyBounds(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	setWindowRect(b, 0, 10, 20, 30, 40)
	if p.winH[0] != 0x0a14 || p.winV[0] != 0x1e28 {
		t.Fatalf("WIN0 bounds = %04x/%04x, want 0a14/1e28", p.winH[0], p.winV[0])
	}

	b.Write16(bus.IOStart+0x048, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+0x04a, 0xffff, bus.Access{})
	if got, _ := b.Read16(bus.IOStart+0x048, bus.Access{}); got != 0x3f3f {
		t.Fatalf("WININ = %04x, want 3f3f", got)
	}
	if got, _ := b.Read16(bus.IOStart+0x04a, bus.Access{}); got != 0x3f3f {
		t.Fatalf("WINOUT = %04x, want 3f3f", got)
	}

	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x040, bus.Access{}); got != 0x2211 {
		t.Fatalf("WIN0H read = %04x, want open-bus low lane 2211", got)
	}
	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x042, bus.Access{}); got != 0x4433 {
		t.Fatalf("WIN1H read = %04x, want open-bus high lane 4433", got)
	}
}

func TestWindowBoundsInclusiveExclusiveAndWrap(t *testing.T) {
	if !windowContains(10<<8|20, 30<<8|40, 10, 30) {
		t.Fatal("left/top boundary should be inside")
	}
	if windowContains(10<<8|20, 30<<8|40, 20, 39) {
		t.Fatal("right boundary should be exclusive")
	}
	if windowContains(10<<8|20, 30<<8|40, 19, 40) {
		t.Fatal("bottom boundary should be exclusive")
	}

	// Reversed bounds wrap: X>=200 or X<40, Y>=150 or Y<20.
	h := uint16(200<<8 | 40)
	v := uint16(150<<8 | 20)
	for _, pt := range [][2]int{{220, 155}, {10, 10}, {220, 10}, {10, 155}} {
		if !windowContains(h, v, pt[0], pt[1]) {
			t.Fatalf("wrapped point %v should be inside", pt)
		}
	}
	if windowContains(h, v, 100, 100) {
		t.Fatal("middle point should be outside wrapped window")
	}

	if windowContains(0, 0, 0, 0) {
		t.Fatal("equal bounds should be empty")
	}
}

func TestWindowPriorityWIN0OverWIN1(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setWindowRect(b, 0, 0, 20, 0, 20)
	setWindowRect(b, 1, 0, 20, 0, 20)

	// WIN0 allows BG0 only; WIN1 allows BG1 only.
	b.Write16(bus.IOStart+0x048, uint16(windowBG0)|uint16(windowBG1)<<8, bus.Access{})
	b.Write16(bus.IOStart+0x04a, windowAll, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, dispWIN0Enable|dispWIN1Enable, bus.Access{})

	if got := p.windowMaskAt(0, 5, 5); got != windowBG0 {
		t.Fatalf("overlap mask = %02x, want WIN0 mask %02x", got, windowBG0)
	}
}

func TestWindowCanRevealLowerPriorityBackground(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 0, 0x7c00) // blue backdrop
	setBGPaletteColor(b, 1, 0x001f) // red BG0
	setBGPaletteColor(b, 2, 0x03e0) // green BG1

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	setScreenEntry(vram, 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 0|(8<<8), bus.Access{})

	set4bppPixel(vram, 1, 1, 0, 0, 2)
	setScreenEntry(vram, 10, 0, 1)
	b.Write16(bus.IOStart+0x00a, 1|(1<<2)|(10<<8), bus.Access{})

	setWindowRect(b, 0, 0, 1, 0, 1)
	// Inside WIN0: BG1 only. Outside: BG0 and BG1.
	b.Write16(bus.IOStart+0x048, windowBG1, bus.Access{})
	b.Write16(bus.IOStart+0x04a, windowBG0|windowBG1, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset,
		(1<<8)|(1<<9)|dispWIN0Enable,
		bus.Access{})

	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("inside window = %v, want lower-priority BG1 green", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("outside window = %v, want BG0 red", got)
	}
}

func TestWindowLayerMaskStillRequiresDISPCNTMasterEnable(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})

	setWindowRect(b, 0, 0, 10, 0, 10)
	b.Write16(bus.IOStart+0x048, windowBG0, bus.Access{})
	// BG0 master enable is intentionally clear.
	b.Write16(bus.IOStart+dispCNTOffset, dispWIN0Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("window bypassed DISPCNT BG0 master enable: %v", got)
	}
}

func TestEnabledWindowWithZeroMasksShowsBackdrop(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})

	setWindowRect(b, 0, 0, 10, 0, 10)
	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|dispWIN0Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("zero WININ mask = %v, want backdrop blue", got)
	}
}

func TestWindowCanMaskVisibleOBJ(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)
	setBGPaletteColor(b, 0, 0x7c00)
	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJ4bppPixel(b, 0, 1, 0, 1)
	setOBJAttrs(b, 0, 0, 0, 0)

	setWindowRect(b, 0, 0, 1, 0, 1)
	// WIN0 hides OBJ; outside allows it.
	b.Write16(bus.IOStart+0x048, 0, bus.Access{})
	b.Write16(bus.IOStart+0x04a, windowOBJ, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable|dispWIN0Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("masked OBJ = %v, want backdrop blue", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("outside OBJ = %v, want red", got)
	}
}

func TestOBJWindowUsesNonTransparentPixelsAndDoesNotDraw(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setBGPaletteColor(b, 0, 0x7c00) // blue backdrop
	setBGPaletteColor(b, 1, 0x001f) // red BG0
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	set4bppPixel(b.VRAM(), 0, 1, 1, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})

	// OBJ-window sprite has a non-transparent pixel at x0 and transparent x1.
	setOBJPaletteColor(b, 1, 0x03e0)
	setOBJ4bppPixel(b, 2, 0, 0, 1)
	setOBJAttrs(b, 0, 2<<10, 0, 2)

	// Outside allows BG0; inside OBJ window disables all layers.
	b.Write16(bus.IOStart+0x04a, windowBG0, bus.Access{})
	b.Write16(bus.IOStart+0x04a, windowBG0, bus.Access{})
	// High byte is OBJ-window control.
	b.Write16(bus.IOStart+0x04a, uint16(windowBG0), bus.Access{})
	p.winOut = uint16(windowBG0) // outside
	p.winOut |= 0 << 8           // OBJ window: backdrop only

	b.Write16(bus.IOStart+dispCNTOffset,
		(1<<8)|dispOBJEnable|dispOBJWINEnable,
		bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("OBJ-window pixel = %v, want backdrop blue", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("transparent OBJ-window pixel = %v, want BG0 red", got)
	}
}

func TestOBJWindowPriorityBelowWIN1(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)
	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0, 2<<10, 0, 0)

	setWindowRect(b, 1, 0, 8, 0, 8)
	// WIN1 mask BG1, OBJ-window mask BG2.
	b.Write16(bus.IOStart+0x048, uint16(windowBG1)<<8, bus.Access{})
	b.Write16(bus.IOStart+0x04a, uint16(windowBG2)<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset,
		dispOBJEnable|dispWIN1Enable|dispOBJWINEnable,
		bus.Access{})

	if got := p.windowMaskAt(0, 0, 0); got != windowBG1 {
		t.Fatalf("WIN1/OBJ overlap mask = %02x, want WIN1 %02x", got, windowBG1)
	}
}

func TestAffineOBJCanDefineOBJWindow(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)
	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAffineParams(b, 0, 0x0100, 0, 0, 0x0100)
	setOBJAttrs(b, 0, (1<<8)|(2<<10), 0, 0)

	b.Write16(bus.IOStart+0x04a, uint16(windowBG2)<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset,
		dispOBJEnable|dispOBJWINEnable,
		bus.Access{})

	if got := p.windowMaskAt(0, 0, 0); got != windowBG2 {
		t.Fatalf("affine OBJ-window mask = %02x, want %02x", got, windowBG2)
	}
}

func TestOBJWindowRequiresOBJMasterEnable(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)
	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0, 2<<10, 0, 0)

	// Outside=BG0; OBJ-window=BG2.
	b.Write16(bus.IOStart+0x04a,
		uint16(windowBG0)|uint16(windowBG2)<<8,
		bus.Access{})
	// OBJ window enabled, but OBJ master display bit 12 is clear.
	b.Write16(bus.IOStart+dispCNTOffset, dispOBJWINEnable, bus.Access{})

	if got := p.windowMaskAt(0, 0, 0); got != windowBG0 {
		t.Fatalf("OBJ-window active without OBJ master enable: mask=%02x", got)
	}
}

func TestWindowEffectBitIsPreservedForNextEffectsSlice(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setWindowRect(b, 0, 0, 8, 0, 8)
	b.Write16(bus.IOStart+0x048, windowBG0|windowEffect, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, dispWIN0Enable, bus.Access{})

	if got := p.windowMaskAt(0, 0, 0); got != windowBG0|windowEffect {
		t.Fatalf("window effect bit lost: %02x", got)
	}
}

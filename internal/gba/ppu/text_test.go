package ppu

import (
	"encoding/binary"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func setBGPaletteColor(b *bus.Bus, index int, color uint16) {
	binary.LittleEndian.PutUint16(b.PaletteRAM()[index*2:index*2+2], color)
}

func setScreenEntry(vram []byte, screenBaseBlock, mapIndex int, entry uint16) {
	offset := (screenBaseBlock*0x800 + mapIndex*2) & 0xffff
	vram[offset] = byte(entry)
	vram[(offset+1)&0xffff] = byte(entry >> 8)
}

func set4bppPixel(vram []byte, charBaseBlock, tile, x, y int, color byte) {
	offset := (charBaseBlock*0x4000 + tile*32 + y*4 + x/2) & 0xffff
	if x&1 == 0 {
		vram[offset] = (vram[offset] & 0xf0) | (color & 0x0f)
	} else {
		vram[offset] = (vram[offset] & 0x0f) | (color << 4)
	}
}

func set8bppPixel(vram []byte, charBaseBlock, tile, x, y int, color byte) {
	offset := (charBaseBlock*0x4000 + tile*64 + y*8 + x) & 0xffff
	vram[offset] = color
}

func TestBGControlAndScrollRegisters(t *testing.T) {
	_, b := newTestPPU(t, Hooks{})

	b.Write16(bus.IOStart+0x008, 0xffff, bus.Access{})
	if got, _ := b.Read16(bus.IOStart+0x008, bus.Access{}); got != 0xffcf {
		t.Fatalf("BG0CNT = %04x, want ffcf with unused bits cleared", got)
	}

	b.Write16(bus.IOStart+0x010, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+0x012, 0x1234, bus.Access{})
	if got, _ := b.Read16(bus.IOStart+0x010, bus.Access{}); got != 0x01ff {
		t.Fatalf("BG0HOFS = %04x, want 01ff", got)
	}
	if got, _ := b.Read16(bus.IOStart+0x012, bus.Access{}); got != 0x0034 {
		t.Fatalf("BG0VOFS = %04x, want 0034", got)
	}
}

func TestMode0Renders4BPPTextBackground(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 0, 0x7c00)  // blue backdrop
	setBGPaletteColor(b, 17, 0x001f) // bank 1, color 1 = red
	setBGPaletteColor(b, 18, 0x03e0) // bank 1, color 2 = green

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	set4bppPixel(vram, 0, 1, 7, 0, 2)
	setScreenEntry(vram, 8, 0, 1|(1<<12))

	// Mode 0, BG0 enabled. BG0: char block 0, screen block 8.
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("4bpp x0 = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 7, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("4bpp x7 = %v, want green", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("transparent tile pixel = %v, want backdrop blue", got)
	}
}

func TestTextBackgroundTileFlips(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)
	setBGPaletteColor(b, 3, 0x7c00)

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	set4bppPixel(vram, 0, 1, 7, 0, 2)
	set4bppPixel(vram, 0, 1, 7, 7, 3)
	setScreenEntry(vram, 8, 0, 1|(1<<10)|(1<<11))

	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("HV-flipped x0/y0 = %v, want blue from source x7/y7", got)
	}
}

func TestMode0Renders8BPPTextBackground(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 5, 0x03ff) // yellow
	set8bppPixel(vram, 1, 2, 0, 0, 5)
	setScreenEntry(vram, 16, 0, 2)

	// priority 0, char block 1, 256-color, screen block 16.
	bgcnt := uint16((1 << 2) | (1 << 7) | (16 << 8))
	b.Write16(bus.IOStart+0x008, bgcnt, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 255, 0} {
		t.Fatalf("8bpp pixel = %v, want yellow", got)
	}
}

func TestTextBackgroundScrollAndLargeScreenBlocks(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f)
	set4bppPixel(vram, 0, 1, 0, 0, 1)

	// 512x512 screen: coordinate (256,256) is block (1,1), which is screen
	// block base+3 when there are two blocks across.
	setScreenEntry(vram, 4+3, 0, 1)
	bgcnt := uint16((4 << 8) | (3 << 14))
	b.Write16(bus.IOStart+0x008, bgcnt, bus.Access{})
	b.Write16(bus.IOStart+0x010, 256, bus.Access{})
	b.Write16(bus.IOStart+0x012, 256, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("512x512 scrolled pixel = %v, want red", got)
	}
}

func TestTextBackgroundPriorityAndTransparency(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 0, 0x7c00) // blue backdrop
	setBGPaletteColor(b, 1, 0x001f) // red
	setBGPaletteColor(b, 2, 0x03e0) // green

	// BG0 tile: x0 transparent, x1 red.
	set4bppPixel(vram, 0, 1, 1, 0, 1)
	setScreenEntry(vram, 8, 0, 1)

	// BG1 tile in char block 1: green at x0 and x1.
	set4bppPixel(vram, 1, 1, 0, 0, 2)
	set4bppPixel(vram, 1, 1, 1, 0, 2)
	setScreenEntry(vram, 10, 0, 1)

	// BG0 priority 1. BG1 priority 3.
	b.Write16(bus.IOStart+0x008, 1|(8<<8), bus.Access{})
	b.Write16(bus.IOStart+0x00a, 3|(1<<2)|(10<<8), bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|(1<<9), bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("transparent front BG did not reveal BG1: %v", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("higher-priority BG0 lost at x1: %v", got)
	}
}

func TestTextBackgroundTiePrefersLowerBGNumber(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 1, 0x001f)
	setBGPaletteColor(b, 2, 0x03e0)

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	setScreenEntry(vram, 8, 0, 1)
	set4bppPixel(vram, 1, 1, 0, 0, 2)
	setScreenEntry(vram, 10, 0, 1)

	// Both priority 2; BG0 should win.
	b.Write16(bus.IOStart+0x008, 2|(8<<8), bus.Access{})
	b.Write16(bus.IOStart+0x00a, 2|(1<<2)|(10<<8), bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|(1<<9), bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("equal-priority winner = %v, want BG0 red", got)
	}
}

func TestMode1UsesOnlyBG0AndBG1AsTextBackgrounds(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()
	setBGPaletteColor(b, 0, 0x7c00)
	setBGPaletteColor(b, 1, 0x001f)

	// Put a valid red text tile on BG2. Mode 1 BG2 is affine, so the text
	// renderer must not accidentally render it.
	set4bppPixel(vram, 0, 1, 0, 0, 1)
	setScreenEntry(vram, 8, 0, 1)
	b.Write16(bus.IOStart+0x00c, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1|(1<<10), bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("mode1 BG2 rendered as text: %v, want backdrop blue", got)
	}

	// Fresh PPU: BG0 is a text layer in mode 1 and should render.
	p2, b2 := newTestPPU(t, Hooks{})
	setBGPaletteColor(b2, 0, 0x7c00)
	setBGPaletteColor(b2, 1, 0x001f)
	set4bppPixel(b2.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b2.VRAM(), 8, 0, 1)
	b2.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})
	b2.Write16(bus.IOStart+dispCNTOffset, 1|(1<<8), bus.Access{})
	p2.Advance(VisibleCycles)
	if got := rgbAt(p2.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("mode1 BG0 = %v, want red", got)
	}
}

func TestMode2DefersAffineBackgroundsToBackdrop(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBGPaletteColor(b, 0, 0x03e0)
	setBGPaletteColor(b, 1, 0x001f)
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)

	b.Write16(bus.IOStart+0x00c, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 2|(1<<10), bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mode2 pre-affine output = %v, want green backdrop", got)
	}
}

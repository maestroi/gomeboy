package ppu

import (
	"encoding/binary"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func rgbAt(frame []byte, x, y int) [3]byte {
	offset := (y*ScreenWidth + x) * 3
	return [3]byte{frame[offset], frame[offset+1], frame[offset+2]}
}

func put16(mem []byte, offset int, value uint16) {
	binary.LittleEndian.PutUint16(mem[offset:offset+2], value)
}

func setBG2Identity(b *bus.Bus) {
	b.Write16(bus.IOStart+0x020, 0x0100, bus.Access{}) // PA
	b.Write16(bus.IOStart+0x022, 0x0000, bus.Access{}) // PB
	b.Write16(bus.IOStart+0x024, 0x0000, bus.Access{}) // PC
	b.Write16(bus.IOStart+0x026, 0x0100, bus.Access{}) // PD
	b.Write32(bus.IOStart+0x028, 0, bus.Access{})      // X
	b.Write32(bus.IOStart+0x02c, 0, bus.Access{})      // Y
}

func TestBGR555Conversion(t *testing.T) {
	cases := []struct {
		in   uint16
		want [3]byte
	}{
		{0x001f, [3]byte{255, 0, 0}},
		{0x03e0, [3]byte{0, 255, 0}},
		{0x7c00, [3]byte{0, 0, 255}},
		{0x7fff, [3]byte{255, 255, 255}},
	}
	for _, tc := range cases {
		if got := bgr555(tc.in); got != tc.want {
			t.Errorf("bgr555(%04x) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMode3RendersDirectColor(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBG2Identity(b)
	vram := b.VRAM()

	put16(vram, 0, 0x001f)
	put16(vram, 2, 0x03e0)
	put16(vram, 4, 0x7c00)

	b.Write16(bus.IOStart+dispCNTOffset, 3|dispBG2Enable, bus.Access{})
	p.Advance(VisibleCycles)

	frame := p.FrameBuffer()
	if got := rgbAt(frame, 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("pixel 0 = %v, want red", got)
	}
	if got := rgbAt(frame, 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("pixel 1 = %v, want green", got)
	}
	if got := rgbAt(frame, 2, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("pixel 2 = %v, want blue", got)
	}
}

func TestMode4PaletteAndPageSelection(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBG2Identity(b)
	vram := b.VRAM()
	pal := b.PaletteRAM()

	put16(pal, 0, 0x03e0) // backdrop/index 0 = green
	put16(pal, 2, 0x001f) // index 1 = red
	put16(pal, 4, 0x7c00) // index 2 = blue

	vram[0] = 1
	vram[1] = 0
	vram[0xa000] = 2

	b.Write16(bus.IOStart+dispCNTOffset, 4|dispBG2Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("page0 pixel0 = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("index0 pixel = %v, want backdrop green", got)
	}

	// Render the next frame's line 0 from page 1.
	p.Advance((CyclesPerLine-VisibleCycles) + uint32(ScanlinesPerFrame-1)*CyclesPerLine)
	b.Write16(bus.IOStart+dispCNTOffset, 4|dispBG2Enable|dispFrameSelect, bus.Access{})
	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("page1 pixel0 = %v, want blue", got)
	}
}

func TestMode5Uses160x128AndBackdropOutside(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBG2Identity(b)
	vram := b.VRAM()
	pal := b.PaletteRAM()

	put16(pal, 0, 0x03e0) // green backdrop
	put16(vram, (0*160+159)*2, 0x001f)
	put16(vram, 0xa000+(0*160+0)*2, 0x7c00)

	b.Write16(bus.IOStart+dispCNTOffset, 5|dispBG2Enable, bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 159, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("mode5 x159 = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 160, 0); got != [3]byte{0, 255, 0} {
		t.Fatalf("mode5 x160 = %v, want backdrop green", got)
	}

	// Page 1 check on a fresh PPU so line zero renders again.
	p2, b2 := newTestPPU(t, Hooks{})
	setBG2Identity(b2)
	put16(b2.PaletteRAM(), 0, 0x03e0)
	put16(b2.VRAM(), 0xa000, 0x7c00)
	b2.Write16(bus.IOStart+dispCNTOffset, 5|dispBG2Enable|dispFrameSelect, bus.Access{})
	p2.Advance(VisibleCycles)
	if got := rgbAt(p2.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("mode5 page1 pixel0 = %v, want blue", got)
	}
}

func TestMode5BottomOutsideUsesBackdrop(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBG2Identity(b)
	put16(b.PaletteRAM(), 0, 0x001f)
	b.Write16(bus.IOStart+dispCNTOffset, 5|dispBG2Enable, bus.Access{})

	// Render through line 128. Line 128 is outside the 160x128 bitmap.
	p.Advance(uint32(128)*CyclesPerLine + VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 128); got != [3]byte{255, 0, 0} {
		t.Fatalf("mode5 y128 = %v, want backdrop red", got)
	}
}

func TestForcedBlankIsWhite(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	put16(b.PaletteRAM(), 0, 0x7c00)
	b.Write16(bus.IOStart+dispCNTOffset, dispForcedBlank, bus.Access{})

	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 100, 0); got != [3]byte{255, 255, 255} {
		t.Fatalf("forced blank pixel = %v, want white", got)
	}
}

func TestDisabledBitmapBGUsesBackdrop(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	put16(b.PaletteRAM(), 0, 0x7c00)
	put16(b.VRAM(), 0, 0x001f)
	b.Write16(bus.IOStart+dispCNTOffset, 3, bus.Access{}) // BG2 disabled

	p.Advance(VisibleCycles)
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{0, 0, 255} {
		t.Fatalf("disabled BG2 pixel = %v, want blue backdrop", got)
	}
}

func TestBitmapModesMoveOBJVRAMBoundary(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	_ = p

	b.Write16(bus.IOStart+dispCNTOffset, 3|dispBG2Enable, bus.Access{})
	b.Write16(bus.VRAMStart+0x10000, 0x1234, bus.Access{})
	b.Write8(bus.VRAMStart+0x10000, 0x99, bus.Access{})
	if got, _ := b.Read16(bus.VRAMStart+0x10000, bus.Access{}); got != 0x9999 {
		t.Fatalf("bitmap BG area byte write = %04x, want 9999", got)
	}

	b.Write16(bus.VRAMStart+0x14000, 0x5678, bus.Access{})
	b.Write8(bus.VRAMStart+0x14000, 0xaa, bus.Access{})
	if got, _ := b.Read16(bus.VRAMStart+0x14000, bus.Access{}); got != 0x5678 {
		t.Fatalf("bitmap OBJ area byte write changed to %04x", got)
	}
}

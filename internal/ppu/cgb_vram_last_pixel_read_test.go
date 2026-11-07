package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBVRAMReadReleasesAfterLastPixelBeforeBusUnlock(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE, types.DMGABC} {
		for _, colourMode := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/colour=%t", model, colourMode), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = colourMode && model.IsCGB()
				b.Write(0x8000, 0x5a)
				b.Write(0xfe00, 0x66)
				b.Lock(gbio.VRAM | gbio.OAM)
				p.mode = ModeVRAM
				p.lineState = PixelTransferLX
				p.lx = 167

				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("pixel transfer prematurely opened VRAM read: %#02x", got)
				}
				p.lx = 168
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("pixel-transfer state opened VRAM before HBlank handover: %#02x", got)
				}

				p.lineState = EnterHBlank
				got := b.Read(0x8000)
				want := byte(0xff)
				if model.IsCGB() {
					want = 0x5a
				}
				if got != want {
					t.Fatalf("VRAM read at last-pixel HBlank edge: got %#02x, want %#02x", got, want)
				}
				b.Write(0x8000, 0xa5)
				if got := b.Get(0x8000); got != 0x5a {
					t.Fatalf("VRAM write bypassed existing lock: %#02x", got)
				}
				if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("OAM read bypassed existing lock: %#02x", got)
				}

				// The shortened LCD-enabled startup line uses a scheduled
				// dummy transfer without ever pushing 168 pixels.
				p.lx = 0
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("startup dummy line lost its VRAM read lock: %#02x", got)
				}

				// Once HBlank actually unlocks VRAM, keep the ordinary
				// unblocked behavior regardless of emulation mode.
				p.lineState = HBlankUpdateLY
				b.Unlock(gbio.VRAM | gbio.OAM)
				if got := b.Read(0x8000); got != 0x5a {
					t.Fatalf("HBlank VRAM read remained locked: %#02x", got)
				}
			})
		}
	}
}

func TestCGBVRAMReadEndOfRealScanline(t *testing.T) {
	for _, doubleSpeed := range []bool{false, true} {
		for _, scx := range []uint8{0, 2, 6} {
			t.Run(fmt.Sprintf("double=%t/scx=%d", doubleSpeed, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				rom[0x143] = 0x80
				b := gbio.NewBus(s, rom)
				b.Map(types.CGBBC)
				p := New(b, s)
				if doubleSpeed {
					s.ChangeSpeed(true)
				}
				b.Set(0x8000, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)

				// The shortened initial line does not render pixels.
				// Advance into LY=1 and the first ordinary scanline
				// transfer, then find its final pixel handover.
				found := false
				for i := 0; i < 1300*2; i++ {
					s.Tick(1)
					if p.ly == 1 && p.lineState == EnterHBlank && p.lx == 168 {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("normal CGB scanline did not reach last-pixel HBlank handover")
				}
				if got := b.Read(0x8000); got != 0x5a {
					t.Fatalf("CPU VRAM read remained blocked after last pixel: got %#02x", got)
				}
				b.Write(0x8000, 0xaa)
				if got := b.Get(0x8000); got != 0x5a {
					t.Fatalf("VRAM write opened before PPU HBlank event: %#02x", got)
				}
			})
		}
	}
}

func TestNCMVRAMReadOpensAtEndOfOrdinaryVisibleLine(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, scx := range []uint8{0, 2, 6} {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only ROM
				b.Map(model)
				p := New(b, s)
				b.Set(0x8000, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if p.cgbMode || !p.enabled {
					t.Fatalf("test fixture should be NCM with LCD enabled")
				}
				found := false
				for i := 0; i < 1300; i++ {
					s.Tick(1)
					if p.ly == 1 && p.lineState == EnterHBlank && p.lx == 168 {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("NCM PPU did not enter normal scanline last-pixel handover")
				}
				if got := b.Read(0x8000); got != 0x5a {
					t.Fatalf("NCM CPU VRAM read not released after last pixel: %#02x", got)
				}
				b.Write(0x8000, 0xaa)
				if got := b.Get(0x8000); got != 0x5a {
					t.Fatalf("NCM VRAM write improperly released before PPU HBlank event: %#02x", got)
				}
			})
		}
	}
}

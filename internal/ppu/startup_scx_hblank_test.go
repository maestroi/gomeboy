package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// First-line pixel generation is skipped, but CGB Mode-3 must still account
// for the discarded SCX pixels before the CPU VRAM read bus opens at HBlank.
func TestCGBStartupHBlankVRAMReleaseTracksSCX(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			for _, scx := range []uint8{0, 1, 2, 3, 6, 7} {
				name := fmt.Sprintf("%s/double=%t/scx=%d", model, doubleSpeed, scx)
				t.Run(name, func(t *testing.T) {
					s := scheduler.NewScheduler()
					rom := make([]byte, 0x8000)
					rom[0x143] = 0x80 // CGB-colour cartridge
					b := gbio.NewBus(s, rom)
					b.Map(model)
					p := New(b, s)
					if doubleSpeed {
						s.ChangeSpeed(true)
					}
					b.Set(0x8000, 0x5a)
					b.Write(types.SCX, scx)
					b.Write(types.LCDC, types.Bit7)
					if !p.cgbMode || !p.enabled {
						t.Fatalf("failed to enter CGB LCD startup, enabled=%t colour=%t", p.enabled, p.cgbMode)
					}

					// On CGB colour mode the startup OAM bus handover is at
					// dot 83; its dummy transfer lasts 168+SCX dots. The
					// normal-speed HBlank event is delayed 2 dots and the
					// double-speed event is delayed 1 dot.
					openDot := uint64(253) + uint64(scx)
					if doubleSpeed {
						openDot = (252 + uint64(scx)) * 2
					}
					s.Tick(openDot - 1)
					if got := b.Read(0x8000); got != 0xff {
						t.Fatalf("VRAM reopened before SCX-dependent HBlank edge: got %#02x", got)
					}
					s.Tick(1)
					if got := b.Read(0x8000); got != 0x5a {
						t.Fatalf("VRAM still blocked at HBlank release: got %#02x", got)
					}
				})
			}
		}
	}
}

func TestCGBCompatibilityStartupHBlankTimingUnchangedBySCX(t *testing.T) {
	for _, doubleSpeed := range []bool{false, true} {
		s := scheduler.NewScheduler()
		b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
		b.Map(types.CGBBC)
		p := New(b, s)
		if doubleSpeed {
			s.ChangeSpeed(true)
		}
		b.Set(0x8000, 0x5a)
		b.Write(types.SCX, 7)
		b.Write(types.LCDC, types.Bit7)
		if p.cgbMode {
			t.Fatal("fixture unexpectedly entered CGB colour mode")
		}
		// DMG-compatible startup begins one dot after LCD enable and
		// continues to use the established fixed-duration dummy transfer.
		openDot := uint64(254)
		if doubleSpeed {
			openDot = 508
		}
		s.Tick(openDot - 1)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("compatibility VRAM reopened too soon (double=%t): %#02x", doubleSpeed, got)
		}
		s.Tick(1)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("compatibility VRAM still blocked (double=%t): %#02x", doubleSpeed, got)
		}
	}
}

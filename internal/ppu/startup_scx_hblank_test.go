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
			for _, scx := range []uint8{0, 1, 2, 3, 4, 5, 6, 7} {
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

					// On CGB colour mode the startup OAM handover reaches
					// its dummy transfer at dot 83. Unlike ordinary lines,
					// this special first line enters HBlank directly after
					// 168+SCX dots, with no extra EnterHBlankDelay event.
					normalDiscard := [...]uint64{0, 0, 2, 3, 4, 4, 6, 7}
					doubleDiscard := [...]uint64{0, 2, 2, 4, 4, 6, 6, 8}
					openDot := uint64(251) + normalDiscard[scx]
					if doubleSpeed {
						openDot = 2 * (251 + doubleDiscard[scx])
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

func TestCGBCompatibilityStartupHBlankReleaseTracksSCX(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			for scx := uint8(0); scx < 8; scx++ {
				t.Run(fmt.Sprintf("%s/double=%t/SCX=%d", model, doubleSpeed, scx), func(t *testing.T) {
					s := scheduler.NewScheduler()
					b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
					b.Map(model)
					p := New(b, s)
					if doubleSpeed {
						s.ChangeSpeed(true)
					}
					b.Set(0x8000, 0x5a)
					b.Write(types.SCX, scx)
					b.Write(types.LCDC, types.Bit7)
					if p.cgbMode || !p.enabled {
						t.Fatal("fixture must be LCD-enabled NCM on CGB")
					}

					// NCM has one initial PPU dot; pixel-transfer dummy
					// begins at T84. At normal speed it runs 166+SCX
					// dots. Artificial NCM double speed retains the old
					// fixed 168-dot dummy transfer (hardware cannot
					// switch NCM into double speed).
					openClock := uint64(250) + uint64(scx)
					if doubleSpeed {
						openClock = 504
					}
					s.Tick(openClock - 1)
					if got := b.Read(0x8000); got != 0xff {
						t.Fatalf("NCM VRAM reopened before scheduled HBlank: got %#02x", got)
					}
					s.Tick(1)
					if got := b.Read(0x8000); got != 0x5a {
						t.Fatalf("NCM VRAM still blocked after HBlank release: got %#02x", got)
					}
				})
			}
		}
	}
}

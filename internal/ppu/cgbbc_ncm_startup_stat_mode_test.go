package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Physical CGB B/C and D/E in DMG-compatible mode keep the CPU-facing
// STAT mode-3 value for one clock at SCX 2/6 after the initial
// HBlank bus release. No VRAM/OAM or IRQ ownership is changed.
func TestCGBCompatibilityStartupSTATModeHandover(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				b.Set(0x8000, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if !p.enabled || p.cgbMode {
					t.Fatal("expected LCD enabled in physical CGB DMG-compatibility mode")
				}
				edge := uint64(250) + uint64(scx)
				s.Tick(edge - 1)
				if got := b.Read(types.STAT) & 3; got != ModeVRAM {
					t.Fatalf("STAT before HBlank edge = %d, want Mode 3", got)
				}
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("VRAM unlocked before HBlank: %02x", got)
				}
				s.Tick(1)
				if got := b.Read(0x8000); got != 0x5a {
					t.Fatalf("VRAM did not unlock at scheduled HBlank: %02x", got)
				}
				if p.mode != ModeHBlank {
					t.Fatalf("internal HBlank mode delayed along with CPU STAT: %d", p.mode)
				}
				wantMode := byte(ModeHBlank)
				if scx&3 == 2 {
					wantMode = ModeVRAM
				}
				if got := b.Read(types.STAT) & 3; got != wantMode {
					t.Fatalf("CPU STAT at first HBlank clock = %d, want %d", got, wantMode)
				}
				s.Tick(1)
				if got := b.Read(types.STAT) & 3; got != ModeHBlank {
					t.Fatalf("CPU STAT did not expose Mode 0 after first HBlank clock: %d", got)
				}
				// The handover phase is fixed at entry, not recomputed
				// when software rewrites SCX after HBlank began.
				b.Write(types.SCX, scx^7)
				if got := b.Read(types.STAT) & 3; got != ModeHBlank {
					t.Fatalf("SCX rewrite reopened STAT Mode 3: %d", got)
				}
			})
		}
	}
}

func TestCGBCompatibilityStartupSTATLatchClearedOnLCDDisable(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			p := New(b, s)
			b.Write(types.SCX, 2)
			b.Write(types.LCDC, types.Bit7)
			s.Tick(252)
			if got := b.Read(types.STAT) & 3; got != ModeVRAM {
				t.Fatalf("expected first-line CPU STAT mode-3 sample, got %d", got)
			}
			b.Write(types.LCDC, 0)
			if p.startupSTATMode3Until != 0 {
				t.Fatal("LCD disable retained startup STAT mode handover clock")
			}
			if got := b.Read(types.STAT) & 3; got != ModeHBlank {
				t.Fatalf("LCD-off STAT retained mode 3: %d", got)
			}
			b.Write(types.LCDC, types.Bit7)
			if got := b.Read(types.STAT) & 3; got != ModeHBlank {
				t.Fatalf("LCD restart incorrectly reused old startup handover: %d", got)
			}
		})
	}
}

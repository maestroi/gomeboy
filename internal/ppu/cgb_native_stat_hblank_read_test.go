package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// CPU-visible STAT, the PPU's own mode, and VRAM/OAM access do not
// necessarily hand over on the same PPU dot. On physical native-colour
// CGB at normal speed the register usually retains Mode 3 until HBlank entry;
// compatibility-mode and double-speed use their existing phases.
func TestNativeCGBFinalPixelSTATModeReadPort(t *testing.T) {
	for _, tc := range []struct {
		model       types.Model
		colourMode  bool
		doubleSpeed bool
		retainMode3 bool
	}{
		{types.DMGABC, false, false, false},
		{types.CGBBC, false, false, false},
		{types.CGBDE, false, false, false},
		{types.CGBBC, true, false, true},
		{types.CGBDE, true, false, true},
		{types.CGBBC, true, true, false},
		{types.CGBDE, true, true, false},
	} {
		t.Run(fmt.Sprintf("%s/colour=%t/double=%t", tc.model, tc.colourMode, tc.doubleSpeed), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = tc.colourMode
			p.mode, p.modeToInt = ModeHBlank, ModeHBlank
			p.lineState = EnterHBlank
			if tc.doubleSpeed {
				s.ChangeSpeed(true)
			}
			edge := p.enterHBlankDelay()
			s.ScheduleEvent(scheduler.PPUHandleVisualLine, edge)
			// The scheduler expresses PPU dots as two CPU clocks at
			// CGB double speed.
			clockEdge := edge
			if tc.doubleSpeed {
				clockEdge *= 2
			}
			if got := b.Read(types.STAT) & 3; (got == ModeVRAM) != tc.retainMode3 {
				t.Fatalf("CPU STAT before HBlank entry = %d, retain Mode 3=%t", got, tc.retainMode3)
			}
			if p.mode != ModeHBlank {
				t.Fatalf("internal PPU mode shifted: got %d", p.mode)
			}
			if clockEdge > 1 {
				s.Tick(clockEdge - 1)
				if got := b.Read(types.STAT) & 3; (got == ModeVRAM) != tc.retainMode3 {
					t.Fatalf("CPU STAT immediately before HBlank entry = %d, retain Mode 3=%t", got, tc.retainMode3)
				}
			}
			s.Tick(1)
			if got := b.Read(types.STAT) & 3; got != ModeHBlank {
				t.Fatalf("CPU STAT after HBlank entry = %d, want Mode 0", got)
			}
			if p.mode != ModeHBlank {
				t.Fatalf("internal PPU mode after HBlank entry = %d", p.mode)
			}
		})
	}
}


// SCX fine-scroll phases 3 and 7 have the earlier CPU STAT Mode-0
// observation edge, unlike other native-colour CGB normal-speed phases.
// This only affects reads: the PPU internal mode and HBlank event remain
// at the same physical dot for all eight scroll phases.
func TestNativeCGBHBlankSTATReadFineScrollPhases(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = true
				p.mode, p.modeToInt = ModeHBlank, ModeHBlank
				p.lineState = EnterHBlank
				b.Write(types.SCX, scx)
				s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())

				want := byte(ModeVRAM)
				if scx&3 == 3 {
					want = ModeHBlank
				}
				for dot := uint64(0); dot < 2; dot++ {
					if got := b.Read(types.STAT) & 3; got != want {
						t.Fatalf("dot %d CPU STAT=%d, want %d", dot, got, want)
					}
					if p.mode != ModeHBlank {
						t.Fatalf("dot %d internal PPU mode=%d, want Mode 0", dot, p.mode)
					}
					s.Tick(1)
				}
				if got := b.Read(types.STAT) & 3; got != ModeHBlank {
					t.Fatalf("STAT after HBlank entry=%d, want Mode 0", got)
				}
			})
		}
	}
}

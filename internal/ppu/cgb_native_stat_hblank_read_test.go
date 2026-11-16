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
// CGB at normal speed the register retains Mode 3 until HBlank entry;
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
			if got := b.Read(types.STAT) & 3; (got == ModeVRAM) != tc.retainMode3 {
				t.Fatalf("CPU STAT before HBlank entry = %d, retain Mode 3=%t", got, tc.retainMode3)
			}
			if p.mode != ModeHBlank {
				t.Fatalf("internal PPU mode shifted: got %d", p.mode)
			}
			if edge > 1 {
				s.Tick(edge - 1)
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

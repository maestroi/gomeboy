package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// At dot 80, native normal-speed CGB transfers bus ownership and internal
// PPU mode to Mode 3, but the CPU-facing STAT read port still returns
// the final Mode-2 sample for that CPU clock. All other profiles retain
// their established observation boundary.
func TestNativeCGBStartTransferSTATModeReadPort(t *testing.T) {
	for _, tc := range []struct {
		model       types.Model
		colourMode  bool
		doubleSpeed bool
		want        uint8
	}{
		{types.DMGABC, false, false, ModeVRAM},
		{types.CGBBC, false, false, ModeVRAM},
		{types.CGBDE, false, false, ModeVRAM},
		{types.CGBBC, true, false, ModeOAM},
		{types.CGBDE, true, false, ModeOAM},
		{types.CGBBC, true, true, ModeVRAM},
		{types.CGBDE, true, true, ModeVRAM},
	} {
		t.Run(fmt.Sprintf("%s/colour=%t/double=%t", tc.model, tc.colourMode, tc.doubleSpeed), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = tc.colourMode
			p.mode, p.modeToInt = ModeOAM, ModeOAM
			p.lineState = StartPixelTransfer
			p.lineDot = s.Cycle()
			b.Set(0x8000, 0x5a)
			if tc.doubleSpeed {
				s.ChangeSpeed(true)
			}
			s.ScheduleEvent(scheduler.PPUHandleVisualLine, 80)
			clockEdge := uint64(80)
			if tc.doubleSpeed {
				clockEdge *= 2
			}
			s.Tick(clockEdge)
			if p.mode != ModeVRAM {
				t.Fatalf("internal PPU mode after dot-80 entry = %d, want Mode 3", p.mode)
			}
			if got := b.Read(types.STAT) & 3; got != tc.want {
				t.Fatalf("CPU STAT on first transfer clock = %d, want %d", got, tc.want)
			}
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM unlocked at the internal Mode-3 transfer: %02x", got)
			}
			s.Tick(1)
			if got := b.Read(types.STAT) & 3; got != ModeVRAM {
				t.Fatalf("CPU STAT after first transfer clock = %d, want Mode 3", got)
			}
		})
	}
}

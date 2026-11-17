package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Physical native-colour CGB at normal speed starts its Mode-2 OAM
// search while the CPU-visible STAT read port still reports the final
// Mode-0 clock. No other model/speed profile gets this observation lag.
// Check both normal visible-line and frame-rollover (LY=0) entry.
func TestNativeCGBStartOAMSTATReadPort(t *testing.T) {
	for _, tc := range []struct {
		model       types.Model
		colourMode  bool
		doubleSpeed bool
		want        uint8
	}{
		{types.DMGABC, false, false, ModeOAM},
		{types.CGBBC, false, false, ModeOAM},
		{types.CGBDE, false, false, ModeOAM},
		{types.CGBBC, true, false, ModeHBlank},
		{types.CGBDE, true, false, ModeHBlank},
		{types.CGBBC, true, true, ModeOAM},
		{types.CGBDE, true, true, ModeOAM},
	} {
		for _, ly := range []uint8{0, 1} {
			t.Run(fmt.Sprintf("%s/colour=%t/double=%t/LY=%d", tc.model, tc.colourMode, tc.doubleSpeed, ly), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(tc.model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = tc.colourMode
				p.ly = ly
				p.mode, p.modeToInt = ModeHBlank, ModeHBlank
				if ly == 0 {
					p.mode, p.modeToInt = ModeVBlank, ModeVBlank
				}
				p.lineState = StartOAMScan
				b.Set(0xfe00, 0x5a)
				if tc.doubleSpeed {
					s.ChangeSpeed(true)
				}
				p.handleVisualLine() // first OAM scan entry at dot 0
				if p.mode != ModeOAM {
					t.Fatalf("internal PPU mode at dot 0 = %d, want Mode 2", p.mode)
				}
				if p.oamScanIndex != 1 {
					t.Fatalf("scan did not read OAM entry 0, index = %d", p.oamScanIndex)
				}
				if got := b.Read(types.STAT) & 3; got != tc.want {
					t.Fatalf("CPU STAT at dot 0 = %d, want %d", got, tc.want)
				}
				if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("OAM bus unlocked during Mode 2: %02x", got)
				}
				s.Tick(1)
				if got := b.Read(types.STAT) & 3; got != ModeOAM {
					t.Fatalf("CPU STAT after first OAM clock = %d, want Mode 2", got)
				}
			})
		}
	}
}

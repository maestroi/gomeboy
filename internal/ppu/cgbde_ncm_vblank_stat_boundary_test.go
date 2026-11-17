package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// At frame rollover the last VBlank line advances through the LY/OAM
// handover before the new frame's first Mode-2 OAM scan. Physical CGB D/E
// in normal-speed DMG compatibility mode keeps Mode 1 visible through
// this interval; physical B/C and DMG still expose the short Mode 0
// interval. The internal scan, LY and next Mode 2 schedule are unchanged.
func TestCGBDENCMSTATModeAtVBlankToMode2(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		colour bool
		double bool
		want byte
	}{
		{types.DMGABC, false, false, ModeHBlank},
		{types.CGBBC, false, false, ModeHBlank},
		{types.CGBDE, false, false, ModeVBlank},
		{types.CGBDE, true, false, ModeVBlank},
		// Artificial compatibility-mode double speed retains its
		// previous behavior; real NCM cannot enter double speed.
		{types.CGBDE, false, true, ModeHBlank},
	} {
		t.Run(fmt.Sprintf("%s/colour=%t/double=%t", tc.model, tc.colour, tc.double),func(t *testing.T){
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b,s)
			p.enabled = true
			p.cgbMode = tc.colour
			if tc.double {s.ChangeSpeed(true)}
			p.ly = 153
			p.mode, p.modeToInt = ModeVBlank, ModeVBlank
			p.offscreenLineState = EndFrame
			p.handleOffscreenLine() // frame rollover, starts LY/OAM handover

			ticks := uint64(1)
			if tc.double {ticks = 2}
			s.Tick(2*ticks) // visible LY update
			if got := b.Read(types.STAT)&3; got != ModeVBlank {
				t.Fatalf("STAT before final HBlank-end phase = %d, want Mode 1", got)
			}
			if got := b.Read(types.LY); got != 0 {
				t.Fatalf("visible LY at next-frame handover = %d, want 0",got)
			}
			s.Tick(ticks) // HBlankEnd, one clock before Mode 2
			if got := b.Read(types.STAT)&3; got != tc.want {
				t.Fatalf("STAT during frame-to-Mode-2 handover = %d, want %d",got,tc.want)
			}
			if p.lineState != StartOAMScan || p.oamScanIndex != 0 {
				t.Fatalf("expected pre-Mode-2 OAM scan state; state=%d entry=%d",
					p.lineState,p.oamScanIndex)
			}
			s.Tick(ticks) // real Mode 2 OAM scan event
			if p.oamScanIndex != 1 || p.mode != ModeOAM {
				t.Fatalf("Mode-2 PPU OAM scan did not begin: index=%d mode=%d",p.oamScanIndex,p.mode)
			}
			// AGE stat-mode's normal-speed native CGB read trace shows
			// the first CPU-facing STAT clock still in Mode 0 even
			// though the PPU is now scanning OAM in internal Mode 2.
			wantFirst := byte(ModeOAM)
			if tc.colour && !tc.double {
				wantFirst = ModeHBlank
				if tc.model == types.CGBDE {
					wantFirst = ModeVBlank // D/E has no last Mode-0 M-cycle
				}
			}
			if got := b.Read(types.STAT)&3; got != wantFirst {
				t.Fatalf("STAT on first OAM scan clock=%d, want %d",got,wantFirst)
			}
			if tc.colour && !tc.double {
				s.Tick(1)
				if got := b.Read(types.STAT)&3; got != ModeOAM {
					t.Fatalf("STAT failed to expose Mode 2 on following clock: %d",got)
				}
			}
		})
	}
}

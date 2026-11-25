package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

// Mealybug m3_window_timing retunes postOffscreenStall when the window
// opens, so mid-Mode-3 BGP writes keep the correct leading-white count.
func TestWindowTriggerRetunesPostOffscreenStall(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wx, lx, ly uint8
		stallIn    uint8
		wantStall  uint8
		wantSettle uint8
	}{
		{"early WX trims BGP stall", 3, 4, 1, 11, 10, 0},
		{"LY0 WX0 uses 6-dot stall", 0, 0, 0, 7, 6, 0},
		{"WX7 at stall boundary stacks fetch", 7, 8, 1, 11, 17, 0},
		{"exact WX boundary adds BGP settle", 9, 10, 1, 0, 0, 1},
		{"mid-line re-open skips stall retune", 9, 50, 1, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(types.DMGABC)
			p.wx, p.lx, p.ly = tc.wx, tc.lx, tc.ly
			p.postOffscreenStall = tc.stallIn
			p.checkWindowTriggerWX()
			if !p.winTriggerWx {
				t.Fatalf("window did not trigger")
			}
			if p.postOffscreenStall != tc.wantStall {
				t.Fatalf("stall=%d want %d", p.postOffscreenStall, tc.wantStall)
			}
			if p.bgpSettleExtra != tc.wantSettle {
				t.Fatalf("settle=%d want %d", p.bgpSettleExtra, tc.wantSettle)
			}
		})
	}
}

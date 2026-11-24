package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

func TestWindowWX6DMGStaysClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    types.Model
		wx, lx   uint8
		wantTrig bool
	}{
		{"DMG WX=6 LX=7 closed", types.DMGABC, 6, 7, false},
		{"DMG WX=6 LX=8 closed", types.DMGABC, 6, 8, false},
		{"DMG WX=7 LX=8 opens", types.DMGABC, 7, 8, true},
		{"DMG far-late WX poke stays closed", types.DMGABC, 0x50, 94, false},
		{"CGB WX=6 LX=7 opens", types.CGBDE, 6, 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(tc.model)
			p.wx, p.lx = tc.wx, tc.lx
			p.checkWindowTriggerWX()
			if p.winTriggerWx != tc.wantTrig {
				t.Fatalf("trigger=%t want %t", p.winTriggerWx, tc.wantTrig)
			}
		})
	}
}

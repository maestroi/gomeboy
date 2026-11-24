package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

// Mid-window WIN_EN clear must restart BG fetch cleanly. Remapping
// window fetcher states with -=8 can pair window low-plane data with a
// BG high plane and push a corrupt tile (Mealybug win_en x=60..64).
func TestWinEnClearRestartsBGFetchCleanly(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(model)
			p.winEnabled = true
			p.winTriggerWy = true
			p.winTriggerWx = true
			p.mode = ModeVRAM
			p.fetcherState = WinGetTileDataLowT2
			p.fetcherData = [2]uint8{0xaa, 0x55}
			p.fetcherTileAttr = 0x12
			p.b.Write(types.LCDC, types.Bit7) // WIN_EN off
			if p.winEnabled {
				t.Fatal("WIN_EN clear did not update winEnabled")
			}
			p.stepPixelFetcher()
			if p.winTriggerWx {
				t.Fatal("clean abort must clear WX latch")
			}
			if p.fetcherState != BGGetTileNoT2 && p.fetcherState != BGGetTileDataLowT1 &&
				p.fetcherState != BGGetTileNoT1 {
				// BGWinActivating falls through into BGGetTileNoT1 which
				// advances; accept early BG states only.
				if p.fetcherState < BGGetTileNoT1 || p.fetcherState >= WinActivating {
					t.Fatalf("expected clean BG restart, got %v", p.fetcherState)
				}
			}
			if p.fetcherData != [2]uint8{} || p.fetcherTileAttr != 0 {
				t.Fatalf("stale window fetch data survived abort: data=%v attr=%#x",
					p.fetcherData, p.fetcherTileAttr)
			}
		})
	}
}

package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

// Mealybug: WIN_EN clear mid-Mode-3 finishes the current window tile first.
func TestWinEnClearDoesNotAbortInFlightWindowFetch(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.winTriggerWx = true
	p.winEnabled = false
	p.fetcherState = WinGetTileDataLowT1
	p.stepPixelFetcher()
	if p.fetcherState != WinGetTileDataLowT2 {
		t.Fatalf("in-flight window fetch aborted early: state=%v", p.fetcherState)
	}
}

func TestWinEnClearSwitchesToBGAtTileBoundary(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.winTriggerWx = true
	p.winEnabled = false
	p.scxToDiscard = 5
	p.scxDiscarded = 2
	p.fetcherState = BGWinActivating
	p.stepPixelFetcher()
	if p.winTriggerWx {
		t.Fatal("expected winTriggerWx cleared at tile boundary")
	}
	if p.scxToDiscard != 0 || p.scxDiscarded != 0 {
		t.Fatalf("SCX fine scroll should be ignored after window→BG: discard=%d/%d",
			p.scxDiscarded, p.scxToDiscard)
	}
	if p.fetcherState != BGGetTileNoT2 && p.fetcherState != BGGetTileNoT1 {
		// step recurses into BGGetTileNoT1 then advances
		if p.fetcherState < BGGetTileNoT1 || p.fetcherState > BGWinPushPixels {
			t.Fatalf("expected BG fetch after boundary, got %v", p.fetcherState)
		}
	}
}

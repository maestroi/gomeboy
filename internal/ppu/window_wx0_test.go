package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newWindowWXTriggerTestPPU(model types.Model) *PPU {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	p := New(b, s)
	p.enabled = true
	p.winEnabled = true
	p.winTriggerWy = true
	p.wly = 255
	p.winFetcherX = 11
	p.bgFIFO.Size = 8
	p.fetcherState = BGGetTileNoT1
	return p
}

// WX=0 hands over to the window before the BG fine-scroll discard at LX=0.
// Those discarded pixels must instead come from the window to give the
// documented SCX-dependent left shift.
func TestWindowWXZeroStartsBeforeFineScroll(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		for _, scx := range []uint8{0, 1, 3, 7} {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				p := newWindowWXTriggerTestPPU(model)
				p.wx = 0
				p.lx = 0
				p.scx = scx
				p.scxToDiscard = scx

				p.checkWindowTriggerWX()
				if !p.winTriggerWx || p.wly != 0 {
					t.Fatalf("WX=0 did not trigger at LX=0: trigger=%t WLY=%d", p.winTriggerWx, p.wly)
				}
				if p.bgFIFO.Size != 0 || p.fetcherState != WinActivating || p.winFetcherX != 0 {
					t.Fatalf("window fetch did not restart: FIFO=%d state=%v fetchX=%d",
						p.bgFIFO.Size, p.fetcherState, p.winFetcherX)
				}
				if p.scxToDiscard != scx || p.scxDiscarded != 0 {
					t.Fatalf("fine-scroll discard phase changed: pending=%d consumed=%d want=%d",
						p.scxToDiscard, p.scxDiscarded, scx)
				}
				p.checkWindowTriggerWX()
				if p.wly != 0 {
					t.Fatalf("window line incremented twice: WLY=%d", p.wly)
				}
			})
		}
	}
}

func TestWindowWXTriggerGuardsAndOrdinaryPhase(t *testing.T) {
	for _, tc := range []struct {
		name string
		wx, lx uint8
		yTriggered, enabled, want bool
	}{
		{"WX0 before initial dot, Y false", 0, 0, false, true, false},
		{"WX0 disabled", 0, 0, true, false, false},
		{"WX0 late", 0, 1, true, true, false},
		{"WX1 before", 1, 1, true, true, false},
		{"WX1 boundary", 1, 2, true, true, true},
		{"WX7 before", 7, 7, true, true, false},
		{"WX7 boundary", 7, 8, true, true, true},
		{"WX165 boundary", 165, 166, true, true, true},
		{"WX166 unchanged", 166, 167, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(types.DMGABC)
			p.wx, p.lx = tc.wx, tc.lx
			p.winTriggerWy = tc.yTriggered
			p.winEnabled = tc.enabled
			p.checkWindowTriggerWX()
			if p.winTriggerWx != tc.want {
				t.Fatalf("WX=%d LX=%d: trigger=%t want=%t", tc.wx, tc.lx, p.winTriggerWx, tc.want)
			}
			if !tc.want && (p.wly != 255 || p.bgFIFO.Size != 8) {
				t.Fatalf("rejected window trigger mutated state: WLY=%d FIFO=%d", p.wly, p.bgFIFO.Size)
			}
		})
	}
}

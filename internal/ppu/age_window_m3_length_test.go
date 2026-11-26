package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func measureMode3Dots(t *testing.T, model types.Model, cgbMode bool, wx, wy, scx uint8, winEn bool) (dots int, winTriggered bool) {
	t.Helper()
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	p := New(b, s)
	p.enabled = true
	p.cgbMode = cgbMode
	p.winEnabled = winEn
	p.bgEnabled = false
	p.wy = wy
	p.wx = wx
	p.scx = scx
	p.wly = 255
	p.objSize = 8
	p.videoOutput = false
	p.ly = 0
	p.lineState = StartOAMScan
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)

	start := -1
	for i := 0; i < 2000; i++ {
		s.Tick(1)
		if start < 0 && p.mode == ModeVRAM {
			start = int(s.Cycle())
		}
		if start >= 0 && p.lineState == EnterHBlank {
			return int(s.Cycle()) - start, p.winTriggerWx
		}
	}
	t.Fatal("no Mode3 end")
	return 0, false
}

// AGE window STAT shows Mode 3 lengthens when WY has latched and WIN_EN is
// set, even with WX=167 so the window never opens for pixel push. Native CGB
// colour already retains Mode 3 through enterHBlankDelay, so it uses a
// smaller transfer stall than DMG / CGB-NCM.
func TestWindowMode3StallExtendsTransfer(t *testing.T) {
	cases := []struct {
		name    string
		model   types.Model
		cgbMode bool
		wantAdd int
	}{
		{name: "DMG", model: types.DMGABC, cgbMode: false, wantAdd: 4},
		{name: "CGB-NCM", model: types.CGBBC, cgbMode: false, wantAdd: 4},
		{name: "CGB-colour", model: types.CGBBC, cgbMode: true, wantAdd: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Base: WIN_EN on but WY never latches — no windowMode3Stall and
			// no BGP post-offscreen stall (winEnabled). withWin: WY latched.
			base, opened := measureMode3Dots(t, tc.model, tc.cgbMode, 167, 255, 0, true)
			if opened {
				t.Fatalf("window opened without WY latch")
			}
			withWin, opened := measureMode3Dots(t, tc.model, tc.cgbMode, 167, 0, 0, true)
			if opened {
				t.Fatalf("WX=167 must not open the window for pixel push")
			}
			if got := withWin - base; got != tc.wantAdd {
				t.Fatalf("Mode3 length delta: got %d (base=%d with=%d), want %d",
					got, base, withWin, tc.wantAdd)
			}
		})
	}
}

func TestWindowMode3StallSkippedWithoutWYLatch(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	p.enabled = true
	p.winEnabled = true
	p.wy = 10 // LY=0 will not latch
	p.wx = 167
	p.ly = 0
	p.lineState = StartOAMScan
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)

	for i := 0; i < 2000; i++ {
		s.Tick(1)
		if p.lineState == PixelTransferDummy || p.lineState == PixelTransferLX {
			if p.windowMode3Stall != 0 {
				t.Fatalf("stall set without WY latch: %d", p.windowMode3Stall)
			}
		}
		if p.lineState == EnterHBlank {
			return
		}
	}
	t.Fatal("no Mode3 end")
}

// WX=7 opens at the first stall dot (lx==8). Stall must not advance the
// restarted window fetcher, or Mode 3 ends ~windowMode3Stall dots early
// compared with WX=8 (AGE window STAT expects equal Mode-3 length).
func TestWindowMode3WX7MatchesWX8(t *testing.T) {
	for _, tc := range []struct {
		name    string
		model   types.Model
		cgbMode bool
	}{
		{name: "DMG", model: types.DMGABC, cgbMode: false},
		{name: "CGB-NCM", model: types.CGBBC, cgbMode: false},
		{name: "CGB-colour", model: types.CGBBC, cgbMode: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wx7, opened7 := measureMode3Dots(t, tc.model, tc.cgbMode, 7, 0, 0, true)
			if !opened7 {
				t.Fatal("WX=7 must open the window")
			}
			wx8, opened8 := measureMode3Dots(t, tc.model, tc.cgbMode, 8, 0, 0, true)
			if !opened8 {
				t.Fatal("WX=8 must open the window")
			}
			if wx7 != wx8 {
				t.Fatalf("Mode3 length WX=7 (%d) != WX=8 (%d)", wx7, wx8)
			}
		})
	}
}

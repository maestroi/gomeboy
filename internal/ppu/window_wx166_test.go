package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// WX=166 is a valid final-pixel window position on physical CGB hardware.
// DMG has a separate full-line quirk and must not take the CGB fetch path.
func TestCGBWX166WindowFetchAtFinalPixel(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		trigger bool
	}{
		{types.DMGABC, false},
		{types.CGBBC, true},
		{types.CGBDE, true},
	} {
		for _, lx := range []uint8{166, 167} {
			t.Run(fmt.Sprintf("%s/LX=%d", tc.model, lx), func(t *testing.T) {
				p := newWindowWXTriggerTestPPU(tc.model)
				p.wx = 166
				p.lx = lx
				p.checkWindowTriggerWX()

				want := tc.trigger && lx == 167
				if p.winTriggerWx != want {
					t.Fatalf("WX=166 at LX=%d: triggered=%t want=%t",
						lx, p.winTriggerWx, want)
				}
				if want {
					if p.wly != 0 || p.bgFIFO.Size != 0 ||
						p.fetcherState != WinActivating || p.winFetcherX != 0 {
						t.Fatalf("CGB final-pixel handover: WLY=%d fifo=%d state=%s fetchX=%d",
							p.wly, p.bgFIFO.Size, p.fetcherState, p.winFetcherX)
					}
					p.checkWindowTriggerWX()
					if p.wly != 0 {
						t.Fatalf("window line incremented twice: WLY=%d", p.wly)
					}
				} else if p.wly != 255 || p.bgFIFO.Size != 8 {
					t.Fatalf("rejected WX=166 changed fetch state: WLY=%d fifo=%d",
						p.wly, p.bgFIFO.Size)
				}
			})
		}
	}
}

func TestCGBWX166RejectsMissingYOrLCDCAndWX167(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, tc := range []struct {
			name string
			wx uint8
			yMatch bool
			windowEnabled bool
		}{
			{"no Y match", 166, false, true},
			{"disabled window", 166, true, false},
			{"WX=167 offscreen", 167, true, true},
		} {
			t.Run(fmt.Sprintf("%s/%s", model, tc.name), func(t *testing.T) {
				p := newWindowWXTriggerTestPPU(model)
				p.wx, p.lx = tc.wx, 167
				p.winTriggerWy = tc.yMatch
				p.winEnabled = tc.windowEnabled
				p.checkWindowTriggerWX()
				if p.winTriggerWx || p.wly != 255 || p.bgFIFO.Size != 8 {
					t.Fatalf("ineligible final-pixel window changed state: X=%t WLY=%d fifo=%d",
						p.winTriggerWx, p.wly, p.bgFIFO.Size)
				}
			})
		}
	}
}

// Exercise the real per-dot PPU pipeline instead of only checking a trigger
// predicate. WX=166 must restart window tile fetching *before* the final
// LCD pixel, which holds Mode 3 open longer than an offscreen WX=167.
func windowMode3EndAtWX(t *testing.T, model types.Model, cgbMode, doubleSpeed bool, wx uint8) (uint64, *PPU) {
	t.Helper()
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	p := New(b, s)
	p.enabled = true
	p.cgbMode = cgbMode
	p.winEnabled = true
	p.wy = 0
	p.wx = wx
	p.wly = 255
	p.objSize = 8
	p.videoOutput = false
	p.lineState = StartOAMScan
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)

	for i := 0; i < 1600; i++ {
		s.Tick(1)
		if p.lineState == EnterHBlank {
			if p.lx != 168 {
				t.Fatalf("Mode 3 ended with LX=%d (want 168)", p.lx)
			}
			return s.Cycle(), p
		}
	}
	t.Fatalf("Mode 3 did not finish for model=%s wx=%d", model, wx)
	return 0, nil
}

func TestCGBWX166LastPixelWindowExtendsMode3(t *testing.T) {
	for _, tc := range []struct {
		name string
		model types.Model
		cgbMode bool
		doubleSpeed bool
	}{
		{"CGB B/C color", types.CGBBC, true, false},
		{"CGB D/E color", types.CGBDE, true, false},
		{"CGB B/C NCM", types.CGBBC, false, false},
		{"CGB D/E NCM", types.CGBDE, false, false},
		{"CGB B/C double", types.CGBBC, true, true},
		{"CGB D/E double", types.CGBDE, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offscreenEnd, offscreen := windowMode3EndAtWX(t, tc.model, tc.cgbMode, tc.doubleSpeed, 167)
			windowEnd, window := windowMode3EndAtWX(t, tc.model, tc.cgbMode, tc.doubleSpeed, 166)
			if offscreen.winTriggerWx || offscreen.wly != 255 {
				t.Fatalf("offscreen WX=167 unexpectedly triggered window: X=%t WLY=%d",
					offscreen.winTriggerWx, offscreen.wly)
			}
			if !window.winTriggerWx || window.wly != 0 {
				t.Fatalf("WX=166 did not fetch last window pixel: X=%t WLY=%d",
					window.winTriggerWx, window.wly)
			}
			if windowEnd <= offscreenEnd {
				t.Fatalf("WX=166 should delay Mode 3: end=%d offscreen=%d",
					windowEnd, offscreenEnd)
			}
		})
	}
}

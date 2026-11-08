package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newWindowYLatchTestPPU(model types.Model, doubleSpeed bool) *PPU {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	p := New(b, s)
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	p.enabled = true
	p.wly = 255
	return p
}

// The PPU compares WY to LY at the beginning of a scanline independently
// of whether LCDC currently enables the window. The later WX handover is
// still gated on the window-enable bit.
func TestWindowWYScanlineLatchIndependentOfLCDC(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			if doubleSpeed && !model.IsCGB() {
				continue
			}
			for _, enabledAtLineStart := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/double=%t/window=%t", model, doubleSpeed, enabledAtLineStart), func(t *testing.T) {
					p := newWindowYLatchTestPPU(model, doubleSpeed)
					p.ly = 12
					p.wy = 12
					p.winEnabled = enabledAtLineStart
					p.lineState = StartOAMScan
					p.handleVisualLine()
					if !p.winTriggerWy {
						t.Fatal("WY==LY did not latch at scanline start")
					}

					// WY changes after the comparator has latched. The
					// horizontal window enable gate remains independent.
					p.wy = 80
					p.winEnabled = true
					p.wx = 7
					p.lx = 8
					p.checkWindowTriggerWX()
					if !p.winTriggerWx || p.wly != 0 {
						t.Fatalf("latched window did not start at WX: trigger=%t WLY=%d",
							p.winTriggerWx, p.wly)
					}
					p.checkWindowTriggerWX()
					if p.wly != 0 {
						t.Fatalf("window row incremented twice: %d", p.wly)
					}
				})
			}
		}
	}
}

// A write to WY that makes it equal LY during Mode 3 cannot retroactively
// satisfy a comparison that occurred at the start of the scanline. The
// comparator can latch again at the *next* scanline start.
func TestWindowWYChangedDuringMode3WaitsUntilNextLine(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			p := newWindowYLatchTestPPU(model, false)
			p.winEnabled = true
			p.ly = 14
			p.wy = 15
			p.lineState = StartOAMScan
			p.handleVisualLine()
			if p.winTriggerWy {
				t.Fatal("mismatching WY latched at line start")
			}

			p.wy = 14 // written after the scanline-start comparison
			p.lineState = PixelTransferLX
			p.lx = 8
			p.bgFIFO.Size = 1
			p.fetcherState = BGGetTileNoT1
			p.handleVisualLine()
			if p.winTriggerWy {
				t.Fatal("Mode-3 pixel output erroneously rechecked WY==LY")
			}
			p.wx = 7
			p.lx = 8
			p.checkWindowTriggerWX()
			if p.winTriggerWx || p.wly != 255 {
				t.Fatalf("window started despite missing Y latch: X=%t WLY=%d",
					p.winTriggerWx, p.wly)
			}

			p.ly = 15
			p.wy = 15
			p.oamScanIndex = 0
			p.lineState = StartOAMScan
			p.handleVisualLine()
			if !p.winTriggerWy {
				t.Fatal("WY comparator did not latch on the following scanline")
			}
		})
	}
}

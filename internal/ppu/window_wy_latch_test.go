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


// CGB hardware clears a previously latched WY match when LCDC.5 falls,
// even while running a DMG-only cartridge. DMG retains the latch until
// frame reset. A later WY==LY match is required on CGB to restart it.
func TestCGBWindowDisableClearsYCondition(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		cgbCartridge bool
	}{
		{types.DMGABC, false},
		{types.CGBBC, false},
		{types.CGBDE, false},
		{types.CGBBC, true},
		{types.CGBDE, true},
	} {
		t.Run(fmt.Sprintf("%s/cgbCartridge=%t", tc.model, tc.cgbCartridge), func(t *testing.T) {
			s := scheduler.NewScheduler()
			rom := make([]byte, 0x8000)
			if tc.cgbCartridge {
				rom[0x143] = 0x80
			}
			b := gbio.NewBus(s, rom)
			b.Map(tc.model)
			p := New(b, s)
			b.Write(types.LCDC, types.Bit7|types.Bit5)
			if !p.enabled || !p.winEnabled {
				t.Fatal("LCD and Window must be enabled for the test")
			}
			p.ly = 20
			p.wy = 20
			p.oamScanIndex = 0
			p.lineState = StartOAMScan
			p.handleVisualLine()
			if !p.winTriggerWy {
				t.Fatal("initial WY=LY match did not latch")
			}

			b.Write(types.LCDC, types.Bit7) // LCD stays on; window disabled
			keepLatch := !tc.model.IsCGB()
			if p.winTriggerWy != keepLatch {
				t.Fatalf("window disable changed WY latch incorrectly: got %t want %t",
					p.winTriggerWy, keepLatch)
			}

			p.wy = 5
			b.Write(types.LCDC, types.Bit7|types.Bit5)
			p.wx = 7
			p.lx = 8
			p.checkWindowTriggerWX()
			if p.winTriggerWx != keepLatch {
				t.Fatalf("unexpected window restart before a new WY match: got %t want %t",
					p.winTriggerWx, keepLatch)
			}

			// At the next line whose LY equals WY, CGB can latch again.
			p.ly = 21
			p.wy = 21
			p.oamScanIndex = 0
			p.lineState = StartOAMScan
			p.handleVisualLine()
			if !p.winTriggerWy {
				t.Fatal("window did not relatch after a later WY=LY match")
			}
		})
	}
}

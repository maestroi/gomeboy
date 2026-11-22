package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// An entire 160-pixel transfer has a final-buffered-pixel interval before
// the coarse HBlank release event. Probe whether DMG and CGB compatibility
// mode keep their CPU-visible Mode-3 value through this interval, while the
// PPU's interrupt and VRAM/OAM ownership transitions remain unchanged.
func TestMonochromeFullLineSTATModeBeforeHBlank(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		double bool
	}{
		{types.DMGABC, false},
		{types.CGBBC, false},
		{types.CGBDE, false},
		{types.CGBBC, true},
		{types.CGBDE, true},
	} {
		t.Run(fmt.Sprintf("%s/double=%t", tc.model, tc.double), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = false
			p.mode, p.modeToInt = ModeHBlank, ModeHBlank
			p.lineState = EnterHBlank
			p.lx = 168 // full pixel line, not abbreviated LCD-on line
			p.status = types.Bit3
			if tc.double {
				s.ChangeSpeed(true)
			}
			b.Set(types.IF, 0)
			edge := p.enterHBlankDelay()
			s.ScheduleEvent(scheduler.PPUHandleVisualLine, edge)
			clocks := edge
			if tc.double {
				clocks *= 2
			}
			for i := uint64(0); i < clocks; i++ {
				if got := b.Read(types.STAT) & 3; got != ModeVRAM {
					t.Fatalf("clock %d STAT mode=%d, want Mode 3", i, got)
				}
				if p.mode != ModeHBlank || p.modeToInt != ModeHBlank {
					t.Fatalf("clock %d internal PPU mode/irq source changed", i)
				}
				if got := b.Get(types.IF) & gbio.LCDINT; got != 0 {
					t.Fatalf("clock %d Mode-0 STAT IRQ delivered early", i)
				}
				s.Tick(1)
			}
			if got := b.Read(types.STAT) & 3; got != ModeHBlank {
				t.Fatalf("after HBlank release STAT mode=%d, want Mode 0", got)
			}
			if got := b.Get(types.IF) & gbio.LCDINT; got == 0 {
				t.Fatal("Mode-0 STAT IRQ did not arrive at HBlank release")
			}
		})
	}
}

func TestMonochromeStartupSTATDoesNotInheritFullLineHold(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	p.enabled = true
	p.cgbMode = false
	p.mode, p.modeToInt = ModeHBlank, ModeHBlank
	p.lineState = EnterHBlank
	p.lx = 0 // abbreviated first LCD-on scanline
	if got := b.Read(types.STAT) & 3; got != ModeHBlank {
		t.Fatalf("startup STAT mode=%d, should retain Mode 0", got)
	}
}

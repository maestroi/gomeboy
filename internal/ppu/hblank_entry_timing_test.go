package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newHBlankEntryTimingTest(t *testing.T, model types.Model, cgbMode bool) (*PPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()

	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	p := New(b, s)
	b.Map(model)

	p.enabled = true
	p.cgbMode = cgbMode
	p.mode = ModeHBlank
	p.modeToInt = ModeHBlank
	p.status = types.Bit3 // enable Mode-0 STAT interrupt
	p.lineState = EnterHBlank
	p.lineDot = s.Cycle()
	b.Set(types.IF, 0)

	s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())
	return p, b, s
}

func TestHBlankSTATEntryEdgeTiming(t *testing.T) {
	tests := []struct {
		name    string
		model   types.Model
		cgbMode bool
		delay   uint64
	}{
		{name: "DMG", model: types.DMGABC, delay: 1},
		{name: "CGB B/C", model: types.CGBBC, cgbMode: true, delay: 2},
		{name: "CGB D/E", model: types.CGBDE, cgbMode: true, delay: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, b, s := newHBlankEntryTimingTest(t, tc.model, tc.cgbMode)

			for dot := uint64(1); dot < tc.delay; dot++ {
				s.Tick(1)
				if got := b.Get(types.IF) & gbio.LCDINT; got != 0 {
					t.Fatalf("Mode-0 STAT interrupt raised at dot %d, want dot %d", dot, tc.delay)
				}
			}

			s.Tick(1)
			if got := b.Get(types.IF) & gbio.LCDINT; got == 0 {
				t.Fatalf("Mode-0 STAT interrupt not raised at dot %d", tc.delay)
			}
		})
	}
}

func TestCGBDoubleSpeedHBlankEntryKeepsOnePPUDotDelay(t *testing.T) {
	p, b, s := newHBlankEntryTimingTest(t, types.CGBBC, true)
	s.ChangeSpeed(true)

	// Re-schedule after changing speed so one PPU dot maps to two CPU clocks.
	s.DescheduleEvent(scheduler.PPUHandleVisualLine)
	b.Set(types.IF, 0)
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())

	s.Tick(1)
	if got := b.Get(types.IF) & gbio.LCDINT; got != 0 {
		t.Fatal("double-speed CGB Mode-0 STAT interrupt raised before one PPU dot elapsed")
	}
	s.Tick(1)
	if got := b.Get(types.IF) & gbio.LCDINT; got == 0 {
		t.Fatal("double-speed CGB Mode-0 STAT interrupt not raised after one PPU dot")
	}
}

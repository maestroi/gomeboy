package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBHaltWakeLeavesPPUOneMachineCycleAhead(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBABC)
	c := NewCPU(b, s)

	b.Set(types.IE, gbio.LCDINT)

	wakeAt := uint64(8)
	witnessAt := wakeAt + 4
	witness := false

	s.RegisterEvent(scheduler.PPUHandleVisualLine, func() {
		b.RaiseInterrupt(gbio.LCDINT)
	})
	s.RegisterEvent(scheduler.PPUHandleGlitchedLine0, func() {
		witness = true
	})
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, wakeAt)
	s.ScheduleEvent(scheduler.PPUHandleGlitchedLine0, witnessAt)

	InstructionSet[0x76].fn(c)

	if c.Halted || s.Halted {
		t.Fatal("CGB stayed halted after enabled STAT interrupt")
	}
	if got := s.Cycle(); got != witnessAt {
		t.Fatalf("CGB HALT wake returned at cycle %d, want %d", got, witnessAt)
	}
	if !witness {
		t.Fatal("PPU event one M-cycle after STAT wake did not run before CPU resumed")
	}
}

func TestDMGHaltWakeDoesNotAddCGBLead(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	c := NewCPU(b, s)

	b.Set(types.IE, gbio.LCDINT)
	s.RegisterEvent(scheduler.PPUHandleVisualLine, func() {
		b.RaiseInterrupt(gbio.LCDINT)
	})
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, 8)

	InstructionSet[0x76].fn(c)

	if got := s.Cycle(); got != 8 {
		t.Fatalf("DMG HALT wake returned at cycle %d, want 8", got)
	}
}

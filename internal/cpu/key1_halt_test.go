package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestKEY1StopHaltsForSpeedSwitchCycles(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, rom)
	b.Map(types.CGBBC)
	c := NewCPU(b, s)
	c.Boot(types.CGBBC)
	b.Write(types.IE, 0)
	b.Write(types.IF, 0)
	b.Write(types.KEY1, types.Bit0)
	start := s.Cycle()
	InstructionSet[0x10].fn(c)
	elapsed := s.Cycle() - start
	// SameBoy halts for 0x20008 CPU T-cycles in total
	if elapsed < speedSwitchHaltCycles {
		t.Fatalf("KEY1 enter-DS halt too short: elapsed=%d", elapsed)
	}
	if !c.DoubleSpeed || !s.DoubleSpeed() {
		t.Fatal("expected double speed after KEY1 STOP")
	}
}

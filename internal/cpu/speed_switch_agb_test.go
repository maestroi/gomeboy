package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newAGBSpeedSwitchCPU(t *testing.T) (*CPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()

	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80 // CGB-compatible cartridge, as used by SameSuite's CGB_MODE.
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, rom)
	b.Map(types.AGB)
	c := NewCPU(b, s)

	b.Write(types.KEY1, types.Bit0)
	if got := b.Get(types.KEY1) & types.Bit0; got == 0 {
		t.Fatal("failed to arm AGB KEY1 speed switch")
	}
	return c, b, s
}

func assertAGBDoubleSpeed(t *testing.T, c *CPU, b *gbio.Bus, s *scheduler.Scheduler) {
	t.Helper()

	if !c.DoubleSpeed || !s.DoubleSpeed() {
		t.Fatalf("AGB STOP did not enter double speed: cpu=%v scheduler=%v", c.DoubleSpeed, s.DoubleSpeed())
	}
	if got := b.Get(types.KEY1); got&types.Bit7 == 0 || got&types.Bit0 != 0 {
		t.Fatalf("AGB KEY1 after speed switch = %#02x, want bit7 set and bit0 clear", got)
	}
}

func TestAGBStopSwitchesSpeedViaInstructionTable(t *testing.T) {
	c, b, s := newAGBSpeedSwitchCPU(t)
	InstructionSet[0x10].fn(c)
	assertAGBDoubleSpeed(t, c, b, s)
}

func TestAGBStopSwitchesSpeedViaDecoder(t *testing.T) {
	c, b, s := newAGBSpeedSwitchCPU(t)
	c.decode(0x10)
	assertAGBDoubleSpeed(t, c, b, s)
}

package timer_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/timer"
)

func TestTimerOverflowFlowsThroughInterruptControllerToCPU(t *testing.T) {
	b := bus.New(nil, nil)
	c := cpu.New()
	if err := c.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)

	irq := gbairq.New(b, c)
	timers := timer.New(b, irq, timer.Hooks{})

	b.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	b.Write16(bus.IOStart+0x208, 1, bus.Access{})
	b.Write16(bus.IOStart+0x100, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+0x102, (1<<6)|(1<<7), bus.Access{})

	timers.Advance(1)
	if irq.IF() != uint16(gbairq.Timer0) || !c.IRQLine() {
		t.Fatalf("timer IRQ path IF=%04x CPU.IRQ=%v", irq.IF(), c.IRQLine())
	}

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != cpu.ExceptionIRQ {
		t.Fatalf("CPU did not take timer IRQ: %+v", result)
	}
	if c.PC() != 0x18 {
		t.Fatalf("timer IRQ vector PC=%08x, want 00000018", c.PC())
	}

	// CPU entry does not clear the hardware IF bit.
	if irq.IF() != uint16(gbairq.Timer0) {
		t.Fatalf("CPU entry unexpectedly cleared timer IF: %04x", irq.IF())
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.Timer0), bus.Access{})
	if c.IRQLine() {
		t.Fatal("timer IF acknowledge did not deassert CPU IRQ line")
	}
}

func Test32BitTimerSetupStartsFromSimultaneouslyWrittenReload(t *testing.T) {
	b := bus.New(nil, nil)
	timers := timer.New(b, nil, timer.Hooks{})

	// STR word to TM0CNT_L/H writes the reload halfword first, then the
	// start/control halfword. The start edge must see the newly written reload.
	b.Write32(bus.IOStart+0x100, uint32(1<<7)<<16|0xfff0, bus.Access{})
	if got := timers.Counter(0); got != 0xfff0 {
		t.Fatalf("32-bit timer setup counter=%04x, want fff0", got)
	}
	timers.Advance(16)
	if got := timers.Counter(0); got != 0xfff0 {
		t.Fatalf("counter after one full period=%04x, want reloaded fff0", got)
	}
}

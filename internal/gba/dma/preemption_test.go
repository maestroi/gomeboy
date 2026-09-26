package dma

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestServiceUnitReArbitratesHigherPriorityChannel(t *testing.T) {
	b := bus.New(nil, nil)
	var starts []uint8
	var completions []int
	d := New(b, nil, Hooks{
		RequestStart: func(channels uint8) {
			starts = append(starts, channels)
		},
		Complete: func(channel int, units uint32, cycles uint32) {
			completions = append(completions, channel)
		},
	})

	lowSource := uint32(bus.IWRAMStart + 0x1000)
	lowDest := uint32(bus.IWRAMStart + 0x1100)
	for i, value := range []uint16{0x3001, 0x3002, 0x3003} {
		b.Write16(lowSource+uint32(i*2), value, bus.Access{})
	}
	programDMA(b, 3, lowSource, lowDest, 3, controlEnable)
	if d.PendingMask() != 1<<3 {
		t.Fatalf("DMA3 pending mask = %02x, want 08", d.PendingMask())
	}

	d.ActivatePending(1 << 3)
	first := d.ServiceUnit()
	if first.Channel != 3 || first.Completed {
		t.Fatalf("first unit = %+v, want active DMA3 non-final", first)
	}
	if got, _ := b.Read16(lowDest, bus.Access{}); got != 0x3001 {
		t.Fatalf("first DMA3 unit = %04x, want 3001", got)
	}
	if !d.Active() || d.ActiveMask() != 1<<3 {
		t.Fatalf("DMA3 active mask after first unit = %02x", d.ActiveMask())
	}

	highSource := uint32(bus.IWRAMStart + 0x1200)
	highDest := uint32(bus.IWRAMStart + 0x1300)
	b.Write16(highSource, 0x0001, bus.Access{})
	programDMA(b, 0, highSource, highDest, 1, controlEnable)
	if d.PendingMask() != 1<<0 {
		t.Fatalf("DMA0 pending mask = %02x, want 01", d.PendingMask())
	}
	d.ActivatePending(1 << 0)

	second := d.ServiceUnit()
	if second.Channel != 0 || !second.Completed {
		t.Fatalf("second unit = %+v, want final DMA0 unit", second)
	}
	d.FinishUnit(second.Channel)
	if got, _ := b.Read16(highDest, bus.Access{}); got != 0x0001 {
		t.Fatalf("preempting DMA0 value = %04x, want 0001", got)
	}
	if len(completions) != 1 || completions[0] != 0 {
		t.Fatalf("completion order after preemption = %v, want [0]", completions)
	}

	third := d.ServiceUnit()
	if third.Channel != 3 || third.Completed {
		t.Fatalf("third unit = %+v, want resumed DMA3 non-final", third)
	}
	fourth := d.ServiceUnit()
	if fourth.Channel != 3 || !fourth.Completed {
		t.Fatalf("fourth unit = %+v, want final DMA3 unit", fourth)
	}
	d.FinishUnit(fourth.Channel)

	if len(completions) != 2 || completions[0] != 0 || completions[1] != 3 {
		t.Fatalf("final completion order = %v, want [0 3]", completions)
	}
	for i, want := range []uint16{0x3001, 0x3002, 0x3003} {
		got, _ := b.Read16(lowDest+uint32(i*2), bus.Access{})
		if got != want {
			t.Fatalf("DMA3 destination[%d] = %04x, want %04x", i, got, want)
		}
	}
	if d.Active() || d.Pending() {
		t.Fatalf("DMA left work behind: active=%02x pending=%02x", d.ActiveMask(), d.PendingMask())
	}
	if len(starts) < 2 {
		t.Fatalf("scheduler start callbacks = %v, want at least two requests", starts)
	}
}

func TestServiceUnitPreservesSequentialTimingAcrossPreemption(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{RequestStart: func(uint8) {}})

	// EWRAM halfword reads cost 3 cycles non-sequential and 3 sequential in
	// the current default waitstate model; ROM makes the distinction observable.
	rom := make([]byte, 0x100)
	for i := 0; i < len(rom); i += 2 {
		rom[i] = byte(i / 2)
	}
	b = bus.New(nil, rom)
	d = New(b, nil, Hooks{RequestStart: func(uint8) {}})

	lowDest := uint32(bus.IWRAMStart + 0x1400)
	programDMA(b, 3, bus.ROM0Start, lowDest, 3, controlEnable)
	d.ActivatePending(1 << 3)

	first := d.ServiceUnit()
	if first.Channel != 3 {
		t.Fatalf("first ROM DMA unit channel = %d, want 3", first.Channel)
	}

	highSource := uint32(bus.IWRAMStart + 0x1500)
	highDest := uint32(bus.IWRAMStart + 0x1600)
	b.Write16(highSource, 0xabcd, bus.Access{})
	programDMA(b, 0, highSource, highDest, 1, controlEnable)
	d.ActivatePending(1 << 0)
	high := d.ServiceUnit()
	d.FinishUnit(high.Channel)

	resumed := d.ServiceUnit()
	if resumed.Channel != 3 {
		t.Fatalf("resumed channel = %d, want 3", resumed.Channel)
	}
	if resumed.Cycles >= first.Cycles {
		t.Fatalf("resumed DMA lost sequential timing: first=%d resumed=%d", first.Cycles, resumed.Cycles)
	}
}

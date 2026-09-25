package dma_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/dma"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

func TestImmediateDMACompletionFlowsThroughIRQControllerToCPU(t *testing.T) {
	b := bus.New(nil, nil)
	c := cpu.New()
	if err := c.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)

	irq := gbairq.New(b, c)
	_ = dma.New(b, irq, dma.Hooks{})

	source := uint32(bus.EWRAMStart + 0x100)
	dest := uint32(bus.IWRAMStart + 0x100)
	b.Write16(source, 0x55aa, bus.Access{})

	b.Write16(bus.IOStart+0x200, uint16(gbairq.DMA0), bus.Access{})
	b.Write16(bus.IOStart+0x208, 1, bus.Access{})

	b.Write32(bus.IOStart+0x0b0, source, bus.Access{})
	b.Write32(bus.IOStart+0x0b4, dest, bus.Access{})
	b.Write16(bus.IOStart+0x0b8, 1, bus.Access{})
	b.Write16(bus.IOStart+0x0ba, (1<<14)|(1<<15), bus.Access{})

	got, _ := b.Read16(dest, bus.Access{})
	if got != 0x55aa {
		t.Fatalf("DMA result = %04x, want 55aa", got)
	}
	if irq.IF() != uint16(gbairq.DMA0) || !c.IRQLine() {
		t.Fatalf("DMA IRQ path IF=%04x CPU.IRQ=%v", irq.IF(), c.IRQLine())
	}

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != cpu.ExceptionIRQ {
		t.Fatalf("CPU did not take DMA IRQ: %+v", result)
	}
	if c.PC() != 0x18 {
		t.Fatalf("DMA IRQ vector PC=%08x, want 00000018", c.PC())
	}

	// Completion IRQ remains latched until software acknowledges IF.
	b.Write16(bus.IOStart+0x202, uint16(gbairq.DMA0), bus.Access{})
	if c.IRQLine() {
		t.Fatal("DMA IF acknowledgement did not deassert CPU IRQ")
	}
}

package interrupt_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
)

func TestPPUIRQFlowsThroughIFToCPU(t *testing.T) {
	b := bus.New(nil, nil)
	c := cpu.New()
	if err := c.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)

	irq := gbairq.New(b, c)
	p := ppu.New(b, ppu.Hooks{IRQ: irq.Request})

	enabled := uint16(gbairq.VBlank | gbairq.HBlank | gbairq.VCount)
	b.Write16(bus.IOStart+0x200, enabled, bus.Access{})
	b.Write16(bus.IOStart+0x208, 1, bus.Access{})

	// Enable all three LCD IRQ sources, comparing VCOUNT against line 1.
	b.Write16(bus.IOStart+0x004, (1<<3)|(1<<4)|(1<<5)|(1<<8), bus.Access{})

	// HBlank IRQ is requested at the delayed DISPSTAT HBlank flag point.
	p.Advance(ppu.HBlankFlagCycle)
	if irq.IF() != uint16(gbairq.HBlank) || !c.IRQLine() {
		t.Fatalf("HBlank path IF=%04x CPU.IRQ=%v", irq.IF(), c.IRQLine())
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.HBlank), bus.Access{})
	if c.IRQLine() {
		t.Fatal("HBlank acknowledge did not deassert CPU IRQ")
	}

	// Disable further HBlank IRQ requests so later checks isolate the other
	// source bits; keep VBlank/VCount enabled and compare against line 1.
	b.Write16(bus.IOStart+0x004, (1<<3)|(1<<5)|(1<<8), bus.Access{})

	// Finishing line 0 enters VCOUNT=1 and requests VCount IRQ.
	p.Advance(ppu.CyclesPerLine - ppu.HBlankFlagCycle)
	if irq.IF() != uint16(gbairq.VCount) || !c.IRQLine() {
		t.Fatalf("VCount path IF=%04x CPU.IRQ=%v", irq.IF(), c.IRQLine())
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.VCount), bus.Access{})

	// Advance from line 1 to VBlank entry at line 160.
	p.Advance(uint32(ppu.VBlankStartLine-1) * ppu.CyclesPerLine)
	if irq.IF() != uint16(gbairq.VBlank) || !c.IRQLine() {
		t.Fatalf("VBlank path IF=%04x CPU.IRQ=%v", irq.IF(), c.IRQLine())
	}

	// The CPU sees the controller's level at the next instruction boundary.
	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != cpu.ExceptionIRQ {
		t.Fatalf("CPU did not enter IRQ from PPU request: %+v", result)
	}
	if got := c.PC(); got != 0x18 {
		t.Fatalf("IRQ vector PC = %08x, want 00000018", got)
	}

	// CPU exception entry does not acknowledge IF; software must W1C it.
	if irq.IF() != uint16(gbairq.VBlank) {
		t.Fatalf("CPU entry unexpectedly cleared IF: %04x", irq.IF())
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.VBlank), bus.Access{})
	if c.IRQLine() {
		t.Fatal("VBlank acknowledge did not deassert controller CPU line")
	}
}

func TestPPURequestLatchesWhileMasterDisabledThenAssertsWhenEnabled(t *testing.T) {
	b := bus.New(nil, nil)
	c := cpu.New()
	irq := gbairq.New(b, c)
	p := ppu.New(b, ppu.Hooks{IRQ: irq.Request})

	b.Write16(bus.IOStart+0x200, uint16(gbairq.VBlank), bus.Access{})
	b.Write16(bus.IOStart+0x004, 1<<3, bus.Access{})
	p.Advance(uint32(ppu.VBlankStartLine) * ppu.CyclesPerLine)

	if irq.IF() != uint16(gbairq.VBlank) {
		t.Fatalf("VBlank did not latch with IME disabled: IF=%04x", irq.IF())
	}
	if c.IRQLine() {
		t.Fatal("IME-disabled pending VBlank asserted CPU IRQ")
	}

	b.Write16(bus.IOStart+0x208, 1, bus.Access{})
	if !c.IRQLine() {
		t.Fatal("enabling IME with pending enabled VBlank did not assert CPU IRQ")
	}
}

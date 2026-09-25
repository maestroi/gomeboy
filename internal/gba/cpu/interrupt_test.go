package cpu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

const subsPCFromLRMinus4 uint32 = 0xe25ef004

func newInterruptBus(rom []byte) *bus.Bus {
	bios := make([]byte, bus.BIOSSize)
	putARM(bios, 0x18, subsPCFromLRMinus4)
	putARM(bios, 0x1c, subsPCFromLRMinus4)
	return bus.New(bios, rom)
}

func TestInterruptLinesResetDeasserted(t *testing.T) {
	c := New()
	if c.IRQLine() || c.FIQLine() {
		t.Fatalf("reset interrupt lines IRQ=%v FIQ=%v, want both false", c.IRQLine(), c.FIQLine())
	}
}

func TestMaskedIRQDoesNotPreventInstructionExecution(t *testing.T) {
	rom := make([]byte, 8)
	putARM(rom, 0, 0xe3a00007) // MOV r0,#7
	b := newInterruptBus(rom)

	c := New() // reset has IRQ/FIQ masked
	c.SetPC(bus.ROM0Start)
	c.SetIRQLine(true)

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExceptionTaken {
		t.Fatalf("masked IRQ was taken: %+v", result)
	}
	if got := c.ReadRegister(0); got != 7 {
		t.Fatalf("masked IRQ blocked MOV: r0=%d, want 7", got)
	}
}

func TestIRQEntryAtInstructionBoundary(t *testing.T) {
	rom := make([]byte, 0x40)
	putARM(rom, 0x20, 0xe3a00009) // must not execute before IRQ
	b := newInterruptBus(rom)

	c := New()
	start := PSR(ModeSystem) | FlagCarry
	if err := c.SetCPSR(start); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start + 0x20)
	c.SetIRQLine(true)

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != ExceptionIRQ || !result.PipelineFlush {
		t.Fatalf("IRQ Step result = %+v", result)
	}
	if result.FetchCycles != 0 || result.MemoryCycles != 0 || result.InternalCycles != 1 {
		t.Fatalf("IRQ boundary timing = %+v, want no canceled opcode/data fetch and one entry cycle", result)
	}
	if got := c.ReadRegister(0); got != 0 {
		t.Fatalf("instruction losing priority executed: r0=%d", got)
	}
	if got := c.CPSR().Mode(); got != ModeIRQ {
		t.Fatalf("IRQ mode = %v, want IRQ", got)
	}
	if c.CPSR().Thumb() {
		t.Fatal("IRQ entry did not force ARM state")
	}
	if !c.CPSR().IRQDisabled() {
		t.Fatal("IRQ entry did not set I mask")
	}
	if c.CPSR().FIQDisabled() {
		t.Fatal("IRQ entry unexpectedly set F mask")
	}
	if got := c.PC(); got != 0x18 {
		t.Fatalf("IRQ vector PC = %08x, want 00000018", got)
	}
	if got := c.ReadRegister(14); got != bus.ROM0Start+0x24 {
		t.Fatalf("LR_irq = %08x, want interrupted PC+4 %08x", got, bus.ROM0Start+0x24)
	}
	saved, ok := c.SPSR()
	if !ok || saved != start {
		t.Fatalf("SPSR_irq = %08x ok=%v, want %08x", saved, ok, start)
	}
}

func TestThumbIRQReturnResumesInterruptedInstruction(t *testing.T) {
	rom := make([]byte, 0x20)
	putThumb(rom, 0x10, 0x2005) // MOV r0,#5
	b := newInterruptBus(rom)

	c := New()
	start := PSR(ModeSystem) | FlagThumb | FlagZero
	if err := c.SetCPSR(start); err != nil {
		t.Fatal(err)
	}
	interruptedPC := uint32(bus.ROM0Start + 0x10)
	c.SetPC(interruptedPC)
	c.SetIRQLine(true)

	entry, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !entry.ExceptionTaken || c.ReadRegister(14) != interruptedPC+4 {
		t.Fatalf("Thumb IRQ entry=%+v LR=%08x, want %08x", entry, c.ReadRegister(14), interruptedPC+4)
	}
	saved, _ := c.SPSR()
	if !saved.Thumb() {
		t.Fatal("Thumb state was not preserved in SPSR_irq")
	}

	// Deassert before executing the IRQ-vector return instruction.
	c.SetIRQLine(false)
	ret, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !ret.PipelineFlush {
		t.Fatalf("SUBS pc,lr,#4 did not flush pipeline: %+v", ret)
	}
	if got := c.CPSR(); got != start {
		t.Fatalf("IRQ return CPSR = %08x, want %08x", got, start)
	}
	if got := c.PC(); got != interruptedPC {
		t.Fatalf("IRQ return PC = %08x, want interrupted instruction %08x", got, interruptedPC)
	}

	resumed, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ExceptionTaken {
		t.Fatalf("unexpected interrupt after deassert: %+v", resumed)
	}
	if got := c.ReadRegister(0); got != 5 {
		t.Fatalf("interrupted Thumb instruction did not resume: r0=%d, want 5", got)
	}
}

func TestFIQHasPriorityOverIRQ(t *testing.T) {
	b := newInterruptBus(nil)
	c := New()
	start := PSR(ModeSystem)
	if err := c.SetCPSR(start); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.SetIRQLine(true)
	c.SetFIQLine(true)

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != ExceptionFIQ {
		t.Fatalf("simultaneous IRQ/FIQ result = %+v, want FIQ", result)
	}
	if got := c.CPSR().Mode(); got != ModeFIQ {
		t.Fatalf("mode = %v, want FIQ", got)
	}
	if !c.CPSR().FIQDisabled() || !c.CPSR().IRQDisabled() {
		t.Fatalf("FIQ masks I=%v F=%v, want both true", c.CPSR().IRQDisabled(), c.CPSR().FIQDisabled())
	}
	if got := c.PC(); got != 0x1c {
		t.Fatalf("FIQ vector = %08x, want 0000001c", got)
	}
	if got := c.ReadRegister(14); got != bus.ROM0Start+4 {
		t.Fatalf("LR_fiq = %08x, want %08x", got, bus.ROM0Start+4)
	}
	saved, ok := c.SPSR()
	if !ok || saved != start {
		t.Fatalf("SPSR_fiq = %08x ok=%v, want %08x", saved, ok, start)
	}
}

func TestMaskedFIQAllowsEligibleIRQ(t *testing.T) {
	b := newInterruptBus(nil)
	c := New()
	if err := c.SetCPSR(PSR(ModeSystem) | FlagFIQDisable); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.SetIRQLine(true)
	c.SetFIQLine(true)

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != ExceptionIRQ {
		t.Fatalf("masked FIQ + IRQ result = %+v, want IRQ", result)
	}
}

func TestFIQCanPreemptIRQHandlerWhenUnmasked(t *testing.T) {
	b := newInterruptBus(nil)
	c := New()
	if err := c.SetCPSR(PSR(ModeSystem)); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.SetIRQLine(true)

	if _, err := c.Step(b); err != nil {
		t.Fatal(err)
	}
	if c.CPSR().Mode() != ModeIRQ || c.CPSR().FIQDisabled() {
		t.Fatalf("IRQ entry state CPSR=%08x", c.CPSR())
	}

	c.SetFIQLine(true)
	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExceptionTaken || result.Exception != ExceptionFIQ {
		t.Fatalf("FIQ did not preempt IRQ handler: %+v", result)
	}
	saved, ok := c.SPSR()
	if !ok || saved.Mode() != ModeIRQ {
		t.Fatalf("SPSR_fiq mode=%v ok=%v, want IRQ", saved.Mode(), ok)
	}
	if got := c.ReadRegister(14); got != 0x1c {
		t.Fatalf("nested LR_fiq=%08x, want IRQ vector PC+4 0000001c", got)
	}
}

func TestLevelSensitiveIRQRetriggersAfterReturnIfLineStaysAsserted(t *testing.T) {
	b := newInterruptBus(nil)
	c := New()
	start := PSR(ModeSystem)
	if err := c.SetCPSR(start); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.SetIRQLine(true)

	if _, err := c.Step(b); err != nil {
		t.Fatal(err)
	}
	// Keep IRQ asserted. I is set while the vector instruction executes, so
	// the handler can return normally.
	ret, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if ret.ExceptionTaken || c.CPSR() != start || c.PC() != bus.ROM0Start {
		t.Fatalf("IRQ return state result=%+v CPSR=%08x PC=%08x", ret, c.CPSR(), c.PC())
	}

	retrigger, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !retrigger.ExceptionTaken || retrigger.Exception != ExceptionIRQ {
		t.Fatalf("asserted IRQ did not retrigger after unmask: %+v", retrigger)
	}
}

func TestResetClearsAssertedInterruptLines(t *testing.T) {
	c := New()
	c.SetIRQLine(true)
	c.SetFIQLine(true)
	c.Reset()
	if c.IRQLine() || c.FIQLine() {
		t.Fatalf("Reset left interrupt lines asserted IRQ=%v FIQ=%v", c.IRQLine(), c.FIQLine())
	}
}

package cpu

import "testing"

func TestResetState(t *testing.T) {
	c := New()
	if got := c.CPSR().Mode(); got != ModeSupervisor {
		t.Fatalf("mode = %v, want supervisor", got)
	}
	if c.CPSR().Thumb() {
		t.Fatal("reset state is Thumb, want ARM")
	}
	if !c.CPSR().IRQDisabled() || !c.CPSR().FIQDisabled() {
		t.Fatalf("reset masks: I=%v F=%v, want both set", c.CPSR().IRQDisabled(), c.CPSR().FIQDisabled())
	}
	if got := c.PC(); got != 0 {
		t.Fatalf("PC = 0x%08x, want 0", got)
	}
	if got := c.VisiblePC(); got != 8 {
		t.Fatalf("visible PC = 0x%08x, want 8", got)
	}
}

func TestBankedRegisters(t *testing.T) {
	c := New()

	// Supervisor shares r8-r12 with User/System but owns a banked SP/LR.
	c.WriteRegister(8, 0x88)
	c.WriteRegister(13, 0x1300)
	c.WriteRegister(14, 0x1400)

	if err := c.SetMode(ModeFIQ); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0 {
		t.Fatalf("FIQ r8 = 0x%x, want private zero bank", got)
	}
	c.WriteRegister(8, 0xf8)
	c.WriteRegister(13, 0xf1300)

	if err := c.SetMode(ModeIRQ); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0x88 {
		t.Fatalf("IRQ r8 = 0x%x, want shared 0x88", got)
	}
	if got := c.ReadRegister(13); got != 0 {
		t.Fatalf("IRQ r13 = 0x%x, want private zero bank", got)
	}
	c.WriteRegister(13, 0x121300)

	if err := c.SetMode(ModeSupervisor); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(13); got != 0x1300 {
		t.Fatalf("SVC r13 = 0x%x, want 0x1300", got)
	}

	if err := c.SetMode(ModeFIQ); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0xf8 {
		t.Fatalf("FIQ r8 = 0x%x, want 0xf8", got)
	}
	if got := c.ReadRegister(13); got != 0xf1300 {
		t.Fatalf("FIQ r13 = 0x%x, want 0xf1300", got)
	}

	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0x88 {
		t.Fatalf("System r8 = 0x%x, want shared 0x88", got)
	}
	if got := c.ReadRegister(13); got != 0 {
		t.Fatalf("System r13 = 0x%x, want User/System bank", got)
	}
}

func TestPCVisibilityAndAlignment(t *testing.T) {
	c := New()
	c.SetPC(0x103)
	if got := c.PC(); got != 0x100 {
		t.Fatalf("ARM PC = 0x%x, want 0x100", got)
	}
	if got := c.ReadRegister(15); got != 0x108 {
		t.Fatalf("ARM r15 = 0x%x, want 0x108", got)
	}

	c.SetThumb(true)
	c.SetPC(0x103)
	if got := c.PC(); got != 0x102 {
		t.Fatalf("Thumb PC = 0x%x, want 0x102", got)
	}
	if got := c.ReadRegister(15); got != 0x106 {
		t.Fatalf("Thumb r15 = 0x%x, want 0x106", got)
	}
}

func TestExceptionEntryAndStatusRestore(t *testing.T) {
	c := New()
	start := PSR(ModeSystem) | FlagThumb | FlagCarry
	if err := c.SetCPSR(start); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x100)

	if err := c.EnterException(ExceptionIRQ, 0x106); err != nil {
		t.Fatal(err)
	}
	if got := c.CPSR().Mode(); got != ModeIRQ {
		t.Fatalf("mode = %v, want IRQ", got)
	}
	if c.CPSR().Thumb() {
		t.Fatal("IRQ did not enter ARM state")
	}
	if !c.CPSR().IRQDisabled() {
		t.Fatal("IRQ mask not set on IRQ entry")
	}
	if got := c.PC(); got != 0x18 {
		t.Fatalf("IRQ vector PC = 0x%x, want 0x18", got)
	}
	if got := c.ReadRegister(14); got != 0x106 {
		t.Fatalf("IRQ LR = 0x%x, want 0x106", got)
	}
	saved, ok := c.SPSR()
	if !ok {
		t.Fatal("IRQ mode has no SPSR")
	}
	if saved != start {
		t.Fatalf("SPSR_irq = 0x%08x, want 0x%08x", saved, start)
	}

	if err := c.RestoreCPSRFromSPSR(); err != nil {
		t.Fatal(err)
	}
	if got := c.CPSR(); got != start {
		t.Fatalf("restored CPSR = 0x%08x, want 0x%08x", got, start)
	}
}

func TestUserAndSystemHaveNoSPSR(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.SPSR(); ok {
		t.Fatal("System mode unexpectedly exposes SPSR")
	}
	if err := c.SetSPSR(PSR(ModeIRQ)); err != ErrNoSPSR {
		t.Fatalf("SetSPSR error = %v, want ErrNoSPSR", err)
	}
}

func TestAddWithCarryOverflowAndBorrow(t *testing.T) {
	result, carry, overflow := addWithCarry(0x7fffffff, 1, false)
	if result != 0x80000000 || carry || !overflow {
		t.Fatalf("positive overflow: result=%08x C=%v V=%v", result, carry, overflow)
	}

	result, carry, overflow = addWithCarry(0, ^uint32(1), true) // 0 - 1
	if result != 0xffffffff || carry || overflow {
		t.Fatalf("borrow: result=%08x C=%v V=%v", result, carry, overflow)
	}

	result, carry, overflow = addWithCarry(5, ^uint32(3), true) // 5 - 3
	if result != 2 || !carry || overflow {
		t.Fatalf("no borrow: result=%08x C=%v V=%v", result, carry, overflow)
	}
}

package cpu

import "testing"

func newThumbCPU(t *testing.T) *CPU {
	t.Helper()
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetThumb(true)
	return c
}

func TestThumbImmediateALUProgram(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x100)

	program := []uint16{
		0x2005, // MOV r0,#5
		0x3003, // ADD r0,#3
		0x3808, // SUB r0,#8
	}
	for _, instruction := range program {
		if _, err := c.ExecuteThumb(instruction); err != nil {
			t.Fatalf("ExecuteThumb(0x%04x): %v", instruction, err)
		}
	}
	if got := c.ReadRegister(0); got != 0 {
		t.Fatalf("r0 = %d, want 0", got)
	}
	if !c.CPSR().Zero() || !c.CPSR().Carry() {
		t.Fatalf("flags Z=%v C=%v, want true/true", c.CPSR().Zero(), c.CPSR().Carry())
	}
	if got := c.PC(); got != 0x106 {
		t.Fatalf("PC = 0x%x, want 0x106", got)
	}
}

func TestThumbAddSubtractRegister(t *testing.T) {
	c := newThumbCPU(t)
	c.WriteRegister(1, 7)
	c.WriteRegister(2, 5)

	// ADD r0,r1,r2: 0001100 Rn=2 Rs=1 Rd=0
	if _, err := c.ExecuteThumb(0x1888); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 12 {
		t.Fatalf("ADD result = %d, want 12", got)
	}

	// SUB r3,r1,r2.
	if _, err := c.ExecuteThumb(0x1a8b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 2 {
		t.Fatalf("SUB result = %d, want 2", got)
	}
}

func TestThumbConditionalBranch(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x100)

	if _, err := c.ExecuteThumb(0x2000); err != nil { // MOV r0,#0 -> Z
		t.Fatal(err)
	}
	result, err := c.ExecuteThumb(0xd001) // BEQ +2
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("taken BEQ did not flush pipeline")
	}
	if got := c.PC(); got != 0x108 {
		t.Fatalf("BEQ PC = 0x%x, want 0x108", got)
	}
}

func TestThumbBXSwitchesToARM(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x100)
	c.WriteRegister(1, 0x200)

	result, err := c.ExecuteThumb(0x4708) // BX r1
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("BX did not flush pipeline")
	}
	if c.CPSR().Thumb() {
		t.Fatal("BX even address did not switch to ARM")
	}
	if got := c.PC(); got != 0x200 {
		t.Fatalf("PC = 0x%x, want 0x200", got)
	}
}

func TestThumbLongBranchWithLink(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x100)

	if _, err := c.ExecuteThumb(0xf000); err != nil { // BL prefix, zero high offset
		t.Fatal(err)
	}
	if got := c.ReadRegister(14); got != 0x104 {
		t.Fatalf("BL prefix LR = 0x%x, want 0x104", got)
	}
	result, err := c.ExecuteThumb(0xf800) // BL suffix, zero low offset
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("BL suffix did not flush pipeline")
	}
	if got := c.PC(); got != 0x104 {
		t.Fatalf("BL target = 0x%x, want 0x104", got)
	}
	if got := c.ReadRegister(14); got != 0x105 {
		t.Fatalf("BL return LR = 0x%x, want 0x105", got)
	}
}

func TestThumbALUAndMultiplyTiming(t *testing.T) {
	c := newThumbCPU(t)
	c.WriteRegister(0, 3)
	c.WriteRegister(1, 4)

	result, err := c.ExecuteThumb(0x4348) // MUL r0,r1
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 12 {
		t.Fatalf("MUL result = %d, want 12", got)
	}
	if result.InternalCycles != 1 {
		t.Fatalf("small MUL internal cycles = %d, want 1", result.InternalCycles)
	}

	c.WriteRegister(1, 0x12345678)
	result, err = c.ExecuteThumb(0x4348)
	if err != nil {
		t.Fatal(err)
	}
	if result.InternalCycles != 4 {
		t.Fatalf("large MUL internal cycles = %d, want 4", result.InternalCycles)
	}
}

func TestThumbSWIPreservesThumbInSPSR(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x200)

	result, err := c.ExecuteThumb(0xdf42)
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush || c.CPSR().Thumb() {
		t.Fatalf("SWI result=%+v Thumb=%v", result, c.CPSR().Thumb())
	}
	if got := c.PC(); got != 0x8 {
		t.Fatalf("SWI PC = 0x%x, want 8", got)
	}
	if got := c.ReadRegister(14); got != 0x202 {
		t.Fatalf("SWI LR_svc = 0x%x, want 0x202", got)
	}
	saved, ok := c.SPSR()
	if !ok || !saved.Thumb() || saved.Mode() != ModeSystem {
		t.Fatalf("SPSR_svc = 0x%08x ok=%v", saved, ok)
	}
}

func TestWrongInstructionStateRejected(t *testing.T) {
	c := New()
	if _, err := c.ExecuteThumb(0x2000); err == nil {
		t.Fatal("ExecuteThumb accepted instruction in ARM state")
	}
	c.SetThumb(true)
	if _, err := c.ExecuteARM(0xe1a00000); err == nil {
		t.Fatal("ExecuteARM accepted instruction in Thumb state")
	}
}

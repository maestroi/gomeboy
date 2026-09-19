package cpu

import "testing"

func armDPImmediate(opcode uint32, setFlags bool, rn, rd uint32, rotate, imm uint32) uint32 {
	instruction := uint32(0xe0000000) | 1<<25 | opcode<<21 | rn<<16 | rd<<12 | rotate<<8 | imm
	if setFlags {
		instruction |= 1 << 20
	}
	return instruction
}

func TestARMDataProcessingProgram(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x100)

	// MOV r0,#5
	if _, err := c.ExecuteARM(armDPImmediate(0xd, false, 0, 0, 0, 5)); err != nil {
		t.Fatal(err)
	}
	// ADD r1,r0,#7
	if _, err := c.ExecuteARM(armDPImmediate(0x4, false, 0, 1, 0, 7)); err != nil {
		t.Fatal(err)
	}
	// SUBS r2,r1,#12
	if _, err := c.ExecuteARM(armDPImmediate(0x2, true, 1, 2, 0, 12)); err != nil {
		t.Fatal(err)
	}

	if got := c.ReadRegister(0); got != 5 {
		t.Fatalf("r0 = %d, want 5", got)
	}
	if got := c.ReadRegister(1); got != 12 {
		t.Fatalf("r1 = %d, want 12", got)
	}
	if got := c.ReadRegister(2); got != 0 {
		t.Fatalf("r2 = %d, want 0", got)
	}
	if !c.CPSR().Zero() || !c.CPSR().Carry() || c.CPSR().Negative() || c.CPSR().Overflow() {
		t.Fatalf("SUBS flags N=%v Z=%v C=%v V=%v",
			c.CPSR().Negative(), c.CPSR().Zero(), c.CPSR().Carry(), c.CPSR().Overflow())
	}
	if got := c.PC(); got != 0x10c {
		t.Fatalf("PC = 0x%x, want 0x10c", got)
	}
}

func TestARMImmediateRotateAndShifterCarry(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	// MOVS r3,#0x80000000 encoded as imm8=0x80 ROR #8.
	instruction := armDPImmediate(0xd, true, 0, 3, 4, 0x80)
	if _, err := c.ExecuteARM(instruction); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 0x80000000 {
		t.Fatalf("r3 = 0x%08x, want 0x80000000", got)
	}
	if !c.CPSR().Negative() || !c.CPSR().Carry() {
		t.Fatalf("MOVS flags N=%v C=%v, want true/true", c.CPSR().Negative(), c.CPSR().Carry())
	}
}

func TestARMConditionFailureSkipsInstruction(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x200)
	c.WriteRegister(0, 99)

	// MOVEQ r0,#1, with Z clear.
	instruction := armDPImmediate(0xd, false, 0, 0, 0, 1)
	instruction &^= 0xf << 28 // EQ condition
	if _, err := c.ExecuteARM(instruction); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 99 {
		t.Fatalf("failed condition wrote r0=%d", got)
	}
	if got := c.PC(); got != 0x204 {
		t.Fatalf("PC = 0x%x, want 0x204", got)
	}
}

func TestARMBranchLinkAndBX(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x100)

	result, err := c.ExecuteARM(0xeb000000) // BL +0 -> visible PC
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("BL did not request pipeline refill")
	}
	if got := c.PC(); got != 0x108 {
		t.Fatalf("BL PC = 0x%x, want 0x108", got)
	}
	if got := c.ReadRegister(14); got != 0x104 {
		t.Fatalf("BL LR = 0x%x, want 0x104", got)
	}

	c.WriteRegister(3, 0x201)
	result, err = c.ExecuteARM(0xe12fff13) // BX r3
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush || !c.CPSR().Thumb() {
		t.Fatalf("BX result=%+v Thumb=%v", result, c.CPSR().Thumb())
	}
	if got := c.PC(); got != 0x200 {
		t.Fatalf("BX PC = 0x%x, want 0x200", got)
	}
}

func TestARMSWIEntersSupervisor(t *testing.T) {
	c := New()
	if err := c.SetCPSR(PSR(ModeSystem) | FlagCarry); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x300)

	result, err := c.ExecuteARM(0xef000123)
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("SWI did not flush pipeline")
	}
	if got := c.CPSR().Mode(); got != ModeSupervisor {
		t.Fatalf("mode = %v, want supervisor", got)
	}
	if got := c.PC(); got != 0x8 {
		t.Fatalf("SWI PC = 0x%x, want 8", got)
	}
	if got := c.ReadRegister(14); got != 0x304 {
		t.Fatalf("SWI LR_svc = 0x%x, want 0x304", got)
	}
	saved, ok := c.SPSR()
	if !ok || saved != PSR(ModeSystem)|FlagCarry {
		t.Fatalf("SPSR_svc = 0x%08x ok=%v", saved, ok)
	}
}

func TestARMRegisterShiftSpecialCases(t *testing.T) {
	value := uint32(0x80000001)

	got, carry, err := shiftImmediate(value, 1, 0, false) // LSR #32
	if err != nil || got != 0 || !carry {
		t.Fatalf("LSR #32 = %08x C=%v err=%v", got, carry, err)
	}

	got, carry, err = shiftImmediate(value, 3, 0, true) // RRX
	if err != nil || got != 0xc0000000 || !carry {
		t.Fatalf("RRX = %08x C=%v err=%v", got, carry, err)
	}
}

func TestConditionCodes(t *testing.T) {
	psr := PSR(ModeSystem) | FlagNegative | FlagZero | FlagCarry
	cases := map[uint8]bool{
		0x0: true,  // EQ
		0x1: false, // NE
		0x2: true,  // CS
		0x3: false, // CC
		0x4: true,  // MI
		0x5: false, // PL
		0x8: false, // HI (Z set)
		0x9: true,  // LS
		0xa: false, // GE (N != V)
		0xb: true,  // LT
		0xc: false, // GT
		0xd: true,  // LE
		0xe: true,  // AL
		0xf: false,
	}
	for cond, want := range cases {
		if got := conditionPassed(cond, psr); got != want {
			t.Errorf("condition %x = %v, want %v", cond, got, want)
		}
	}
}

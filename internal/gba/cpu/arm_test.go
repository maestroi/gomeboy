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


func TestARMRegisterSpecifiedShift(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(0, 1)
	c.WriteRegister(1, 4)

	// MOVS r2,r0,LSL r1.
	result, err := c.ExecuteARM(0xe1b02110)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 16 {
		t.Fatalf("shifted result = %d, want 16", got)
	}
	if result.InternalCycles != 2 {
		t.Fatalf("register shift cycles = %d, want 2", result.InternalCycles)
	}
	if c.CPSR().Zero() || c.CPSR().Negative() {
		t.Fatalf("unexpected flags N=%v Z=%v", c.CPSR().Negative(), c.CPSR().Zero())
	}
}

func TestARMMultiplyAndLongMultiply(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(0, 7)
	c.WriteRegister(1, 6)

	// MUL r2,r0,r1.
	result, err := c.ExecuteARM(0xe0020190)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 42 {
		t.Fatalf("MUL result = %d, want 42", got)
	}
	if result.InternalCycles != 1 {
		t.Fatalf("MUL cycles = %d, want 1", result.InternalCycles)
	}

	// MLA r3,r0,r1,r2.
	result, err = c.ExecuteARM(0xe0232190)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 84 {
		t.Fatalf("MLA result = %d, want 84", got)
	}
	if result.InternalCycles != 2 {
		t.Fatalf("MLA cycles = %d, want 2", result.InternalCycles)
	}

	c.WriteRegister(0, 0xffffffff)
	c.WriteRegister(1, 2)
	// UMULL r2,r3,r0,r1.
	result, err = c.ExecuteARM(0xe0832190)
	if err != nil {
		t.Fatal(err)
	}
	if lo, hi := c.ReadRegister(2), c.ReadRegister(3); lo != 0xfffffffe || hi != 1 {
		t.Fatalf("UMULL = %08x:%08x, want 00000001:fffffffe", hi, lo)
	}
	if result.InternalCycles != 2 {
		t.Fatalf("UMULL cycles = %d, want 2", result.InternalCycles)
	}

	c.WriteRegister(0, 0xffffffff) // -1
	c.WriteRegister(1, 2)
	// SMULL r4,r5,r0,r1.
	if _, err := c.ExecuteARM(0xe0c54190); err != nil {
		t.Fatal(err)
	}
	if lo, hi := c.ReadRegister(4), c.ReadRegister(5); lo != 0xfffffffe || hi != 0xffffffff {
		t.Fatalf("SMULL = %08x:%08x, want ffffffff:fffffffe", hi, lo)
	}
}

func TestARMPSRTransfers(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSupervisor); err != nil {
		t.Fatal(err)
	}

	// MRS r2,CPSR.
	if _, err := c.ExecuteARM(0xe10f2000); err != nil {
		t.Fatal(err)
	}
	if got := PSR(c.ReadRegister(2)); got.Mode() != ModeSupervisor {
		t.Fatalf("MRS CPSR mode = %v, want supervisor", got.Mode())
	}

	c.WriteRegister(0, uint32(FlagNegative|FlagCarry))
	// MSR CPSR_f,r0.
	if _, err := c.ExecuteARM(0xe128f000); err != nil {
		t.Fatal(err)
	}
	if !c.CPSR().Negative() || !c.CPSR().Carry() || c.CPSR().Zero() {
		t.Fatalf("MSR flags N=%v Z=%v C=%v", c.CPSR().Negative(), c.CPSR().Zero(), c.CPSR().Carry())
	}
	if got := c.CPSR().Mode(); got != ModeSupervisor {
		t.Fatalf("flag-only MSR changed mode to %v", got)
	}

	c.WriteRegister(0, uint32(ModeIRQ)|uint32(FlagIRQDisable))
	// MSR CPSR_c,r0.
	if _, err := c.ExecuteARM(0xe121f000); err != nil {
		t.Fatal(err)
	}
	if got := c.CPSR().Mode(); got != ModeIRQ {
		t.Fatalf("control MSR mode = %v, want IRQ", got)
	}
}

func TestMultiplyEarlyTerminationBoundaries(t *testing.T) {
	cases := []struct {
		value uint32
		want  uint8
	}{
		{0x0000007f, 1},
		{0xffffff80, 1},
		{0x00008000, 2},
		{0xffff8000, 2},
		{0x00ffffff, 3},
		{0xff000000, 3},
		{0x12345678, 4},
	}
	for _, tc := range cases {
		if got := multiplyInternalCycles(tc.value); got != tc.want {
			t.Errorf("multiplyInternalCycles(%08x) = %d, want %d", tc.value, got, tc.want)
		}
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

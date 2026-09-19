package cpu

import (
	"fmt"
	"math/bits"
)

// ExecuteARM executes one already-fetched ARM instruction.
//
// Memory-transfer, multiply, coprocessor, and PSR-transfer encodings are left
// for follow-up slices; unsupported encodings return an error without moving
// PC. This keeps the initial decoder strict instead of silently mis-decoding
// special ARM encodings as data-processing instructions.
func (c *CPU) ExecuteARM(instruction uint32) (ExecutionResult, error) {
	if c.cpsr.Thumb() {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ExecuteARM called in Thumb state")
	}

	cond := uint8(instruction >> 28)
	if !conditionPassed(cond, c.cpsr) {
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	if instruction&0x0ffffff0 == 0x012fff10 {
		rm := int(instruction & 0xf)
		c.branchExchange(c.ReadRegister(rm))
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	// SWI.
	if instruction&0x0f000000 == 0x0f000000 {
		if err := c.EnterException(ExceptionSWI, c.pc+4); err != nil {
			return ExecutionResult{}, err
		}
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	// B / BL.
	if instruction&0x0e000000 == 0x0a000000 {
		link := instruction&(1<<24) != 0
		if link {
			c.WriteRegister(14, c.pc+4)
		}
		imm24 := int32(instruction & 0x00ffffff)
		if imm24&(1<<23) != 0 {
			imm24 |= ^int32(0x00ffffff)
		}
		offset := imm24 << 2
		c.SetPC(uint32(int32(c.VisiblePC()) + offset))
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	if instruction&0x0c000000 != 0 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: unsupported ARM instruction 0x%08x", instruction)
	}

	// Multiply and multiply-long occupy the data-processing major opcode but
	// have the distinctive 1001 low nibble.
	if instruction&0x0fc000f0 == 0x00000090 || instruction&0x0f8000f0 == 0x00800090 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM multiply not implemented: 0x%08x", instruction)
	}

	// Halfword/signed transfer encodings also overlap the major opcode.
	if instruction&0x0e000090 == 0x00000090 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM halfword transfer not implemented: 0x%08x", instruction)
	}

	// MRS/MSR use data-processing-looking encodings with S=0 and special
	// operand fields. Reject them until explicit PSR-transfer support lands.
	if instruction&0x0fbf0fff == 0x010f0000 ||
		instruction&0x0db0f000 == 0x0120f000 ||
		instruction&0x0db0f000 == 0x0320f000 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM PSR transfer not implemented: 0x%08x", instruction)
	}

	return c.executeARMDataProcessing(instruction)
}

func (c *CPU) executeARMDataProcessing(instruction uint32) (ExecutionResult, error) {
	opcode := uint8((instruction >> 21) & 0xf)
	setFlags := instruction&(1<<20) != 0
	rn := int((instruction >> 16) & 0xf)
	rd := int((instruction >> 12) & 0xf)

	op2, shifterCarry, err := c.armOperand2(instruction)
	if err != nil {
		return ExecutionResult{}, err
	}
	op1 := c.ReadRegister(rn)

	var result uint32
	var carry, overflow bool
	writeResult := true
	arithmeticFlags := false

	switch opcode {
	case 0x0: // AND
		result = op1 & op2
	case 0x1: // EOR
		result = op1 ^ op2
	case 0x2: // SUB
		result, carry, overflow = addWithCarry(op1, ^op2, true)
		arithmeticFlags = true
	case 0x3: // RSB
		result, carry, overflow = addWithCarry(op2, ^op1, true)
		arithmeticFlags = true
	case 0x4: // ADD
		result, carry, overflow = addWithCarry(op1, op2, false)
		arithmeticFlags = true
	case 0x5: // ADC
		result, carry, overflow = addWithCarry(op1, op2, c.cpsr.Carry())
		arithmeticFlags = true
	case 0x6: // SBC
		result, carry, overflow = addWithCarry(op1, ^op2, c.cpsr.Carry())
		arithmeticFlags = true
	case 0x7: // RSC
		result, carry, overflow = addWithCarry(op2, ^op1, c.cpsr.Carry())
		arithmeticFlags = true
	case 0x8: // TST
		result = op1 & op2
		writeResult = false
		setFlags = true
	case 0x9: // TEQ
		result = op1 ^ op2
		writeResult = false
		setFlags = true
	case 0xa: // CMP
		result, carry, overflow = addWithCarry(op1, ^op2, true)
		writeResult = false
		setFlags = true
		arithmeticFlags = true
	case 0xb: // CMN
		result, carry, overflow = addWithCarry(op1, op2, false)
		writeResult = false
		setFlags = true
		arithmeticFlags = true
	case 0xc: // ORR
		result = op1 | op2
	case 0xd: // MOV
		result = op2
	case 0xe: // BIC
		result = op1 &^ op2
	case 0xf: // MVN
		result = ^op2
	}

	if setFlags {
		if arithmeticFlags {
			c.setNZCV(result, carry, overflow)
		} else {
			c.setNZC(result, shifterCarry)
		}
	}

	if writeResult {
		if rd == 15 {
			if setFlags {
				if err := c.RestoreCPSRFromSPSR(); err != nil {
					return ExecutionResult{}, err
				}
			}
			c.SetPC(result)
			return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
		}
		c.WriteRegister(rd, result)
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: 1}, nil
}

func (c *CPU) armOperand2(instruction uint32) (uint32, bool, error) {
	oldCarry := c.cpsr.Carry()
	if instruction&(1<<25) != 0 {
		imm := uint32(instruction & 0xff)
		rotate := int((instruction>>8)&0xf) * 2
		if rotate == 0 {
			return imm, oldCarry, nil
		}
		value := bits.RotateLeft32(imm, -rotate)
		return value, value>>31 != 0, nil
	}

	if instruction&(1<<4) != 0 {
		return 0, false, fmt.Errorf("arm7tdmi: register-specified ARM shift not implemented: 0x%08x", instruction)
	}

	rm := int(instruction & 0xf)
	value := c.ReadRegister(rm)
	shiftType := uint8((instruction >> 5) & 0x3)
	amount := uint8((instruction >> 7) & 0x1f)
	return shiftImmediate(value, shiftType, amount, oldCarry)
}

func shiftImmediate(value uint32, shiftType, amount uint8, oldCarry bool) (uint32, bool, error) {
	switch shiftType {
	case 0: // LSL
		if amount == 0 {
			return value, oldCarry, nil
		}
		return value << amount, value&(1<<(32-amount)) != 0, nil
	case 1: // LSR; encoded zero means 32
		if amount == 0 {
			return 0, value>>31 != 0, nil
		}
		return value >> amount, value&(1<<(amount-1)) != 0, nil
	case 2: // ASR; encoded zero means 32
		if amount == 0 {
			if value>>31 != 0 {
				return ^uint32(0), true, nil
			}
			return 0, false, nil
		}
		return uint32(int32(value) >> amount), value&(1<<(amount-1)) != 0, nil
	case 3: // ROR; encoded zero is RRX
		if amount == 0 {
			var top uint32
			if oldCarry {
				top = 1 << 31
			}
			return top | value>>1, value&1 != 0, nil
		}
		result := bits.RotateLeft32(value, -int(amount))
		return result, result>>31 != 0, nil
	default:
		return 0, false, fmt.Errorf("arm7tdmi: invalid shift type %d", shiftType)
	}
}

func shiftRegister(value uint32, shiftType, amount uint8, oldCarry bool) (uint32, bool) {
	if amount == 0 {
		return value, oldCarry
	}
	n := uint32(amount)
	switch shiftType {
	case 0: // LSL
		switch {
		case n < 32:
			return value << n, value&(1<<(32-n)) != 0
		case n == 32:
			return 0, value&1 != 0
		default:
			return 0, false
		}
	case 1: // LSR
		switch {
		case n < 32:
			return value >> n, value&(1<<(n-1)) != 0
		case n == 32:
			return 0, value>>31 != 0
		default:
			return 0, false
		}
	case 2: // ASR
		if n >= 32 {
			if value>>31 != 0 {
				return ^uint32(0), true
			}
			return 0, false
		}
		return uint32(int32(value) >> n), value&(1<<(n-1)) != 0
	case 3: // ROR
		rot := n & 31
		if rot == 0 {
			return value, value>>31 != 0
		}
		result := bits.RotateLeft32(value, -int(rot))
		return result, result>>31 != 0
	default:
		panic("arm7tdmi: invalid shift type")
	}
}

func (c *CPU) branchExchange(target uint32) {
	c.SetThumb(target&1 != 0)
	c.SetPC(target)
}

func conditionPassed(cond uint8, psr PSR) bool {
	switch cond {
	case 0x0:
		return psr.Zero()
	case 0x1:
		return !psr.Zero()
	case 0x2:
		return psr.Carry()
	case 0x3:
		return !psr.Carry()
	case 0x4:
		return psr.Negative()
	case 0x5:
		return !psr.Negative()
	case 0x6:
		return psr.Overflow()
	case 0x7:
		return !psr.Overflow()
	case 0x8:
		return psr.Carry() && !psr.Zero()
	case 0x9:
		return !psr.Carry() || psr.Zero()
	case 0xa:
		return psr.Negative() == psr.Overflow()
	case 0xb:
		return psr.Negative() != psr.Overflow()
	case 0xc:
		return !psr.Zero() && psr.Negative() == psr.Overflow()
	case 0xd:
		return psr.Zero() || psr.Negative() != psr.Overflow()
	case 0xe:
		return true
	default:
		// 0xf is never on ARM7TDMI/ARMv4T.
		return false
	}
}

package cpu

import (
	"fmt"
	"math/bits"
)

// ExecuteARM executes one already-fetched ARM instruction.
//
// Memory-transfer and coprocessor encodings are left for follow-up slices;
// unsupported encodings return an error without moving PC. This keeps the
// decoder strict instead of silently treating overlapping ARM encodings as
// data-processing instructions.
func (c *CPU) ExecuteARM(instruction uint32) (ExecutionResult, error) {
	return c.executeARM(instruction, nil)
}

// ExecuteARMWithMemory executes one fetched ARM instruction with data-memory
// access enabled for load/store encodings.
func (c *CPU) ExecuteARMWithMemory(instruction uint32, mem Memory) (ExecutionResult, error) {
	return c.executeARM(instruction, mem)
}

func (c *CPU) executeARM(instruction uint32, mem Memory) (ExecutionResult, error) {
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

	if instruction&0x0e000000 == 0x08000000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeARMBlockTransfer(instruction, mem)
	}

	if instruction&0x0c000000 == 0x04000000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeARMSingleTransfer(instruction, mem)
	}
	if instruction&0x0c000000 != 0 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: unsupported ARM instruction 0x%08x", instruction)
	}

	// SWP/SWPB overlap the data-processing/multiply major opcode.
	if instruction&0x0fb00ff0 == 0x01000090 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeARMSwap(instruction, mem)
	}

	// Multiply and multiply-long occupy the data-processing major opcode but
	// have the distinctive 1001 low nibble.
	if instruction&0x0fc000f0 == 0x00000090 {
		return c.executeARMMultiply(instruction)
	}
	if instruction&0x0f8000f0 == 0x00800090 {
		return c.executeARMMultiplyLong(instruction)
	}

	// Halfword/signed transfer encodings also overlap the major opcode.
	if instruction&0x0e000090 == 0x00000090 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeARMHalfwordTransfer(instruction, mem)
	}

	// MRS/MSR use data-processing-looking encodings with S=0 and special
	// operand fields.
	if instruction&0x0fbf0fff == 0x010f0000 ||
		instruction&0x0db0f000 == 0x0120f000 ||
		instruction&0x0db0f000 == 0x0320f000 {
		return c.executeARMPSRTransfer(instruction)
	}

	return c.executeARMDataProcessing(instruction)
}

func (c *CPU) executeARMDataProcessing(instruction uint32) (ExecutionResult, error) {
	opcode := uint8((instruction >> 21) & 0xf)
	setFlags := instruction&(1<<20) != 0
	rn := int((instruction >> 16) & 0xf)
	rd := int((instruction >> 12) & 0xf)

	if instruction&(1<<4) != 0 && rn == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: register-shifted data processing with r15 Rn is unsupported")
	}
	op2, shifterCarry, coreCycles, err := c.armOperand2(instruction)
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
			return ExecutionResult{InternalCycles: coreCycles, PipelineFlush: true}, nil
		}
		c.WriteRegister(rd, result)
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: coreCycles}, nil
}

func (c *CPU) armOperand2(instruction uint32) (uint32, bool, uint8, error) {
	oldCarry := c.cpsr.Carry()
	if instruction&(1<<25) != 0 {
		imm := uint32(instruction & 0xff)
		rotate := int((instruction>>8)&0xf) * 2
		if rotate == 0 {
			return imm, oldCarry, 1, nil
		}
		value := bits.RotateLeft32(imm, -rotate)
		return value, value>>31 != 0, 1, nil
	}

	rm := int(instruction & 0xf)
	if instruction&(1<<4) == 0 {
		value := c.ReadRegister(rm)
		shiftType := uint8((instruction >> 5) & 0x3)
		amount := uint8((instruction >> 7) & 0x1f)
		value, carry, err := shiftImmediate(value, shiftType, amount, oldCarry)
		return value, carry, 1, err
	}

	// Register-specified shifts take an additional internal cycle. PC operands
	// observe a different pipeline value (+12), so keep those edge cases
	// explicit until the fetch pipeline itself is modeled.
	rs := int((instruction >> 8) & 0xf)
	if rm == 15 || rs == 15 {
		return 0, false, 0, fmt.Errorf("arm7tdmi: register-specified shift using r15 is unsupported")
	}
	value, carry := shiftRegister(c.ReadRegister(rm), uint8((instruction>>5)&0x3), uint8(c.ReadRegister(rs)), oldCarry)
	return value, carry, 2, nil
}

func (c *CPU) executeARMMultiply(instruction uint32) (ExecutionResult, error) {
	accumulate := instruction&(1<<21) != 0
	setFlags := instruction&(1<<20) != 0
	rd := int((instruction >> 16) & 0xf)
	rn := int((instruction >> 12) & 0xf)
	rs := int((instruction >> 8) & 0xf)
	rm := int(instruction & 0xf)

	if rd == 15 || rm == 15 || rs == 15 || (accumulate && rn == 15) {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM multiply using r15 is unpredictable")
	}
	if rd == rm {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM multiply with Rd == Rm is unpredictable on ARM7TDMI")
	}

	result := c.ReadRegister(rm) * c.ReadRegister(rs)
	cycles := multiplyInternalCycles(c.ReadRegister(rs))
	if accumulate {
		result += c.ReadRegister(rn)
		cycles++
	}
	c.WriteRegister(rd, result)
	if setFlags {
		// N/Z are architecturally meaningful. C/V are documented as
		// meaningless/corrupted on ARMv4, so leave their deterministic prior
		// values untouched instead of inventing a false architectural value.
		c.setNZ(result)
	}
	c.advancePC()
	return ExecutionResult{InternalCycles: cycles}, nil
}

func (c *CPU) executeARMMultiplyLong(instruction uint32) (ExecutionResult, error) {
	signed := instruction&(1<<22) != 0
	accumulate := instruction&(1<<21) != 0
	setFlags := instruction&(1<<20) != 0
	rdHi := int((instruction >> 16) & 0xf)
	rdLo := int((instruction >> 12) & 0xf)
	rs := int((instruction >> 8) & 0xf)
	rm := int(instruction & 0xf)

	if rdHi == 15 || rdLo == 15 || rm == 15 || rs == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM long multiply using r15 is unpredictable")
	}
	if rdHi == rdLo {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM long multiply requires distinct destination registers")
	}

	rmValue := c.ReadRegister(rm)
	rsValue := c.ReadRegister(rs)
	var result uint64
	if signed {
		result = uint64(int64(int32(rmValue)) * int64(int32(rsValue)))
	} else {
		result = uint64(rmValue) * uint64(rsValue)
	}
	if accumulate {
		result += uint64(c.ReadRegister(rdHi))<<32 | uint64(c.ReadRegister(rdLo))
	}

	c.WriteRegister(rdLo, uint32(result))
	c.WriteRegister(rdHi, uint32(result>>32))
	if setFlags {
		c.cpsr = c.cpsr.withFlag(FlagNegative, result>>63 != 0)
		c.cpsr = c.cpsr.withFlag(FlagZero, result == 0)
	}

	cycles := multiplyInternalCycles(rsValue) + 1
	if accumulate {
		cycles++
	}
	c.advancePC()
	return ExecutionResult{InternalCycles: cycles}, nil
}

func (c *CPU) executeARMPSRTransfer(instruction uint32) (ExecutionResult, error) {
	// MRS: cond 00010 R 001111 Rd 000000000000.
	if instruction&0x0fbf0fff == 0x010f0000 {
		useSPSR := instruction&(1<<22) != 0
		rd := int((instruction >> 12) & 0xf)
		if rd == 15 {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: MRS with r15 destination is unpredictable")
		}
		value := c.cpsr
		if useSPSR {
			var ok bool
			value, ok = c.SPSR()
			if !ok {
				return ExecutionResult{}, ErrNoSPSR
			}
		}
		c.WriteRegister(rd, uint32(value))
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	immediate := instruction&(1<<25) != 0
	useSPSR := instruction&(1<<22) != 0
	fieldMask := uint8((instruction >> 16) & 0xf)
	mask := psrWriteMask(fieldMask)
	if mask == 0 {
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	var source uint32
	if immediate {
		imm := uint32(instruction & 0xff)
		rotate := int((instruction>>8)&0xf) * 2
		source = bits.RotateLeft32(imm, -rotate)
	} else {
		rm := int(instruction & 0xf)
		if rm == 15 {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: MSR with r15 source is unpredictable")
		}
		source = c.ReadRegister(rm)
	}

	if useSPSR {
		old, ok := c.SPSR()
		if !ok {
			return ExecutionResult{}, ErrNoSPSR
		}
		next := PSR((uint32(old) &^ mask) | (source & mask))
		if mask&0xff != 0 && !next.Mode().valid() {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: MSR produced invalid SPSR mode 0x%02x", uint8(next.Mode()))
		}
		if err := c.SetSPSR(next); err != nil {
			return ExecutionResult{}, err
		}
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// User mode may only update the flags field.
	if c.cpsr.Mode() == ModeUser {
		mask &= 0xff000000
	}
	old := uint32(c.cpsr)
	next := PSR((old &^ mask) | (source & mask))
	if mask&0xff != 0 {
		if !next.Mode().valid() {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: MSR produced invalid CPSR mode 0x%02x", uint8(next.Mode()))
		}
		if next.Thumb() != c.cpsr.Thumb() {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: changing CPSR T bit with MSR is unpredictable")
		}
	}
	if err := c.SetCPSR(next); err != nil {
		return ExecutionResult{}, err
	}
	c.advancePC()
	return ExecutionResult{InternalCycles: 1}, nil
}

func psrWriteMask(fields uint8) uint32 {
	var mask uint32
	if fields&0x8 != 0 {
		mask |= 0xff000000
	}
	if fields&0x4 != 0 {
		mask |= 0x00ff0000
	}
	if fields&0x2 != 0 {
		mask |= 0x0000ff00
	}
	if fields&0x1 != 0 {
		mask |= 0x000000ff
	}
	return mask
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

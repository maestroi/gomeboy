package cpu

import "fmt"

// ExecuteThumb executes one already-fetched Thumb instruction.
func (c *CPU) ExecuteThumb(instruction uint16) (ExecutionResult, error) {
	return c.executeThumb(instruction, nil)
}

// ExecuteThumbWithMemory executes one fetched Thumb instruction with
// data-memory access enabled for load/store encodings.
func (c *CPU) ExecuteThumbWithMemory(instruction uint16, mem Memory) (ExecutionResult, error) {
	return c.executeThumb(instruction, mem)
}

func (c *CPU) executeThumb(instruction uint16, mem Memory) (ExecutionResult, error) {
	if !c.cpsr.Thumb() {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ExecuteThumb called in ARM state")
	}

	// Format 1: move shifted register.
	if instruction&0xe000 == 0x0000 && instruction&0x1800 != 0x1800 {
		shiftType := uint8((instruction >> 11) & 0x3)
		amount := uint8((instruction >> 6) & 0x1f)
		rs := int((instruction >> 3) & 0x7)
		rd := int(instruction & 0x7)
		result, carry, err := shiftImmediate(c.ReadRegister(rs), shiftType, amount, c.cpsr.Carry())
		if err != nil {
			return ExecutionResult{}, err
		}
		c.WriteRegister(rd, result)
		c.setNZC(result, carry)
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// Format 2: add/subtract register or immediate 3-bit.
	if instruction&0xf800 == 0x1800 {
		immediate := instruction&(1<<10) != 0
		subtract := instruction&(1<<9) != 0
		rnOrImm := uint32((instruction >> 6) & 0x7)
		rs := int((instruction >> 3) & 0x7)
		rd := int(instruction & 0x7)
		left := c.ReadRegister(rs)
		right := rnOrImm
		if !immediate {
			right = c.ReadRegister(int(rnOrImm))
		}
		var result uint32
		var carry, overflow bool
		if subtract {
			result, carry, overflow = addWithCarry(left, ^right, true)
		} else {
			result, carry, overflow = addWithCarry(left, right, false)
		}
		c.WriteRegister(rd, result)
		c.setNZCV(result, carry, overflow)
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// Format 3: MOV/CMP/ADD/SUB immediate.
	if instruction&0xe000 == 0x2000 {
		op := uint8((instruction >> 11) & 0x3)
		rd := int((instruction >> 8) & 0x7)
		imm := uint32(instruction & 0xff)
		left := c.ReadRegister(rd)
		var result uint32
		var carry, overflow bool
		switch op {
		case 0: // MOV
			result = imm
			c.WriteRegister(rd, result)
			c.setNZ(result)
		case 1: // CMP
			result, carry, overflow = addWithCarry(left, ^imm, true)
			c.setNZCV(result, carry, overflow)
		case 2: // ADD
			result, carry, overflow = addWithCarry(left, imm, false)
			c.WriteRegister(rd, result)
			c.setNZCV(result, carry, overflow)
		case 3: // SUB
			result, carry, overflow = addWithCarry(left, ^imm, true)
			c.WriteRegister(rd, result)
			c.setNZCV(result, carry, overflow)
		}
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// Format 6: PC-relative LDR.
	if instruction&0xf800 == 0x4800 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeThumbLiteralLoad(instruction, mem)
	}

	// Formats 7/8: register-offset and signed/halfword transfers.
	if instruction&0xf000 == 0x5000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeThumbRegisterTransfer(instruction, mem)
	}

	// Format 9: immediate word/byte transfer.
	if instruction&0xe000 == 0x6000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeThumbImmediateTransfer(instruction, mem)
	}

	// Format 10: immediate halfword transfer.
	if instruction&0xf000 == 0x8000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeThumbHalfwordImmediate(instruction, mem)
	}

	// Format 11: SP-relative word transfer.
	if instruction&0xf000 == 0x9000 {
		if mem == nil {
			return ExecutionResult{}, ErrMemoryRequired
		}
		return c.executeThumbSPRelativeTransfer(instruction, mem)
	}

	// Format 12: load address from PC/SP.
	if instruction&0xf000 == 0xa000 {
		return c.executeThumbLoadAddress(instruction)
	}

	// Format 13: add/subtract immediate to SP.
	if instruction&0xff00 == 0xb000 {
		return c.executeThumbAdjustSP(instruction)
	}

	// Format 4: ALU operations.
	if instruction&0xfc00 == 0x4000 {
		return c.executeThumbALU(instruction)
	}

	// Format 5: high register operations / BX.
	if instruction&0xfc00 == 0x4400 {
		op := uint8((instruction >> 8) & 0x3)
		rs := int((instruction>>3)&0x7) | int((instruction>>3)&0x8)
		rd := int(instruction&0x7) | int((instruction>>4)&0x8)
		left := c.ReadRegister(rd)
		right := c.ReadRegister(rs)
		switch op {
		case 0: // ADD, flags unchanged
			result := left + right
			if rd == 15 {
				c.SetPC(result)
				return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
			}
			c.WriteRegister(rd, result)
		case 1: // CMP
			result, carry, overflow := addWithCarry(left, ^right, true)
			c.setNZCV(result, carry, overflow)
		case 2: // MOV, flags unchanged
			if rd == 15 {
				c.SetPC(right)
				return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
			}
			c.WriteRegister(rd, right)
		case 3: // BX
			c.branchExchange(right)
			return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
		}
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// SWI.
	if instruction&0xff00 == 0xdf00 {
		if err := c.EnterException(ExceptionSWI, c.pc+2); err != nil {
			return ExecutionResult{}, err
		}
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	// Format 16: conditional branch.
	if instruction&0xf000 == 0xd000 {
		cond := uint8((instruction >> 8) & 0xf)
		if cond >= 0xe {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: unsupported Thumb conditional opcode 0x%04x", instruction)
		}
		if conditionPassed(cond, c.cpsr) {
			offset := int32(int8(instruction&0xff)) << 1
			c.SetPC(uint32(int32(c.VisiblePC()) + offset))
			return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
		}
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// Format 18: unconditional branch.
	if instruction&0xf800 == 0xe000 {
		offset := int32(instruction & 0x07ff)
		if offset&(1<<10) != 0 {
			offset |= ^int32(0x07ff)
		}
		offset <<= 1
		c.SetPC(uint32(int32(c.VisiblePC()) + offset))
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	// Format 19: long branch with link, first half.
	if instruction&0xf800 == 0xf000 {
		offset := int32(instruction & 0x07ff)
		if offset&(1<<10) != 0 {
			offset |= ^int32(0x07ff)
		}
		c.WriteRegister(14, uint32(int32(c.VisiblePC())+(offset<<12)))
		c.advancePC()
		return ExecutionResult{InternalCycles: 1}, nil
	}

	// Format 19: long branch with link, second half.
	if instruction&0xf800 == 0xf800 {
		target := c.ReadRegister(14) + uint32(instruction&0x07ff)<<1
		c.WriteRegister(14, (c.pc+2)|1)
		c.SetPC(target)
		return ExecutionResult{InternalCycles: 1, PipelineFlush: true}, nil
	}

	return ExecutionResult{}, fmt.Errorf("arm7tdmi: unsupported Thumb instruction 0x%04x", instruction)
}

func (c *CPU) executeThumbALU(instruction uint16) (ExecutionResult, error) {
	op := uint8((instruction >> 6) & 0xf)
	rs := int((instruction >> 3) & 0x7)
	rd := int(instruction & 0x7)
	left := c.ReadRegister(rd)
	right := c.ReadRegister(rs)
	var result uint32
	var carry, overflow bool
	write := true
	setLogicalCarry := false
	arithmetic := false

	switch op {
	case 0x0: // AND
		result = left & right
	case 0x1: // EOR
		result = left ^ right
	case 0x2: // LSL register
		result, carry = shiftRegister(left, 0, uint8(right), c.cpsr.Carry())
		setLogicalCarry = true
	case 0x3: // LSR register
		result, carry = shiftRegister(left, 1, uint8(right), c.cpsr.Carry())
		setLogicalCarry = true
	case 0x4: // ASR register
		result, carry = shiftRegister(left, 2, uint8(right), c.cpsr.Carry())
		setLogicalCarry = true
	case 0x5: // ADC
		result, carry, overflow = addWithCarry(left, right, c.cpsr.Carry())
		arithmetic = true
	case 0x6: // SBC
		result, carry, overflow = addWithCarry(left, ^right, c.cpsr.Carry())
		arithmetic = true
	case 0x7: // ROR register
		result, carry = shiftRegister(left, 3, uint8(right), c.cpsr.Carry())
		setLogicalCarry = true
	case 0x8: // TST
		result = left & right
		write = false
	case 0x9: // NEG
		result, carry, overflow = addWithCarry(0, ^right, true)
		arithmetic = true
	case 0xa: // CMP
		result, carry, overflow = addWithCarry(left, ^right, true)
		arithmetic = true
		write = false
	case 0xb: // CMN
		result, carry, overflow = addWithCarry(left, right, false)
		arithmetic = true
		write = false
	case 0xc: // ORR
		result = left | right
	case 0xd: // MUL
		result = left * right
	case 0xe: // BIC
		result = left &^ right
	case 0xf: // MVN
		result = ^right
	}

	if write {
		c.WriteRegister(rd, result)
	}
	if arithmetic {
		c.setNZCV(result, carry, overflow)
	} else if setLogicalCarry {
		c.setNZC(result, carry)
	} else {
		// Logical operations and MUL update N/Z while preserving C/V.
		c.setNZ(result)
	}
	c.advancePC()

	cycles := uint8(1)
	if op == 0xd {
		// ARM7TDMI MUL timing depends on the high bytes of the multiplier.
		// Expose a conservative internal-cycle estimate now; bus timing is
		// added by the future GBA memory layer.
		cycles = multiplyInternalCycles(right)
	}
	return ExecutionResult{InternalCycles: cycles}, nil
}

func multiplyInternalCycles(multiplier uint32) uint8 {
	// ARM7TDMI early termination examines progressively larger high portions
	// of the multiplier. m=1 when bits 31:8 are all zero or all one, m=2 when
	// bits 31:16 are all zero/all one, m=3 when bits 31:24 are all zero/all
	// one, otherwise m=4. Return the internal m cycles only.
	if hi := multiplier & 0xffffff00; hi == 0 || hi == 0xffffff00 {
		return 1
	}
	if hi := multiplier & 0xffff0000; hi == 0 || hi == 0xffff0000 {
		return 2
	}
	if hi := multiplier & 0xff000000; hi == 0 || hi == 0xff000000 {
		return 3
	}
	return 4
}

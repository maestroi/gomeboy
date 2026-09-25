package cpu

import (
	"fmt"

	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

func (c *CPU) executeARMSingleTransfer(instruction uint32, mem Memory) (ExecutionResult, error) {
	preIndex := instruction&(1<<24) != 0
	addOffset := instruction&(1<<23) != 0
	byteTransfer := instruction&(1<<22) != 0
	writeBackBit := instruction&(1<<21) != 0
	load := instruction&(1<<20) != 0
	rn := int((instruction >> 16) & 0xf)
	rd := int((instruction >> 12) & 0xf)

	offset, err := c.armTransferOffset(instruction)
	if err != nil {
		return ExecutionResult{}, err
	}
	base := c.ReadRegister(rn)
	indexed := base
	if addOffset {
		indexed += offset
	} else {
		indexed -= offset
	}

	address := base
	writeBack := !preIndex || writeBackBit
	if preIndex {
		address = indexed
	}
	if writeBack && rn == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM transfer writeback to r15 is unpredictable")
	}
	if load && writeBack && rn == rd {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: ARM load with writeback and Rn == Rd is unpredictable")
	}
	if byteTransfer && rd == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: LDRB/STRB using r15 is unpredictable")
	}

	access := gbamemory.Access{}
	var cycles uint32
	if load {
		var value uint32
		if byteTransfer {
			var raw byte
			raw, cycles = mem.Read8(address, access)
			value = uint32(raw)
		} else {
			value, cycles = mem.Read32(address, access)
		}

		if writeBack {
			c.WriteRegister(rn, indexed)
		}
		if rd == 15 {
			c.SetPC(value)
			return ExecutionResult{
				InternalCycles: 1,
				MemoryCycles:   cycles,
				PipelineFlush:  true,
			}, nil
		}
		c.WriteRegister(rd, value)
	} else {
		value := c.ReadRegister(rd)
		if rd == 15 {
			// ARM7TDMI stores PC as the current instruction address +12.
			value = c.pc + 12
		}
		if byteTransfer {
			cycles = mem.Write8(address, byte(value), access)
		} else {
			cycles = mem.Write32(address, value, access)
		}
		if writeBack {
			c.WriteRegister(rn, indexed)
		}
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) armTransferOffset(instruction uint32) (uint32, error) {
	if instruction&(1<<25) == 0 {
		return instruction & 0x0fff, nil
	}
	if instruction&(1<<4) != 0 {
		return 0, fmt.Errorf("arm7tdmi: shifted register transfer requires bit4=0")
	}
	rm := int(instruction & 0xf)
	shiftType := uint8((instruction >> 5) & 0x3)
	amount := uint8((instruction >> 7) & 0x1f)
	value, _, err := shiftImmediate(c.ReadRegister(rm), shiftType, amount, c.cpsr.Carry())
	return value, err
}

func (c *CPU) executeARMHalfwordTransfer(instruction uint32, mem Memory) (ExecutionResult, error) {
	preIndex := instruction&(1<<24) != 0
	addOffset := instruction&(1<<23) != 0
	immediate := instruction&(1<<22) != 0
	writeBackBit := instruction&(1<<21) != 0
	load := instruction&(1<<20) != 0
	rn := int((instruction >> 16) & 0xf)
	rd := int((instruction >> 12) & 0xf)
	kind := uint8((instruction >> 5) & 0x3)

	if kind == 0 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: unsupported ARM swap/reserved transfer 0x%08x", instruction)
	}
	if rd == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: halfword/signed transfer using r15 is unpredictable")
	}
	if !load && kind != 1 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: signed ARM store encoding is invalid")
	}

	var offset uint32
	if immediate {
		offset = ((instruction >> 8) & 0xf) << 4
		offset |= instruction & 0xf
	} else {
		if instruction&0x00000f00 != 0 {
			return ExecutionResult{}, fmt.Errorf("arm7tdmi: malformed register halfword transfer")
		}
		offset = c.ReadRegister(int(instruction & 0xf))
	}

	base := c.ReadRegister(rn)
	indexed := base
	if addOffset {
		indexed += offset
	} else {
		indexed -= offset
	}
	address := base
	writeBack := !preIndex || writeBackBit
	if preIndex {
		address = indexed
	}
	if writeBack && rn == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: halfword transfer writeback to r15 is unpredictable")
	}
	if load && writeBack && rn == rd {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: halfword load with writeback and Rn == Rd is unpredictable")
	}

	access := gbamemory.Access{}
	var cycles uint32
	if load {
		var value uint32
		switch kind {
		case 1: // LDRH
			var raw uint16
			raw, cycles = mem.Read16(address, access)
			value = uint32(raw)
		case 2: // LDRSB
			var raw byte
			raw, cycles = mem.Read8(address, access)
			value = uint32(int32(int8(raw)))
		case 3: // LDRSH
			if address&1 != 0 {
				var raw byte
				raw, cycles = mem.Read8(address, access)
				value = uint32(int32(int8(raw)))
			} else {
				var raw uint16
				raw, cycles = mem.Read16(address, access)
				value = uint32(int32(int16(raw)))
			}
		}
		if writeBack {
			c.WriteRegister(rn, indexed)
		}
		c.WriteRegister(rd, value)
	} else {
		cycles = mem.Write16(address, uint16(c.ReadRegister(rd)), access)
		if writeBack {
			c.WriteRegister(rn, indexed)
		}
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

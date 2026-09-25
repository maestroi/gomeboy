package cpu

import gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"

func (c *CPU) executeThumbLiteralLoad(instruction uint16, mem Memory) (ExecutionResult, error) {
	rd := int((instruction >> 8) & 0x7)
	address := (c.VisiblePC() &^ 3) + uint32(instruction&0xff)<<2
	value, cycles := mem.Read32(address, gbamemory.Access{})
	c.WriteRegister(rd, value)
	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) executeThumbRegisterTransfer(instruction uint16, mem Memory) (ExecutionResult, error) {
	op := uint8((instruction >> 9) & 0x7)
	rm := int((instruction >> 6) & 0x7)
	rn := int((instruction >> 3) & 0x7)
	rd := int(instruction & 0x7)
	address := c.ReadRegister(rn) + c.ReadRegister(rm)
	access := gbamemory.Access{}

	var cycles uint32
	switch op {
	case 0: // STR
		cycles = mem.Write32(address, c.ReadRegister(rd), access)
	case 1: // STRH
		cycles = mem.Write16(address, uint16(c.ReadRegister(rd)), access)
	case 2: // STRB
		cycles = mem.Write8(address, byte(c.ReadRegister(rd)), access)
	case 3: // LDRSB
		raw, n := mem.Read8(address, access)
		cycles = n
		c.WriteRegister(rd, uint32(int32(int8(raw))))
	case 4: // LDR
		value, n := mem.Read32(address, access)
		cycles = n
		c.WriteRegister(rd, value)
	case 5: // LDRH
		value, n := mem.Read16(address, access)
		cycles = n
		c.WriteRegister(rd, uint32(value))
	case 6: // LDRB
		value, n := mem.Read8(address, access)
		cycles = n
		c.WriteRegister(rd, uint32(value))
	case 7: // LDRSH
		if address&1 != 0 {
			raw, n := mem.Read8(address, access)
			cycles = n
			c.WriteRegister(rd, uint32(int32(int8(raw))))
		} else {
			raw, n := mem.Read16(address, access)
			cycles = n
			c.WriteRegister(rd, uint32(int32(int16(raw))))
		}
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) executeThumbImmediateTransfer(instruction uint16, mem Memory) (ExecutionResult, error) {
	byteTransfer := instruction&(1<<12) != 0
	load := instruction&(1<<11) != 0
	offset := uint32((instruction >> 6) & 0x1f)
	rn := int((instruction >> 3) & 0x7)
	rd := int(instruction & 0x7)
	if !byteTransfer {
		offset <<= 2
	}
	address := c.ReadRegister(rn) + offset
	access := gbamemory.Access{}

	var cycles uint32
	if load {
		if byteTransfer {
			value, n := mem.Read8(address, access)
			cycles = n
			c.WriteRegister(rd, uint32(value))
		} else {
			value, n := mem.Read32(address, access)
			cycles = n
			c.WriteRegister(rd, value)
		}
	} else if byteTransfer {
		cycles = mem.Write8(address, byte(c.ReadRegister(rd)), access)
	} else {
		cycles = mem.Write32(address, c.ReadRegister(rd), access)
	}

	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) executeThumbHalfwordImmediate(instruction uint16, mem Memory) (ExecutionResult, error) {
	load := instruction&(1<<11) != 0
	offset := uint32((instruction>>6)&0x1f) << 1
	rn := int((instruction >> 3) & 0x7)
	rd := int(instruction & 0x7)
	address := c.ReadRegister(rn) + offset
	access := gbamemory.Access{}

	var cycles uint32
	if load {
		value, n := mem.Read16(address, access)
		cycles = n
		c.WriteRegister(rd, uint32(value))
	} else {
		cycles = mem.Write16(address, uint16(c.ReadRegister(rd)), access)
	}
	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) executeThumbSPRelativeTransfer(instruction uint16, mem Memory) (ExecutionResult, error) {
	load := instruction&(1<<11) != 0
	rd := int((instruction >> 8) & 0x7)
	address := c.ReadRegister(13) + uint32(instruction&0xff)<<2
	access := gbamemory.Access{}

	var cycles uint32
	if load {
		value, n := mem.Read32(address, access)
		cycles = n
		c.WriteRegister(rd, value)
	} else {
		cycles = mem.Write32(address, c.ReadRegister(rd), access)
	}
	c.advancePC()
	return ExecutionResult{InternalCycles: 1, MemoryCycles: cycles}, nil
}

func (c *CPU) executeThumbLoadAddress(instruction uint16) (ExecutionResult, error) {
	useSP := instruction&(1<<11) != 0
	rd := int((instruction >> 8) & 0x7)
	base := c.VisiblePC() &^ 3
	if useSP {
		base = c.ReadRegister(13)
	}
	c.WriteRegister(rd, base+uint32(instruction&0xff)<<2)
	c.advancePC()
	return ExecutionResult{InternalCycles: 1}, nil
}

func (c *CPU) executeThumbAdjustSP(instruction uint16) (ExecutionResult, error) {
	offset := uint32(instruction&0x7f) << 2
	sp := c.ReadRegister(13)
	if instruction&(1<<7) != 0 {
		sp -= offset
	} else {
		sp += offset
	}
	c.WriteRegister(13, sp)
	c.advancePC()
	return ExecutionResult{InternalCycles: 1}, nil
}

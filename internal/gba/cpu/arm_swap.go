package cpu

import (
	"fmt"

	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

func (c *CPU) executeARMSwap(instruction uint32, mem Memory) (ExecutionResult, error) {
	byteSwap := instruction&(1<<22) != 0
	rn := int((instruction >> 16) & 0xf)
	rd := int((instruction >> 12) & 0xf)
	rm := int(instruction & 0xf)

	if rn == 15 || rd == 15 || rm == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: SWP/SWPB using r15 is unpredictable")
	}
	if rn == rd || rn == rm {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: SWP/SWPB base register overlapping source/destination is unpredictable")
	}

	address := c.ReadRegister(rn)
	source := c.ReadRegister(rm)
	access := gbamemory.Access{Locked: true}

	var memoryCycles uint32
	if byteSwap {
		old, readCycles := mem.Read8(address, access)
		writeCycles := mem.Write8(address, byte(source), access)
		memoryCycles = readCycles + writeCycles
		c.WriteRegister(rd, uint32(old))
	} else {
		// SWP word accesses have the same alignment behavior as an LDR followed
		// by STR: an unaligned read rotates the aligned word, while the write
		// is aligned down by the bus.
		old, readCycles := mem.Read32(address, access)
		writeCycles := mem.Write32(address, source, access)
		memoryCycles = readCycles + writeCycles
		c.WriteRegister(rd, old)
	}

	c.advancePC()
	return ExecutionResult{
		InternalCycles: 1,
		MemoryCycles:   memoryCycles,
	}, nil
}

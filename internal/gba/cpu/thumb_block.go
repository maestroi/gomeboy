package cpu

import (
	"math/bits"

	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

func (c *CPU) executeThumbPushPop(instruction uint16, mem Memory) (ExecutionResult, error) {
	load := instruction&(1<<11) != 0
	extra := instruction&(1<<8) != 0
	registerList := uint16(instruction & 0xff)
	if extra {
		if load {
			registerList |= 1 << 15
		} else {
			registerList |= 1 << 14
		}
	}

	addressCount := bits.OnesCount16(registerList)
	if registerList == 0 {
		// ARM7TDMI empty-list quirk inherited by Thumb multiple transfers.
		registerList = 1 << 15
		addressCount = 16
	}

	sp := c.ReadRegister(13)
	var address, writeBack uint32
	if load {
		address = sp &^ 3
		writeBack = sp + uint32(addressCount*4)
	} else {
		writeBack = sp - uint32(addressCount*4)
		address = writeBack &^ 3
	}

	transferIndex := 0
	var memoryCycles uint32
	var loadedPC uint32
	pcLoaded := false

	for reg := 0; reg < 16; reg++ {
		if registerList&(1<<reg) == 0 {
			continue
		}
		access := gbamemory.Access{Sequential: transferIndex != 0}
		if load {
			value, cycles := mem.Read32(address, access)
			memoryCycles += cycles
			if reg == 15 {
				loadedPC = value
				pcLoaded = true
			} else {
				c.WriteRegister(reg, value)
			}
		} else {
			value := c.ReadRegister(reg)
			if reg == 15 {
				value = c.VisiblePC()
			}
			memoryCycles += mem.Write32(address, value, access)
		}
		address += 4
		transferIndex++
	}

	c.WriteRegister(13, writeBack)
	if pcLoaded {
		c.SetPC(loadedPC)
		return ExecutionResult{
			InternalCycles: 1,
			MemoryCycles:   memoryCycles,
			PipelineFlush:  true,
		}, nil
	}

	c.advancePC()
	internal := uint8(0)
	if load {
		internal = 1
	}
	return ExecutionResult{InternalCycles: internal, MemoryCycles: memoryCycles}, nil
}

func (c *CPU) executeThumbMultiple(instruction uint16, mem Memory) (ExecutionResult, error) {
	load := instruction&(1<<11) != 0
	rb := int((instruction >> 8) & 0x7)
	registerList := uint16(instruction & 0xff)

	addressCount := bits.OnesCount16(registerList)
	if registerList == 0 {
		registerList = 1 << 15
		addressCount = 16
	}

	base := c.ReadRegister(rb)
	address := base &^ 3
	writeBack := base + uint32(addressCount*4)
	firstReg := bits.TrailingZeros16(registerList)

	transferIndex := 0
	var memoryCycles uint32
	var loadedPC uint32
	pcLoaded := false

	for reg := 0; reg < 16; reg++ {
		if registerList&(1<<reg) == 0 {
			continue
		}
		access := gbamemory.Access{Sequential: transferIndex != 0}
		if load {
			value, cycles := mem.Read32(address, access)
			memoryCycles += cycles
			if reg == 15 {
				loadedPC = value
				pcLoaded = true
			} else {
				c.WriteRegister(reg, value)
			}
		} else {
			value := c.ReadRegister(reg)
			if reg == 15 {
				value = c.VisiblePC()
			} else if reg == rb && reg != firstReg {
				// On ARM7TDMI a base register stored after the first transfer
				// observes the updated writeback value.
				value = writeBack
			}
			memoryCycles += mem.Write32(address, value, access)
		}
		address += 4
		transferIndex++
	}

	// Thumb LDMIA suppresses writeback when Rb is in the register list; the
	// loaded value remains in Rb. STMIA always writes back.
	if !load || registerList&(1<<rb) == 0 {
		c.WriteRegister(rb, writeBack)
	}

	if pcLoaded {
		c.SetPC(loadedPC)
		return ExecutionResult{
			InternalCycles: 1,
			MemoryCycles:   memoryCycles,
			PipelineFlush:  true,
		}, nil
	}

	c.advancePC()
	internal := uint8(0)
	if load {
		internal = 1
	}
	return ExecutionResult{InternalCycles: internal, MemoryCycles: memoryCycles}, nil
}

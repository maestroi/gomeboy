package cpu

import (
	"fmt"
	"math/bits"

	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

func (c *CPU) executeARMBlockTransfer(instruction uint32, mem Memory) (ExecutionResult, error) {
	preIndex := instruction&(1<<24) != 0
	up := instruction&(1<<23) != 0
	psrOrUser := instruction&(1<<22) != 0
	writeBack := instruction&(1<<21) != 0
	load := instruction&(1<<20) != 0
	rn := int((instruction >> 16) & 0xf)
	rawList := uint16(instruction)

	if rn == 15 {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: LDM/STM using r15 as base is unpredictable")
	}

	registerList := rawList
	addressCount := bits.OnesCount16(registerList)
	if registerList == 0 {
		// ARM7TDMI/GBA empty-rlist quirk: transfer r15 while updating the
		// base as though sixteen registers had been listed.
		registerList = 1 << 15
		addressCount = 16
	}

	hasPC := registerList&(1<<15) != 0
	userBank := psrOrUser && (!load || !hasPC)
	if userBank && writeBack {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: user-bank LDM/STM with writeback is unpredictable")
	}
	if psrOrUser && c.cpsr.Mode() == ModeUser {
		return ExecutionResult{}, fmt.Errorf("arm7tdmi: LDM/STM ^ is unpredictable in user mode")
	}

	base := c.ReadRegister(rn)
	delta := uint32(addressCount * 4)
	writeBackValue := base + delta
	if !up {
		writeBackValue = base - delta
	}

	var address uint32
	switch {
	case up && !preIndex: // IA
		address = base
	case up && preIndex: // IB
		address = base + 4
	case !up && !preIndex: // DA
		address = base - delta + 4
	case !up && preIndex: // DB
		address = base - delta
	}
	address &^= 3

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
			} else if userBank {
				c.writeUserRegister(reg, value)
			} else {
				c.WriteRegister(reg, value)
			}
		} else {
			var value uint32
			switch {
			case reg == 15:
				value = c.pc + 12
			case writeBack && reg == rn && reg != firstReg:
				// ARM7TDMI writes back after the first store cycle. If Rn is
				// later in the list, the stored value is the updated base.
				value = writeBackValue
			case userBank:
				value = c.readUserRegister(reg)
			default:
				value = c.ReadRegister(reg)
			}
			memoryCycles += mem.Write32(address, value, access)
		}

		address += 4
		transferIndex++
	}

	if writeBack {
		// LDM with Rn in the list finishes with the loaded value in Rn; that
		// loaded value overwrites the transient writeback value.
		if !load || registerList&(1<<rn) == 0 {
			c.WriteRegister(rn, writeBackValue)
		}
	}

	if pcLoaded {
		if psrOrUser {
			if err := c.RestoreCPSRFromSPSR(); err != nil {
				return ExecutionResult{}, err
			}
		}
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

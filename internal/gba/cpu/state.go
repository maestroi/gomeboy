package cpu

// State is a portable snapshot of ARM7TDMI execution state.
type State struct {
	Low             [8]uint32
	UserHigh        [5]uint32
	FIQHigh         [5]uint32
	UserSPLR        [2]uint32
	BankSPLR        [5][2]uint32
	PC              uint32
	CPSR            PSR
	SPSR            [5]PSR
	FetchSequential bool
	IRQLine         bool
	FIQLine         bool
}

func (c *CPU) Snapshot() State {
	return State{
		Low: c.low, UserHigh: c.userHigh, FIQHigh: c.fiqHigh,
		UserSPLR: c.userSPLR, BankSPLR: c.bankSPLR, PC: c.pc,
		CPSR: c.cpsr, SPSR: c.spsr, FetchSequential: c.fetchSequential,
		IRQLine: c.irqLine, FIQLine: c.fiqLine,
	}
}

func (c *CPU) Restore(s State) {
	c.low = s.Low
	c.userHigh = s.UserHigh
	c.fiqHigh = s.FIQHigh
	c.userSPLR = s.UserSPLR
	c.bankSPLR = s.BankSPLR
	c.pc = s.PC
	c.cpsr = s.CPSR
	c.spsr = s.SPSR
	c.fetchSequential = s.FetchSequential
	c.irqLine = s.IRQLine
	c.fiqLine = s.FIQLine
}

package cpu

import "github.com/maestroi/gomeboy/internal/types"

const (
	// speedSwitchHaltCycles is SameBoy's speed_switch_halt_countdown: a KEY1
	// STOP with no pending interrupt halts the CPU for this many CPU T-cycles
	// (in whichever speed is active, so 0x10004 device dots in double speed).
	speedSwitchHaltCycles = 0x20008
	// speedSwitchDelay is speed_switch_countdown: the speed flip lands this
	// many cycles after STOP when entering double speed.
	speedSwitchDelay = 6
	// speedSwitchFreeze is speed_switch_freeze: the PPU is frozen for the
	// first cycles of the switch.
	speedSwitchFreeze = 5
)

// keyOneSwitch performs the KEY1 speed switch triggered by STOP, following
// SameBoy's stop(). The caller has already handled DIV reset and PC++.
func (c *CPU) keyOneSwitch() {
	enteringDS := !c.DoubleSpeed
	noIRQ := !c.b.HasInterrupts()

	// STOP's second byte is a real bus read when no interrupt is pending.
	if noIRQ {
		c.s.Tick(4)
	}

	c.DoubleSpeed = !c.DoubleSpeed
	if c.DoubleSpeed {
		c.b.SetBit(types.KEY1, types.Bit7)
	} else {
		c.b.ClearBit(types.KEY1, types.Bit7)
	}
	c.b.ClearBit(types.KEY1, types.Bit0) // clear armed bit

	if !noIRQ {
		c.s.ChangeSpeed(c.DoubleSpeed)
		return
	}

	c.s.Halted = true
	c.Halted = true

	left := uint64(speedSwitchHaltCycles)
	step := func(n uint64, frozen uint64) {
		if frozen > 0 {
			c.s.TickFrozen(frozen)
		}
		c.s.Tick(n - frozen)
		left -= n
	}

	if enteringDS {
		// First cycles run at normal speed; the flip lands mid-way.
		step(speedSwitchDelay, speedSwitchFreeze)
	} else {
		step(speedSwitchFreeze, speedSwitchFreeze)
	}
	c.s.ChangeSpeed(c.DoubleSpeed)

	// Halt in M-cycles; an interrupt wakes the CPU early. Clear the frame
	// latches so a halt spanning several VBlanks is not cut short.
	for left > 0 && !c.b.HasInterrupts() {
		c.hasFrame = false
		c.hasInt = false
		step(min(left, 4), 0)
	}

	c.s.Halted = false
	c.Halted = false
	c.hasFrame = false
	c.hasInt = false
}

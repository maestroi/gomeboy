package cpu

import "github.com/maestroi/gomeboy/internal/types"

const (
	// speedSwitchHaltCycles is SameBoy's speed_switch_halt_countdown: a KEY1
	// STOP with no pending interrupthalts the CPU for this many CPU T-cycles.
	speedSwitchHaltCycles = 0x20008
	// speedSwitchDelay is speed_switch_countdown: when entering double speed
	// the flip lands this many cycles after STOP.
	speedSwitchDelay = 6
	// speedSwitchFreeze is speed_switch_freeze: the PPU is frozen for the
	// first cycles of a no-IRQ speed switch.
	speedSwitchFreeze = 5
)

// keyOneSwitch performs the KEY1 speed switch triggered by STOP, following
// SameBoy's stop() + GB_advance_cycles speed_switch_* handling.
// The caller has already handled DIV reset and PC++.
func (c *CPU) keyOneSwitch() {
	enteringDS := !c.DoubleSpeed
	noIRQ := !c.b.HasInterrupts()

	// STOP's second byte is a real bus read when no interrupt is pending.
	if noIRQ {
		c.s.Tick(4)
	}

	// KEY1 is cleared immediately (armed + current-speed bit), matching
	// SameBoy writing KEY1 = 0. The CGB double-speed flag is updated below.
	c.b.Set(types.KEY1, 0)

	if !noIRQ {
		// Interrupt pending: flip immediately, no halt/freeze.
		c.DoubleSpeed = !c.DoubleSpeed
		if c.DoubleSpeed {
			c.b.SetBit(types.KEY1, types.Bit7)
		}
		c.s.ChangeSpeed(c.DoubleSpeed)
		return
	}

	c.s.Halted = true
	c.Halted = true

	left := uint64(speedSwitchHaltCycles)

	haltStep := func(n uint64) {
		c.hasFrame = false
		c.hasInt = false
		c.s.Tick(n)
		left -= n
	}

	if enteringDS {
		// SameBoy: countdown=6, freeze=5 (no-IRQ). PPU frozen for 5 cycles
		// at normal speed, then one more NS cycle, then the flip.
		c.s.TickFrozen(speedSwitchFreeze)
		left -= speedSwitchFreeze
		haltStep(speedSwitchDelay - speedSwitchFreeze)
		c.DoubleSpeed = true
		c.b.SetBit(types.KEY1, types.Bit7)
		c.s.ChangeSpeed(true)
	} else {
		// SameBoy: leaving DS flips cgb_double_speed immediately, then
		// freeze=5 + halt. PPU freeze and halt run at the new (NS) rate.
		c.DoubleSpeed = false
		c.s.ChangeSpeed(false)
		c.s.TickFrozen(speedSwitchFreeze)
		left -= speedSwitchFreeze
	}

	// Halt remainder in M-cycles; an interrupt wakes the CPU early.
	for left > 0 && !c.b.HasInterrupts() {
		haltStep(min(left, 4))
	}

	c.s.Halted = false
	c.Halted = false
	c.hasFrame = false
	c.hasInt = false
}

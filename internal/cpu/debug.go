package cpu

import "github.com/thelolagemann/gomeboy/internal/types"

// InstructionStep describes one debugger-level CPU step. Executed is false
// when the step serviced an interrupt before fetching the next opcode. Frames
// counts display frame boundaries crossed while the CPU was halted or while
// the instruction/interrupt was executing.
type InstructionStep struct {
	Opcode    uint8
	Executed  bool
	Interrupt bool
	Frames    uint64
}

// StepInstruction advances the SM83 by one debugger-level step. Ordinarily
// that is exactly one opcode. If an interrupt is pending before the next
// opcode, the interrupt service sequence is the step instead. While HALTed,
// scheduler time is advanced until the CPU can make progress; any frame
// boundaries crossed are reported in Frames.
//
// This method deliberately mirrors the prefetch/interrupt timing in Frame but
// does not use Debug/DebugBreakpoint, so debugger stepping cannot perturb the
// emulated program's own debug flags.
func (c *CPU) StepInstruction() InstructionStep {
	var out InstructionStep

	// Frame() resumes a HALT that stopped only because a frame boundary was
	// reached. A debugger step has no frame-loop caller to do that for it, so
	// continue skipping until the CPU actually wakes. Count every crossed
	// frame so GameBoy can keep its public frame counter correct.
	for c.skippingHalt {
		c.skippingHalt = false
		c.hasFrame = false
		c.hasInt = false
		c.skipHALT()
		if c.hasFrame {
			out.Frames++
			c.hasFrame = false
		}
		c.hasInt = false
	}

	// Match Frame's instruction-prefetch timing and interrupt check.
	c.s.Tick(2)
	if c.b.CanInterrupt() {
		c.s.Tick(2)
		c.serviceDebuggerInterrupt()
		out.Interrupt = true
		if c.hasFrame {
			out.Frames++
		}
		c.hasFrame = false
		c.hasInt = false
		return out
	}
	c.s.Tick(2)

	if c.haltBug {
		c.PC--
		c.haltBug = false
	}

	op := c.b.Read(c.PC)
	c.PC++
	out.Opcode = op
	out.Executed = true
	InstructionSet[op].fn(c)

	if c.hasFrame {
		out.Frames++
	}
	// hasInt/hasFrame are Frame's loop-control latches, not hardware-visible
	// interrupt state. The bus still retains IF/IE/IME for the next step.
	c.hasFrame = false
	c.hasInt = false
	return out
}

func (c *CPU) serviceDebuggerInterrupt() {
	c.b.Debugf(" %04x Servicing Interrupt %08b\n", c.PC, c.b.Get(types.IE))
	c.s.Tick(4)
	c.s.Tick(4)

	c.SP--
	c.b.ClockedWrite(c.SP, uint8(c.PC>>8))

	irq := c.b.Get(types.IE)

	c.SP--
	c.b.ClockedWrite(c.SP, uint8(c.PC&0xff))

	c.PC = c.b.IRQVector(irq)
	c.s.Tick(4)
	c.b.DisableInterrupts()
}

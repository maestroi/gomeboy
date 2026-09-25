package cpu

import (
	"errors"
	"fmt"
)

// ExecutionResult describes CPU-internal timing/control-flow effects of an
// instruction. Bus fetch/refill timing is deliberately not included.
type ExecutionResult struct {
	InternalCycles uint8
	PipelineFlush  bool
}

// Exception identifies an ARM exception vector.
type Exception uint8

const (
	ExceptionUndefined Exception = iota
	ExceptionSWI
	ExceptionPrefetchAbort
	ExceptionDataAbort
	ExceptionIRQ
	ExceptionFIQ
)

var ErrNoSPSR = errors.New("arm7tdmi: current mode has no SPSR")

// CPU contains architectural ARM7TDMI state.
//
// Registers are stored by physical bank rather than copied on mode switches.
// That keeps bank switching explicit and avoids losing hidden register values.
type CPU struct {
	low [8]uint32

	userHigh [5]uint32
	fiqHigh  [5]uint32

	userSPLR [2]uint32
	bankSPLR [5][2]uint32 // FIQ, IRQ, SVC, ABT, UND

	pc   uint32
	cpsr PSR
	spsr [5]PSR
}

// New returns an ARM7TDMI in reset state: ARM state, Supervisor mode, IRQ/FIQ
// masked, PC at the reset vector.
func New() *CPU {
	c := &CPU{}
	c.Reset()
	return c
}

// Reset restores architectural reset state.
func (c *CPU) Reset() {
	*c = CPU{}
	c.cpsr = PSR(ModeSupervisor) | FlagIRQDisable | FlagFIQDisable
	c.pc = 0
}

func bankIndex(mode Mode) (int, bool) {
	switch mode {
	case ModeFIQ:
		return 0, true
	case ModeIRQ:
		return 1, true
	case ModeSupervisor:
		return 2, true
	case ModeAbort:
		return 3, true
	case ModeUndefined:
		return 4, true
	default:
		return 0, false
	}
}

// CPSR returns the current program status register.
func (c *CPU) CPSR() PSR { return c.cpsr }

// SetCPSR replaces CPSR after validating the encoded processor mode.
func (c *CPU) SetCPSR(psr PSR) error {
	if !psr.Mode().valid() {
		return fmt.Errorf("arm7tdmi: invalid CPSR mode 0x%02x", uint8(psr.Mode()))
	}
	c.cpsr = psr
	c.alignPC()
	return nil
}

// SetMode changes processor mode while preserving all other CPSR bits.
func (c *CPU) SetMode(mode Mode) error {
	psr, err := c.cpsr.withMode(mode)
	if err != nil {
		return err
	}
	c.cpsr = psr
	return nil
}

// SetThumb switches instruction-set state and aligns PC for that state.
func (c *CPU) SetThumb(thumb bool) {
	c.cpsr = c.cpsr.withFlag(FlagThumb, thumb)
	c.alignPC()
}

// SPSR returns the saved status register for the current exception mode.
func (c *CPU) SPSR() (PSR, bool) {
	idx, ok := bankIndex(c.cpsr.Mode())
	if !ok {
		return 0, false
	}
	return c.spsr[idx], true
}

// SetSPSR replaces the saved status register for the current exception mode.
func (c *CPU) SetSPSR(psr PSR) error {
	idx, ok := bankIndex(c.cpsr.Mode())
	if !ok {
		return ErrNoSPSR
	}
	c.spsr[idx] = psr
	return nil
}

// RestoreCPSRFromSPSR performs the status-register half of exception return.
func (c *CPU) RestoreCPSRFromSPSR() error {
	psr, ok := c.SPSR()
	if !ok {
		return ErrNoSPSR
	}
	return c.SetCPSR(psr)
}

// PC is the address of the instruction currently being executed.
func (c *CPU) PC() uint32 { return c.pc }

// VisiblePC returns the architectural value observed when an instruction reads
// r15: current instruction +8 in ARM state and +4 in Thumb state.
func (c *CPU) VisiblePC() uint32 {
	if c.cpsr.Thumb() {
		return c.pc + 4
	}
	return c.pc + 8
}

// SetPC sets the execution address, applying ARM/Thumb alignment.
func (c *CPU) SetPC(pc uint32) {
	c.pc = pc
	c.alignPC()
}

func (c *CPU) alignPC() {
	if c.cpsr.Thumb() {
		c.pc &^= 1
	} else {
		c.pc &^= 3
	}
}

func (c *CPU) advancePC() {
	if c.cpsr.Thumb() {
		c.pc += 2
	} else {
		c.pc += 4
	}
}

// ReadRegister returns the architectural value of r0-r15.
func (c *CPU) ReadRegister(reg int) uint32 {
	if reg < 0 || reg > 15 {
		panic(fmt.Sprintf("arm7tdmi: invalid register r%d", reg))
	}
	switch {
	case reg < 8:
		return c.low[reg]
	case reg < 13:
		if c.cpsr.Mode() == ModeFIQ {
			return c.fiqHigh[reg-8]
		}
		return c.userHigh[reg-8]
	case reg < 15:
		if idx, ok := bankIndex(c.cpsr.Mode()); ok {
			return c.bankSPLR[idx][reg-13]
		}
		return c.userSPLR[reg-13]
	default:
		return c.VisiblePC()
	}
}

// WriteRegister writes r0-r15. Writing r15 branches using current state.
func (c *CPU) WriteRegister(reg int, value uint32) {
	if reg < 0 || reg > 15 {
		panic(fmt.Sprintf("arm7tdmi: invalid register r%d", reg))
	}
	switch {
	case reg < 8:
		c.low[reg] = value
	case reg < 13:
		if c.cpsr.Mode() == ModeFIQ {
			c.fiqHigh[reg-8] = value
		} else {
			c.userHigh[reg-8] = value
		}
	case reg < 15:
		if idx, ok := bankIndex(c.cpsr.Mode()); ok {
			c.bankSPLR[idx][reg-13] = value
		} else {
			c.userSPLR[reg-13] = value
		}
	default:
		c.SetPC(value)
	}
}

// EnterException saves CPSR into the target mode's SPSR, writes the supplied
// architectural return address into that mode's LR, enters ARM state, masks
// interrupts as required, and jumps to the exception vector.
//
// The caller supplies returnAddress because the exact LR value differs between
// exception sources and pipeline positions.
func (c *CPU) EnterException(kind Exception, returnAddress uint32) error {
	mode, vector, maskIRQ, maskFIQ, err := exceptionTarget(kind)
	if err != nil {
		return err
	}

	old := c.cpsr
	next, _ := old.withMode(mode)
	next &^= FlagThumb
	next = next.withFlag(FlagIRQDisable, maskIRQ || old.IRQDisabled())
	next = next.withFlag(FlagFIQDisable, maskFIQ || old.FIQDisabled())

	c.cpsr = next
	idx, _ := bankIndex(mode)
	c.spsr[idx] = old
	c.WriteRegister(14, returnAddress)
	c.pc = vector
	return nil
}

func exceptionTarget(kind Exception) (Mode, uint32, bool, bool, error) {
	switch kind {
	case ExceptionUndefined:
		return ModeUndefined, 0x04, true, false, nil
	case ExceptionSWI:
		return ModeSupervisor, 0x08, true, false, nil
	case ExceptionPrefetchAbort:
		return ModeAbort, 0x0c, true, false, nil
	case ExceptionDataAbort:
		return ModeAbort, 0x10, true, false, nil
	case ExceptionIRQ:
		return ModeIRQ, 0x18, true, false, nil
	case ExceptionFIQ:
		return ModeFIQ, 0x1c, true, true, nil
	default:
		return 0, 0, false, false, fmt.Errorf("arm7tdmi: unknown exception %d", kind)
	}
}

func (c *CPU) setNZ(value uint32) {
	c.cpsr = c.cpsr.withFlag(FlagNegative, value>>31 != 0)
	c.cpsr = c.cpsr.withFlag(FlagZero, value == 0)
}

func (c *CPU) setNZC(value uint32, carry bool) {
	c.setNZ(value)
	c.cpsr = c.cpsr.withFlag(FlagCarry, carry)
}

func (c *CPU) setNZCV(value uint32, carry, overflow bool) {
	c.setNZC(value, carry)
	c.cpsr = c.cpsr.withFlag(FlagOverflow, overflow)
}

func addWithCarry(x, y uint32, carryIn bool) (result uint32, carry, overflow bool) {
	var cin uint64
	if carryIn {
		cin = 1
	}
	unsigned := uint64(x) + uint64(y) + cin
	result = uint32(unsigned)
	carry = unsigned>>32 != 0

	sx := int64(int32(x))
	sy := int64(int32(y))
	sc := int64(cin)
	signed := sx + sy + sc
	overflow = signed > int64(^uint32(0)>>1) || signed < -1<<31
	return
}

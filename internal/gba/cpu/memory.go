package cpu

import (
	"errors"

	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

var ErrMemoryRequired = errors.New("arm7tdmi: instruction requires attached memory")

// Memory is the CPU-facing GBA memory boundary. The concrete bus implements
// this interface, while unit tests may provide smaller deterministic memories.
type Memory interface {
	Read8(addr uint32, access gbamemory.Access) (byte, uint32)
	Read16(addr uint32, access gbamemory.Access) (uint16, uint32)
	Read32(addr uint32, access gbamemory.Access) (uint32, uint32)
	Write8(addr uint32, value byte, access gbamemory.Access) uint32
	Write16(addr uint32, value uint16, access gbamemory.Access) uint32
	Write32(addr uint32, value uint32, access gbamemory.Access) uint32
	Idle(cycles uint32)
}

// StepResult separates opcode-fetch, data-memory, and CPU-internal timing so a
// future scheduler can consume them without hiding where time was spent.
type StepResult struct {
	FetchCycles    uint32
	MemoryCycles   uint32
	InternalCycles uint8
	TotalCycles    uint32
	PipelineFlush  bool

	ExceptionTaken bool
	Exception      Exception
}

// Step fetches and executes one instruction at the current PC.
//
// Straight-line fetches become sequential after the first instruction.
// Branches/exceptions request a non-sequential fetch for the next step.
func (c *CPU) Step(mem Memory) (StepResult, error) {
	if mem == nil {
		return StepResult{}, ErrMemoryRequired
	}

	if kind, ok := c.pendingInterrupt(); ok {
		// At an IRQ/FIQ boundary c.pc is the address of the instruction that
		// loses priority to the interrupt. ARM7TDMI records PC+4 in LR for
		// both ARM and Thumb interrupt entry.
		if err := c.EnterException(kind, c.pc+4); err != nil {
			return StepResult{}, err
		}
		return StepResult{
			InternalCycles: 1,
			TotalCycles:    1,
			PipelineFlush:  true,
			ExceptionTaken: true,
			Exception:      kind,
		}, nil
	}

	access := gbamemory.Access{
		Sequential: c.fetchSequential,
		Instruction: true,
	}

	var (
		exec  ExecutionResult
		fetch uint32
		err   error
	)
	if c.cpsr.Thumb() {
		var instruction uint16
		instruction, fetch = mem.Read16(c.pc, access)
		exec, err = c.ExecuteThumbWithMemory(instruction, mem)
	} else {
		var instruction uint32
		instruction, fetch = mem.Read32(c.pc, access)
		exec, err = c.ExecuteARMWithMemory(instruction, mem)
	}
	if err != nil {
		return StepResult{FetchCycles: fetch}, err
	}

	if exec.InternalCycles != 0 {
		mem.Idle(uint32(exec.InternalCycles))
	}

	c.fetchSequential = !exec.PipelineFlush
	total := fetch + exec.MemoryCycles + uint32(exec.InternalCycles)
	return StepResult{
		FetchCycles:    fetch,
		MemoryCycles:   exec.MemoryCycles,
		InternalCycles: exec.InternalCycles,
		TotalCycles:    total,
		PipelineFlush:  exec.PipelineFlush,
	}, nil
}

func (c *CPU) pendingInterrupt() (Exception, bool) {
	if c.fiqLine && !c.cpsr.FIQDisabled() {
		return ExceptionFIQ, true
	}
	if c.irqLine && !c.cpsr.IRQDisabled() {
		return ExceptionIRQ, true
	}
	return 0, false
}

package gomeboy

import "fmt"

// Condition is a stopping predicate for RunUntil or RunCPUUntil.
// Implementations supplied by this package are side-effect free with respect
// to the emulator. Stateful conditions such as MemoryChanged keep only their
// own observation/latch state.
type Condition interface {
	Match(*Emulator) bool
	String() string
}

type conditionFunc struct {
	name string
	fn   func(*Emulator) bool
}

func (c conditionFunc) Match(e *Emulator) bool { return c.fn(e) }
func (c conditionFunc) String() string         { return c.name }

// ConditionFunc turns a caller-provided predicate into a named condition. The
// callback must not step or mutate the emulator.
func ConditionFunc(name string, fn func(*Emulator) bool) Condition {
	if name == "" {
		name = "custom condition"
	}
	return conditionFunc{name: name, fn: fn}
}

// MemoryEquals stops when the side-effect-free byte at addr equals value.
func MemoryEquals(addr uint16, value byte) Condition {
	return conditionFunc{
		name: fmt.Sprintf("memory[%04X] == %02X", addr, value),
		fn:   func(e *Emulator) bool { return e.Peek8(addr) == value },
	}
}

// MemoryMaskedEquals stops when memory[addr]&mask equals value&mask.
func MemoryMaskedEquals(addr uint16, mask, value byte) Condition {
	return conditionFunc{
		name: fmt.Sprintf("memory[%04X] & %02X == %02X", addr, mask, value&mask),
		fn:   func(e *Emulator) bool { return e.Peek8(addr)&mask == value&mask },
	}
}

// MemoryChanged latches true after the observed byte first differs from the
// value seen on the condition's first Match call. Both RunUntil and
// RunCPUUntil perform an initial Match before stepping, so a newly constructed
// condition naturally means "changed since this run started".
func MemoryChanged(addr uint16) Condition {
	var before byte
	initialised := false
	triggered := false
	return conditionFunc{
		name: fmt.Sprintf("memory[%04X] changed", addr),
		fn: func(e *Emulator) bool {
			if triggered {
				return true
			}
			v := e.Peek8(addr)
			if !initialised {
				before = v
				initialised = true
				return false
			}
			if v != before {
				triggered = true
			}
			return triggered
		},
	}
}

// MemoryWatchpoint is the debugger-oriented name for MemoryChanged. With
// RunCPUUntil it is checked after every debugger-level CPU step, so a memory
// value that changes and later changes back within one display frame is still
// observed. It watches state changes, not attempted writes that leave the byte
// unchanged.
func MemoryWatchpoint(addr uint16) Condition { return MemoryChanged(addr) }

// PCEquals stops when the CPU program counter reaches pc.
func PCEquals(pc uint16) Condition {
	return conditionFunc{
		name: fmt.Sprintf("PC == %04X", pc),
		fn:   func(e *Emulator) bool { return e.gb.CPU.PC == pc },
	}
}

// FrameAtLeast stops once the emulator frame counter reaches frame.
func FrameAtLeast(frame uint64) Condition {
	return conditionFunc{
		name: fmt.Sprintf("frame >= %d", frame),
		fn:   func(e *Emulator) bool { return e.FrameCount() >= frame },
	}
}

// CycleAtLeast stops once the master clock reaches cycle.
func CycleAtLeast(cycle uint64) Condition {
	return conditionFunc{
		name: fmt.Sprintf("cycle >= %d", cycle),
		fn:   func(e *Emulator) bool { return e.Cycle() >= cycle },
	}
}

// InterruptPending stops when at least one enabled hardware interrupt is
// pending, regardless of whether IME currently allows it to be serviced.
func InterruptPending() Condition {
	return conditionFunc{
		name: "enabled interrupt pending",
		fn: func(e *Emulator) bool {
			cpu := e.CPUState()
			return cpu.IE&cpu.IF&0x1f != 0
		},
	}
}

// Any stops when any child condition matches.
func Any(conditions ...Condition) Condition {
	return conditionFunc{
		name: "any condition",
		fn: func(e *Emulator) bool {
			for _, c := range conditions {
				if c != nil && c.Match(e) {
					return true
				}
			}
			return false
		},
	}
}

// All stops when every child condition matches. An empty All never matches.
// Every child is evaluated on every check, even after one fails, so stateful
// predicates establish/update their observations from the same run boundary.
func All(conditions ...Condition) Condition {
	return conditionFunc{
		name: "all conditions",
		fn: func(e *Emulator) bool {
			if len(conditions) == 0 {
				return false
			}
			matched := true
			for _, c := range conditions {
				if c == nil {
					matched = false
					continue
				}
				if !c.Match(e) {
					matched = false
				}
			}
			return matched
		},
	}
}

// RunStop describes why a bounded condition run returned. FramesStepped is
// populated by both runners. CPUSteps is non-zero only for RunCPUUntil; one
// CPU step is one StepInstruction call and can represent interrupt service
// instead of an executed opcode.
type RunStop struct {
	Matched        bool
	ConditionIndex int
	Condition      string
	FramesStepped  uint64
	CPUSteps       uint64
	Frame          uint64
	Cycle          uint64
}

func validateConditions(op string, limit uint64, limitName string, conditions []Condition) error {
	if limit == 0 {
		return fmt.Errorf("gomeboy: %s: %s must be > 0", op, limitName)
	}
	if len(conditions) == 0 {
		return fmt.Errorf("gomeboy: %s: at least one condition is required", op)
	}
	for i, c := range conditions {
		if c == nil {
			return fmt.Errorf("gomeboy: %s: condition %d is nil", op, i)
		}
	}
	return nil
}

func (e *Emulator) matchedCondition(conditions []Condition, frames, cpuSteps uint64) (RunStop, bool) {
	for i, c := range conditions {
		if c.Match(e) {
			return RunStop{
				Matched:        true,
				ConditionIndex: i,
				Condition:      c.String(),
				FramesStepped:  frames,
				CPUSteps:       cpuSteps,
				Frame:          e.FrameCount(),
				Cycle:          e.Cycle(),
			}, true
		}
	}
	return RunStop{}, false
}

// RunUntil advances at most maxFrames display frames and stops as soon as one
// condition matches. Conditions are checked once before the first step and
// again after every frame. Reaching maxFrames is a normal result with
// Matched=false; invalid arguments return an error.
func (e *Emulator) RunUntil(maxFrames uint64, conditions ...Condition) (RunStop, error) {
	if err := validateConditions("RunUntil", maxFrames, "maxFrames", conditions); err != nil {
		return RunStop{}, err
	}
	if stop, ok := e.matchedCondition(conditions, 0, 0); ok {
		return stop, nil
	}
	for stepped := uint64(1); stepped <= maxFrames; stepped++ {
		e.StepFrame()
		if stop, ok := e.matchedCondition(conditions, stepped, 0); ok {
			return stop, nil
		}
	}
	return RunStop{
		ConditionIndex: -1,
		FramesStepped:  maxFrames,
		Frame:          e.FrameCount(),
		Cycle:          e.Cycle(),
	}, nil
}

// RunCPUUntil advances at most maxSteps debugger-level CPU steps and stops as
// soon as one condition matches. Conditions are checked before the first step
// and after every StepInstruction call. This is the appropriate runner for
// instruction-granular memory watchpoints and PC breakpoints.
//
// A CPU step normally executes one opcode, but a pending interrupt may consume
// a step before another opcode is fetched, and HALT may cross frame boundaries
// while waiting for the CPU to make progress. CPUSteps therefore counts
// debugger steps, while FramesStepped reports display frames crossed.
func (e *Emulator) RunCPUUntil(maxSteps uint64, conditions ...Condition) (RunStop, error) {
	if err := validateConditions("RunCPUUntil", maxSteps, "maxSteps", conditions); err != nil {
		return RunStop{}, err
	}
	if stop, ok := e.matchedCondition(conditions, 0, 0); ok {
		return stop, nil
	}
	startFrame := e.FrameCount()
	for stepped := uint64(1); stepped <= maxSteps; stepped++ {
		e.StepInstruction()
		frames := e.FrameCount() - startFrame
		if stop, ok := e.matchedCondition(conditions, frames, stepped); ok {
			return stop, nil
		}
	}
	return RunStop{
		ConditionIndex: -1,
		FramesStepped:  e.FrameCount() - startFrame,
		CPUSteps:       maxSteps,
		Frame:          e.FrameCount(),
		Cycle:          e.Cycle(),
	}, nil
}

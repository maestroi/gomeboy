package gomeboy

import "fmt"

// Condition is a reusable stopping predicate for RunUntil. Implementations
// supplied by this package are side-effect free with respect to the emulator;
// MemoryChanged keeps only its own previous observation.
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

// ConditionFunc turns a caller-provided predicate into a named RunUntil
// condition. The callback must not step or mutate the emulator.
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

// MemoryChanged stops after the observed byte differs from the value seen on
// the condition's first Match call. RunUntil performs an initial Match before
// stepping, so this naturally means "changed since RunUntil started".
func MemoryChanged(addr uint16) Condition {
	var before byte
	initialised := false
	return conditionFunc{
		name: fmt.Sprintf("memory[%04X] changed", addr),
		fn: func(e *Emulator) bool {
			v := e.Peek8(addr)
			if !initialised {
				before = v
				initialised = true
				return false
			}
			return v != before
		},
	}
}

// PCEquals stops when the CPU program counter reaches pc.
func PCEquals(pc uint16) Condition {
	return conditionFunc{
		name: fmt.Sprintf("PC == %04X", pc),
		fn:   func(e *Emulator) bool { return e.gb.CPU.PC == pc },
	}
}

// CycleAtLeast stops once the master clock reaches cycle.
func CycleAtLeast(cycle uint64) Condition {
	return conditionFunc{
		name: fmt.Sprintf("cycle >= %d", cycle),
		fn:   func(e *Emulator) bool { return e.Cycle() >= cycle },
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
// predicates such as MemoryChanged establish/update their own observations
// from the same RunUntil boundary.
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

// RunStop describes why RunUntil returned.
type RunStop struct {
	Matched        bool
	ConditionIndex int
	Condition      string
	FramesStepped  uint64
	Frame          uint64
	Cycle          uint64
}

// RunUntil advances at most maxFrames frames and stops as soon as one
// condition matches. Conditions are checked once before the first step and
// again after every frame. Reaching maxFrames is a normal result with
// Matched=false; invalid arguments return an error.
func (e *Emulator) RunUntil(maxFrames uint64, conditions ...Condition) (RunStop, error) {
	if maxFrames == 0 {
		return RunStop{}, fmt.Errorf("gomeboy: RunUntil: maxFrames must be > 0")
	}
	if len(conditions) == 0 {
		return RunStop{}, fmt.Errorf("gomeboy: RunUntil: at least one condition is required")
	}
	for i, c := range conditions {
		if c == nil {
			return RunStop{}, fmt.Errorf("gomeboy: RunUntil: condition %d is nil", i)
		}
	}

	check := func(stepped uint64) (RunStop, bool) {
		for i, c := range conditions {
			if c.Match(e) {
				return RunStop{
					Matched:        true,
					ConditionIndex: i,
					Condition:      c.String(),
					FramesStepped:  stepped,
					Frame:          e.FrameCount(),
					Cycle:          e.Cycle(),
				}, true
			}
		}
		return RunStop{}, false
	}

	if stop, ok := check(0); ok {
		return stop, nil
	}
	for stepped := uint64(1); stepped <= maxFrames; stepped++ {
		e.StepFrame()
		if stop, ok := check(stepped); ok {
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

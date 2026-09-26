// Package conformance provides deterministic, headless execution of GBA test
// ROMs against the integrated GBA machine.
package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/system"
)

// Status is the terminal outcome of a conformance test ROM.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
)

// BootMode controls how execution begins.
type BootMode string

const (
	// BootDirect starts at EntryPoint (or the first Game Pak ROM window when
	// EntryPoint is zero). This is intended for BIOS-independent test ROMs.
	BootDirect BootMode = "direct"
	// BootReset leaves the CPU at the architectural reset vector. A BIOS image
	// is therefore normally required for useful execution.
	BootReset BootMode = "reset"
)

// RegisterValue describes one architectural register initializer.
type RegisterValue struct {
	Register int
	Value    uint32
}

// Condition describes one deterministic emulator-visible predicate.
type Condition struct {
	Type     string `json:"type"`
	Address  uint32 `json:"address,omitempty"`
	Width    uint8  `json:"width,omitempty"`
	Register int    `json:"register,omitempty"`
	Value    uint32 `json:"value"`
	Mask     uint32 `json:"mask,omitempty"`
}

// Limits bounds a ROM run. Zero leaves a particular limit disabled, but every
// case must enable at least one limit.
type Limits struct {
	Steps  uint64 `json:"steps,omitempty"`
	Cycles uint64 `json:"cycles,omitempty"`
	Frames uint64 `json:"frames,omitempty"`
}

// FailureGroup maps an upstream test-number range to a human-readable group.
type FailureGroup struct {
	Name  string
	First uint32
	Last  uint32
}

// Progress describes a first-failure suite convention. When the case passes,
// every numbered check is known to have passed. When a failure condition
// matches, FailureRegister identifies the first failed upstream test number.
type Progress struct {
	FailureRegister int
	Groups          []FailureGroup
}

// Case is one ROM execution request.
type Case struct {
	Name             string
	ROM              []byte
	ROMSHA256        string
	BIOS             []byte
	Boot             BootMode
	EntryPoint       uint32
	InitialCPSR      *uint32
	InitialRegisters []RegisterValue
	Limits           Limits
	PassAll          []Condition
	FailAny          []Condition
	Progress         *Progress
}

// CheckSummary reports check-level progress for suites that expose an ordered
// first-failure test number.
type CheckSummary struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	NotRun int `json:"not_run"`
	Total  int `json:"total"`
}

// FailureInfo records structured context for a first-failure convention.
type FailureInfo struct {
	TestID   uint32 `json:"test_id"`
	Group    string `json:"group,omitempty"`
	Register string `json:"register"`
}

// Result is the stable machine-readable outcome for one ROM.
type Result struct {
	Suite         string        `json:"suite"`
	Test          string        `json:"test"`
	Status        Status        `json:"status"`
	Cycles        uint64        `json:"cycles"`
	Frames        uint64        `json:"frames"`
	Steps         uint64        `json:"steps"`
	Checks        *CheckSummary `json:"checks,omitempty"`
	Failure       *FailureInfo  `json:"failure,omitempty"`
	Detail        string        `json:"detail,omitempty"`
	GomeBoyCommit string        `json:"gomeboy_commit"`
	SuiteRevision string        `json:"suite_revision"`
	ROMSHA256     string        `json:"rom_sha256"`
}

// Summary contains aggregate counts without collapsing different suites or
// categories into a single accuracy percentage.
type Summary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	TimedOut int `json:"timed_out"`
	Total    int `json:"total"`
}

// Report is the deterministic JSON document emitted by the runner.
type Report struct {
	SchemaVersion int      `json:"schema_version"`
	Suite         string   `json:"suite"`
	SuiteRevision string   `json:"suite_revision"`
	GomeBoyCommit string   `json:"gomeboy_commit"`
	Summary       Summary  `json:"summary"`
	Results       []Result `json:"results"`
}

// Runner supplies suite/build metadata shared by a set of test cases.
type Runner struct {
	Suite         string
	SuiteRevision string
	GomeBoyCommit string
}

// Run executes one case until it passes, fails, hits an emulation error, or
// exhausts one of its declared deterministic budgets.
func (r Runner) Run(tc Case) Result {
	result := Result{
		Suite:         r.Suite,
		Test:          tc.Name,
		Status:        StatusFail,
		GomeBoyCommit: r.GomeBoyCommit,
		SuiteRevision: r.SuiteRevision,
		ROMSHA256:     sha256Hex(tc.ROM),
	}

	if err := validateCase(tc); err != nil {
		result.Detail = err.Error()
		return result
	}
	if tc.ROMSHA256 != "" && tc.ROMSHA256 != result.ROMSHA256 {
		result.Detail = fmt.Sprintf("ROM SHA-256 mismatch: got %s, want %s", result.ROMSHA256, tc.ROMSHA256)
		return result
	}

	m := system.New(tc.BIOS, tc.ROM)
	if tc.Boot == BootDirect {
		if tc.InitialCPSR != nil {
			if err := m.CPU.SetCPSR(cpu.PSR(*tc.InitialCPSR)); err != nil {
				result.Detail = fmt.Sprintf("initialize CPSR: %v", err)
				return result
			}
		}
		for _, register := range tc.InitialRegisters {
			m.CPU.WriteRegister(register.Register, register.Value)
		}
		entry := tc.EntryPoint
		if entry == 0 {
			entry = bus.ROM0Start
		}
		m.CPU.SetPC(entry)
	}

	startCycle := m.Cycle()
	startFrame := m.PPU.FrameCount()
	for {
		if matched, detail, err := anyMatch(m, tc.FailAny); err != nil {
			result.Detail = err.Error()
			return result
		} else if matched {
			result.Detail = "failure condition matched: " + detail
			applyFailureProgress(&result, m, tc.Progress)
			return result
		}
		if matched, err := allMatch(m, tc.PassAll); err != nil {
			result.Detail = err.Error()
			return result
		} else if matched {
			result.Status = StatusPass
			applyPassProgress(&result, tc.Progress)
			return result
		}
		if limitReached(tc.Limits, result) {
			result.Status = StatusTimeout
			result.Detail = "execution budget exhausted before a terminal condition matched"
			return result
		}

		_, err := m.Step()
		result.Steps++
		result.Cycles = m.Cycle() - startCycle
		result.Frames = m.PPU.FrameCount() - startFrame
		if err != nil {
			result.Detail = fmt.Sprintf("emulation error: %v", err)
			return result
		}
	}
}

// RunAll executes cases in manifest order and returns aggregate counts.
func (r Runner) RunAll(cases []Case) Report {
	report := Report{
		SchemaVersion: 1,
		Suite:         r.Suite,
		SuiteRevision: r.SuiteRevision,
		GomeBoyCommit: r.GomeBoyCommit,
		Results:       make([]Result, 0, len(cases)),
	}
	for _, tc := range cases {
		result := r.Run(tc)
		report.Results = append(report.Results, result)
		report.Summary.Total++
		switch result.Status {
		case StatusPass:
			report.Summary.Passed++
		case StatusTimeout:
			report.Summary.TimedOut++
		default:
			report.Summary.Failed++
		}
	}
	return report
}

// Passed reports whether every executed case passed.
func (r Report) Passed() bool {
	return r.Summary.Total > 0 && r.Summary.Passed == r.Summary.Total
}

func validateCase(tc Case) error {
	if tc.Name == "" {
		return fmt.Errorf("conformance: test name is required")
	}
	if len(tc.ROM) == 0 {
		return fmt.Errorf("conformance: %s has an empty ROM", tc.Name)
	}
	if tc.Boot != BootDirect && tc.Boot != BootReset {
		return fmt.Errorf("conformance: %s has invalid boot mode %q", tc.Name, tc.Boot)
	}
	if tc.Boot == BootReset && (tc.InitialCPSR != nil || len(tc.InitialRegisters) != 0) {
		return fmt.Errorf("conformance: %s reset boot cannot override initial CPU state", tc.Name)
	}
	for _, register := range tc.InitialRegisters {
		if register.Register < 0 || register.Register > 14 {
			return fmt.Errorf("conformance: %s boot register must name r0-r14, got r%d", tc.Name, register.Register)
		}
	}
	if tc.Limits.Steps == 0 && tc.Limits.Cycles == 0 && tc.Limits.Frames == 0 {
		return fmt.Errorf("conformance: %s must declare a step, cycle, or frame limit", tc.Name)
	}
	if len(tc.PassAll) == 0 {
		return fmt.Errorf("conformance: %s must declare at least one pass condition", tc.Name)
	}
	return validateProgress(tc.Progress)
}

func validateProgress(progress *Progress) error {
	if progress == nil {
		return nil
	}
	if progress.FailureRegister < 0 || progress.FailureRegister > 14 {
		return fmt.Errorf("conformance: progress failure register must name r0-r14, got r%d", progress.FailureRegister)
	}
	if len(progress.Groups) == 0 {
		return fmt.Errorf("conformance: progress must declare at least one group")
	}
	var previousLast uint32
	for i, group := range progress.Groups {
		if group.Name == "" || group.First == 0 || group.Last < group.First {
			return fmt.Errorf("conformance: invalid progress group %+v", group)
		}
		if i != 0 && group.First <= previousLast {
			return fmt.Errorf("conformance: progress groups must be ordered and non-overlapping")
		}
		previousLast = group.Last
	}
	return nil
}

func limitReached(l Limits, r Result) bool {
	return (l.Steps != 0 && r.Steps >= l.Steps) ||
		(l.Cycles != 0 && r.Cycles >= l.Cycles) ||
		(l.Frames != 0 && r.Frames >= l.Frames)
}

func allMatch(m *system.Machine, conditions []Condition) (bool, error) {
	for _, condition := range conditions {
		matched, err := condition.matches(m)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func anyMatch(m *system.Machine, conditions []Condition) (bool, string, error) {
	for _, condition := range conditions {
		matched, err := condition.matches(m)
		if err != nil {
			return false, "", err
		}
		if matched {
			return true, condition.String(), nil
		}
	}
	return false, "", nil
}

func (c Condition) matches(m *system.Machine) (bool, error) {
	var got uint32
	switch c.Type {
	case "memory":
		switch c.Width {
		case 1:
			got = uint32(m.Bus.Peek8(c.Address))
		case 2:
			got = uint32(m.Bus.Peek8(c.Address)) | uint32(m.Bus.Peek8(c.Address+1))<<8
		case 4:
			got = peek32(m, c.Address)
		default:
			return false, fmt.Errorf("conformance: memory condition width must be 1, 2, or 4, got %d", c.Width)
		}
	case "register":
		if c.Register < 0 || c.Register > 14 {
			return false, fmt.Errorf("conformance: register condition must name r0-r14, got r%d", c.Register)
		}
		got = m.CPU.ReadRegister(c.Register)
	case "pc":
		got = m.CPU.PC()
	case "swi":
		if m.CPU.CPSR().Thumb() {
			instruction := uint16(m.Bus.Peek8(m.CPU.PC())) | uint16(m.Bus.Peek8(m.CPU.PC()+1))<<8
			if instruction>>8 != 0xdf {
				return false, nil
			}
			got = uint32(instruction & 0xff)
		} else {
			instruction := peek32(m, m.CPU.PC())
			if instruction>>24 != 0xef {
				return false, nil
			}
			got = instruction & 0x00ffffff
		}
	case "self_loop":
		if m.CPU.CPSR().Thumb() {
			instruction := uint16(m.Bus.Peek8(m.CPU.PC())) | uint16(m.Bus.Peek8(m.CPU.PC()+1))<<8
			return instruction == 0xe7fe, nil
		}
		return peek32(m, m.CPU.PC()) == 0xeafffffe, nil
	default:
		return false, fmt.Errorf("conformance: unknown condition type %q", c.Type)
	}

	mask := c.Mask
	if mask == 0 {
		switch c.Type {
		case "memory":
			switch c.Width {
			case 1:
				mask = 0xff
			case 2:
				mask = 0xffff
			default:
				mask = ^uint32(0)
			}
		case "swi":
			if m.CPU.CPSR().Thumb() {
				mask = 0xff
			} else {
				mask = 0x00ffffff
			}
		default:
			mask = ^uint32(0)
		}
	}
	return got&mask == c.Value&mask, nil
}

func (c Condition) String() string {
	switch c.Type {
	case "memory":
		return fmt.Sprintf("memory[%#08x]/%d == %#x", c.Address, c.Width, c.Value)
	case "register":
		return fmt.Sprintf("r%d == %#x", c.Register, c.Value)
	case "pc":
		return fmt.Sprintf("pc == %#x", c.Value)
	case "swi":
		return fmt.Sprintf("swi %#x", c.Value)
	case "self_loop":
		return "self-loop branch"
	default:
		return c.Type
	}
}

func peek32(m *system.Machine, addr uint32) uint32 {
	return uint32(m.Bus.Peek8(addr)) |
		uint32(m.Bus.Peek8(addr+1))<<8 |
		uint32(m.Bus.Peek8(addr+2))<<16 |
		uint32(m.Bus.Peek8(addr+3))<<24
}

func applyPassProgress(result *Result, progress *Progress) {
	if progress == nil {
		return
	}
	total := progressTotal(progress)
	result.Checks = &CheckSummary{Passed: total, Total: total}
}

func applyFailureProgress(result *Result, m *system.Machine, progress *Progress) {
	if progress == nil {
		return
	}
	testID := m.CPU.ReadRegister(progress.FailureRegister)
	info := &FailureInfo{
		TestID:   testID,
		Register: fmt.Sprintf("r%d", progress.FailureRegister),
	}
	total := progressTotal(progress)
	passed := 0
	known := false
	for _, group := range progress.Groups {
		count := int(group.Last - group.First + 1)
		if testID >= group.First && testID <= group.Last {
			info.Group = group.Name
			passed += int(testID - group.First)
			known = true
			break
		}
		passed += count
	}
	result.Failure = info
	if known {
		result.Checks = &CheckSummary{
			Passed: passed,
			Failed: 1,
			NotRun: total - passed - 1,
			Total:  total,
		}
		result.Detail += fmt.Sprintf("; %s=%d; group=%s", info.Register, testID, info.Group)
	} else {
		result.Detail += fmt.Sprintf("; %s=%d (outside declared progress groups)", info.Register, testID)
	}
}

func progressTotal(progress *Progress) int {
	total := 0
	for _, group := range progress.Groups {
		total += int(group.Last - group.First + 1)
	}
	return total
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

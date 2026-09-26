// Package conformance provides deterministic, headless execution of GBA test
// ROMs against the integrated GBA machine.
package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/maestroi/gomeboy/internal/gba/bus"
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

// Case is one ROM execution request.
type Case struct {
	Name       string
	ROM        []byte
	ROMSHA256  string
	BIOS       []byte
	Boot       BootMode
	EntryPoint uint32
	Limits     Limits
	PassAll    []Condition
	FailAny    []Condition
}

// Result is the stable machine-readable outcome for one ROM.
type Result struct {
	Suite         string `json:"suite"`
	Test          string `json:"test"`
	Status        Status `json:"status"`
	Cycles        uint64 `json:"cycles"`
	Frames        uint64 `json:"frames"`
	Steps         uint64 `json:"steps"`
	Detail        string `json:"detail,omitempty"`
	GomeBoyCommit string `json:"gomeboy_commit"`
	SuiteRevision string `json:"suite_revision"`
	ROMSHA256     string `json:"rom_sha256"`
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
		entry := tc.EntryPoint
		if entry == 0 {
			entry = bus.ROM0Start
		}
		m.CPU.SetPC(entry)
	}

	startCycle := m.Cycle()
	startFrame := m.PPU.FrameCount()
	for {
		_, err := m.Step()
		result.Steps++
		result.Cycles = m.Cycle() - startCycle
		result.Frames = m.PPU.FrameCount() - startFrame
		if err != nil {
			result.Detail = fmt.Sprintf("emulation error: %v", err)
			return result
		}
		if matched, detail, err := anyMatch(m, tc.FailAny); err != nil {
			result.Detail = err.Error()
			return result
		} else if matched {
			result.Detail = "failure condition matched: " + detail
			return result
		}
		if matched, err := allMatch(m, tc.PassAll); err != nil {
			result.Detail = err.Error()
			return result
		} else if matched {
			result.Status = StatusPass
			return result
		}

		if limitReached(tc.Limits, result) {
			result.Status = StatusTimeout
			result.Detail = "execution budget exhausted before a terminal condition matched"
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
	if tc.Limits.Steps == 0 && tc.Limits.Cycles == 0 && tc.Limits.Frames == 0 {
		return fmt.Errorf("conformance: %s must declare a step, cycle, or frame limit", tc.Name)
	}
	if len(tc.PassAll) == 0 {
		return fmt.Errorf("conformance: %s must declare at least one pass condition", tc.Name)
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
			got = uint32(m.Bus.Peek8(c.Address)) |
				uint32(m.Bus.Peek8(c.Address+1))<<8 |
				uint32(m.Bus.Peek8(c.Address+2))<<16 |
				uint32(m.Bus.Peek8(c.Address+3))<<24
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
	default:
		return c.Type
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

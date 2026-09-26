// Package compatibility provides deterministic staged GBA ROM smoke runs.
// It measures how far software gets without treating compatibility as a
// hardware-accuracy score.
package compatibility

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/conformance"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/system"
)

// Stage is the highest compatibility milestone reached by one ROM run.
type Stage string

const (
	StageNone       Stage = ""
	StageLoaded     Stage = "loaded"
	StageExecuted   Stage = "executed"
	StageFirstFrame Stage = "first_frame"
	StageCheckpoint Stage = "checkpoint"
	StageStable     Stage = "bounded_stability"
)

// Status is the terminal result of a compatibility run.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
)

// RegisterValue initializes one architectural register for BIOS-less direct
// boot. This is useful for homebrew/test ROMs that assume post-BIOS state.
type RegisterValue struct {
	Register int
	Value    uint32
}

// Checkpoint is one deterministic state predicate used for a compatibility
// milestone. A checkpoint is intentionally emulator-visible rather than
// game-specific core logic.
type Checkpoint struct {
	Type     string `json:"type"`
	Address  uint32 `json:"address,omitempty"`
	Width    uint8  `json:"width,omitempty"`
	Register int    `json:"register,omitempty"`
	Value    uint32 `json:"value"`
	Mask     uint32 `json:"mask,omitempty"`
}

// Case is one staged ROM compatibility run.
type Case struct {
	Name             string
	ROM              []byte
	ROMSHA256        string
	BIOS             []byte
	Boot             conformance.BootMode
	EntryPoint       uint32
	InitialCPSR      *uint32
	InitialRegisters []RegisterValue
	Limits           conformance.Limits
	TargetStage      Stage
	Checkpoint       *Checkpoint
}

// Result is the stable machine-readable compatibility result for one ROM.
type Result struct {
	Suite         string `json:"suite"`
	Test          string `json:"test"`
	Status        Status `json:"status"`
	HighestStage  Stage  `json:"highest_stage"`
	TargetStage   Stage  `json:"target_stage"`
	Cycles        uint64 `json:"cycles"`
	Frames        uint64 `json:"frames"`
	Steps         uint64 `json:"steps"`
	Detail        string `json:"detail,omitempty"`
	GomeBoyCommit string `json:"gomeboy_commit"`
	SuiteRevision string `json:"suite_revision"`
	ROMSHA256     string `json:"rom_sha256"`
}

// Summary contains compatibility run outcome counts.
type Summary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	TimedOut int `json:"timed_out"`
	Total    int `json:"total"`
}

// Report is the deterministic result document for one compatibility corpus.
type Report struct {
	SchemaVersion int      `json:"schema_version"`
	Suite         string   `json:"suite"`
	SuiteRevision string   `json:"suite_revision"`
	GomeBoyCommit string   `json:"gomeboy_commit"`
	Summary       Summary  `json:"summary"`
	Results       []Result `json:"results"`
}

// Runner supplies metadata shared by a compatibility corpus.
type Runner struct {
	Suite         string
	SuiteRevision string
	GomeBoyCommit string
}

// Run executes one ROM until it reaches its requested milestone, encounters an
// emulation failure, or exhausts its deterministic budget.
//
// Checkpoint credit is awarded only after the first rendered frame so the
// ordered stages retain their meaning. Bounded stability is awarded when the
// declared budget is exhausted without an emulation error.
func (r Runner) Run(tc Case) Result {
	result := Result{
		Suite:         r.Suite,
		Test:          tc.Name,
		Status:        StatusFail,
		HighestStage:  StageNone,
		TargetStage:   tc.TargetStage,
		GomeBoyCommit: r.GomeBoyCommit,
		SuiteRevision: r.SuiteRevision,
		ROMSHA256:     sha256Hex(tc.ROM),
	}

	if err := validateCase(tc); err != nil {
		result.Detail = err.Error()
		return result
	}
	if tc.ROMSHA256 != result.ROMSHA256 {
		result.Detail = fmt.Sprintf("ROM SHA-256 mismatch: got %s, want %s", result.ROMSHA256, tc.ROMSHA256)
		return result
	}

	m := system.New(tc.BIOS, tc.ROM)
	m.Audio.SetHeadless(true)
	if err := configureBoot(m, tc); err != nil {
		result.Detail = err.Error()
		return result
	}
	result.HighestStage = StageLoaded
	if reached(result.HighestStage, tc.TargetStage) {
		result.Status = StatusPass
		return result
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

		if stageRank(result.HighestStage) < stageRank(StageExecuted) {
			result.HighestStage = StageExecuted
		}
		if result.Frames > 0 && stageRank(result.HighestStage) < stageRank(StageFirstFrame) {
			result.HighestStage = StageFirstFrame
		}
		if result.Frames > 0 && tc.Checkpoint != nil {
			matched, err := tc.Checkpoint.matches(m)
			if err != nil {
				result.Detail = err.Error()
				return result
			}
			if matched && stageRank(result.HighestStage) < stageRank(StageCheckpoint) {
				result.HighestStage = StageCheckpoint
			}
		}

		if tc.TargetStage != StageStable && reached(result.HighestStage, tc.TargetStage) {
			result.Status = StatusPass
			return result
		}

		if limitReached(tc.Limits, result) {
			if tc.TargetStage == StageStable {
				result.HighestStage = StageStable
				result.Status = StatusPass
				return result
			}
			result.Status = StatusTimeout
			result.Detail = fmt.Sprintf("execution budget exhausted at %s before target stage %s", result.HighestStage, tc.TargetStage)
			return result
		}
	}
}

// RunAll executes cases in manifest order.
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

// Passed reports whether every ROM reached its declared target stage.
func (r Report) Passed() bool {
	return r.Summary.Total > 0 && r.Summary.Passed == r.Summary.Total
}

func configureBoot(m *system.Machine, tc Case) error {
	if tc.Boot != conformance.BootDirect {
		return nil
	}

	// Match the BIOS-less desktop frontend's useful post-BIOS stack layout.
	// Compatibility runs should exercise the same startup state as a ROM launched
	// through GomeBoy rather than an artificially bare reset CPU.
	if err := m.CPU.SetMode(cpu.ModeIRQ); err != nil {
		return fmt.Errorf("compatibility: %s enter IRQ mode: %w", tc.Name, err)
	}
	m.CPU.WriteRegister(13, 0x03007fa0)
	if err := m.CPU.SetMode(cpu.ModeSupervisor); err != nil {
		return fmt.Errorf("compatibility: %s enter supervisor mode: %w", tc.Name, err)
	}
	m.CPU.WriteRegister(13, 0x03007fe0)
	if err := m.CPU.SetMode(cpu.ModeSystem); err != nil {
		return fmt.Errorf("compatibility: %s enter system mode: %w", tc.Name, err)
	}
	m.CPU.WriteRegister(13, 0x03007f00)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		return fmt.Errorf("compatibility: %s initialize CPSR: %w", tc.Name, err)
	}

	// Manifests can override the default post-BIOS state when a fixture needs a
	// more specific starting environment.
	if tc.InitialCPSR != nil {
		if err := m.CPU.SetCPSR(cpu.PSR(*tc.InitialCPSR)); err != nil {
			return fmt.Errorf("compatibility: %s initial CPSR: %w", tc.Name, err)
		}
	}
	for _, register := range tc.InitialRegisters {
		if register.Register < 0 || register.Register > 14 {
			return fmt.Errorf("compatibility: %s initial register must be r0-r14, got r%d", tc.Name, register.Register)
		}
		m.CPU.WriteRegister(register.Register, register.Value)
	}
	entry := tc.EntryPoint
	if entry == 0 {
		entry = bus.ROM0Start
	}
	m.CPU.SetPC(entry)
	return nil
}

func validateCase(tc Case) error {
	if tc.Name == "" {
		return fmt.Errorf("compatibility: test name is required")
	}
	if len(tc.ROM) == 0 {
		return fmt.Errorf("compatibility: %s has an empty ROM", tc.Name)
	}
	if tc.ROMSHA256 == "" {
		return fmt.Errorf("compatibility: %s must pin rom_sha256", tc.Name)
	}
	if tc.Boot != conformance.BootDirect && tc.Boot != conformance.BootReset {
		return fmt.Errorf("compatibility: %s has invalid boot mode %q", tc.Name, tc.Boot)
	}
	if tc.Limits.Steps == 0 {
		return fmt.Errorf("compatibility: %s must declare a step limit so STOP/stall states remain bounded", tc.Name)
	}
	if stageRank(tc.TargetStage) == 0 {
		return fmt.Errorf("compatibility: %s has invalid target stage %q", tc.Name, tc.TargetStage)
	}
	if tc.TargetStage == StageCheckpoint && tc.Checkpoint == nil {
		return fmt.Errorf("compatibility: %s targets checkpoint but declares no checkpoint", tc.Name)
	}
	return nil
}

func stageRank(stage Stage) int {
	switch stage {
	case StageLoaded:
		return 1
	case StageExecuted:
		return 2
	case StageFirstFrame:
		return 3
	case StageCheckpoint:
		return 4
	case StageStable:
		return 5
	default:
		return 0
	}
}

func reached(got, want Stage) bool {
	return stageRank(got) >= stageRank(want)
}

func limitReached(l conformance.Limits, r Result) bool {
	return (l.Steps != 0 && r.Steps >= l.Steps) ||
		(l.Cycles != 0 && r.Cycles >= l.Cycles) ||
		(l.Frames != 0 && r.Frames >= l.Frames)
}

func (c Checkpoint) matches(m *system.Machine) (bool, error) {
	var got uint32
	switch c.Type {
	case "memory":
		switch c.Width {
		case 1:
			got = uint32(m.Bus.Peek8(c.Address))
		case 2:
			got = uint32(m.Bus.Peek8(c.Address)) |
				uint32(m.Bus.Peek8(c.Address+1))<<8
		case 4:
			got = uint32(m.Bus.Peek8(c.Address)) |
				uint32(m.Bus.Peek8(c.Address+1))<<8 |
				uint32(m.Bus.Peek8(c.Address+2))<<16 |
				uint32(m.Bus.Peek8(c.Address+3))<<24
		default:
			return false, fmt.Errorf("compatibility: checkpoint memory width must be 1, 2, or 4, got %d", c.Width)
		}
	case "register":
		if c.Register < 0 || c.Register > 14 {
			return false, fmt.Errorf("compatibility: checkpoint register must be r0-r14, got r%d", c.Register)
		}
		got = m.CPU.ReadRegister(c.Register)
	case "pc":
		got = m.CPU.PC()
	default:
		return false, fmt.Errorf("compatibility: unknown checkpoint type %q", c.Type)
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

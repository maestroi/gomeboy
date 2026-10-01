// Package compatibility provides deterministic staged GB/GBC ROM smoke runs.
// Compatibility is reported separately from hardware conformance: a ROM reaching
// a stage says that an end-to-end user-facing path works, not that the hardware
// implementation is cycle accurate.
package compatibility

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Stage is the highest compatibility milestone reached by one ROM run.
type Stage string

const (
	StageNone        Stage = ""
	StageLoaded      Stage = "loaded"
	StageRendered    Stage = "rendered_output"
	StageCheckpoint  Stage = "checkpoint"
	StageInput       Stage = "input_progress"
	StagePersistence Stage = "battery_persistence"
	StageStable      Stage = "bounded_stability"
)

// Status is the terminal result of a compatibility run.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
)

// InputEvent is one deterministic joypad transition at a rendered-frame
// boundary relative to the beginning of the run.
type InputEvent struct {
	Frame   uint64
	Button  gomeboy.Button
	Pressed bool
}

// Checkpoint is an emulator-visible predicate used by a compatibility stage.
type Checkpoint struct {
	Address uint16
	Value   []byte
}

// Persistence declares the checkpoint before and after a battery-save
// close/reopen round trip.
type Persistence struct {
	Before Checkpoint
	After  Checkpoint
}

// Case is one staged GB/GBC compatibility run.
type Case struct {
	Name           string
	ROM            []byte
	ROMName        string
	ROMSHA256      string
	Source         string
	SourceRevision string
	Model          gomeboy.Model
	MaxFrames      uint64
	TargetStage    Stage
	Checkpoint     *Checkpoint
	Inputs         []InputEvent
	Persistence    *Persistence
}

// Result is one stable machine-readable compatibility result.
type Result struct {
	Suite           string        `json:"suite"`
	Test            string        `json:"test"`
	Status          Status        `json:"status"`
	HighestStage    Stage         `json:"highest_stage"`
	TargetStage     Stage         `json:"target_stage"`
	Model           gomeboy.Model `json:"model"`
	Frames          uint64        `json:"frames"`
	Cycles          uint64        `json:"cycles"`
	Detail          string        `json:"detail,omitempty"`
	GomeBoyCommit   string        `json:"gomeboy_commit"`
	SuiteRevision   string        `json:"suite_revision"`
	ROMSHA256       string        `json:"rom_sha256"`
	Source          string        `json:"source,omitempty"`
	SourceRevision  string        `json:"source_revision,omitempty"`
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

// Run executes one compatibility case.
func (r Runner) Run(tc Case) Result {
	result := Result{
		Suite:          r.Suite,
		Test:           tc.Name,
		Status:         StatusFail,
		TargetStage:    tc.TargetStage,
		Model:          tc.Model,
		GomeBoyCommit:  r.GomeBoyCommit,
		SuiteRevision:  r.SuiteRevision,
		ROMSHA256:      sha256Hex(tc.ROM),
		Source:         tc.Source,
		SourceRevision: tc.SourceRevision,
	}

	if err := validateCase(tc); err != nil {
		result.Detail = err.Error()
		return result
	}
	if result.ROMSHA256 != strings.ToLower(tc.ROMSHA256) {
		result.Detail = fmt.Sprintf("ROM SHA-256 mismatch: got %s, want %s", result.ROMSHA256, tc.ROMSHA256)
		return result
	}

	if tc.Persistence != nil {
		return r.runPersistence(tc, result)
	}

	emu, err := newEmulator(tc, "")
	if err != nil {
		result.Detail = fmt.Sprintf("load ROM: %v", err)
		return result
	}
	defer emu.Close()

	result.HighestStage = StageLoaded
	if tc.TargetStage == StageLoaded {
		result.Status = StatusPass
		return result
	}

	nextInput := 0
	checkpointMatched := tc.Checkpoint == nil
	for result.Frames < tc.MaxFrames {
		for nextInput < len(tc.Inputs) && tc.Inputs[nextInput].Frame <= result.Frames {
			event := tc.Inputs[nextInput]
			if event.Pressed {
				emu.Press(event.Button)
			} else {
				emu.Release(event.Button)
			}
			nextInput++
		}

		emu.StepFrame()
		result.Frames++
		result.Cycles = emu.Cycle()

		if frameRendered(emu.Frame()) && stageRank(result.HighestStage) < stageRank(StageRendered) {
			result.HighestStage = StageRendered
		}
		if tc.Checkpoint != nil && tc.Checkpoint.matches(emu) {
			checkpointMatched = true
			if stageRank(result.HighestStage) < stageRank(StageCheckpoint) {
				result.HighestStage = StageCheckpoint
			}
		}
		if len(tc.Inputs) > 0 && nextInput == len(tc.Inputs) && checkpointMatched &&
			stageRank(result.HighestStage) < stageRank(StageInput) {
			result.HighestStage = StageInput
		}

		if tc.TargetStage != StageStable && reached(result.HighestStage, tc.TargetStage) {
			result.Status = StatusPass
			return result
		}
	}

	if tc.TargetStage == StageStable {
		if !reached(result.HighestStage, StageRendered) {
			result.Status = StatusTimeout
			result.Detail = "frame budget exhausted before non-uniform rendered output"
			return result
		}
		if !checkpointMatched {
			result.Status = StatusTimeout
			result.Detail = "frame budget exhausted before required checkpoint"
			return result
		}
		if len(tc.Inputs) > 0 && nextInput != len(tc.Inputs) {
			result.Status = StatusTimeout
			result.Detail = "frame budget exhausted before all input events were delivered"
			return result
		}
		result.HighestStage = StageStable
		result.Status = StatusPass
		return result
	}

	result.Status = StatusTimeout
	result.Detail = fmt.Sprintf("frame budget exhausted at %s before target stage %s", result.HighestStage, tc.TargetStage)
	return result
}

func (r Runner) runPersistence(tc Case, result Result) Result {
	dir, err := os.MkdirTemp("", "gomeboy-gb-compat-*")
	if err != nil {
		result.Detail = fmt.Sprintf("create save directory: %v", err)
		return result
	}
	defer os.RemoveAll(dir)

	first, err := newEmulator(tc, dir)
	if err != nil {
		result.Detail = fmt.Sprintf("load first persistence run: %v", err)
		return result
	}
	result.HighestStage = StageLoaded

	beforeMatched := false
	for result.Frames < tc.MaxFrames {
		first.StepFrame()
		result.Frames++
		result.Cycles = first.Cycle()
		if tc.Persistence.Before.matches(first) {
			beforeMatched = true
			break
		}
	}
	if !beforeMatched {
		_ = first.Close()
		result.Status = StatusTimeout
		result.Detail = "first persistence run did not reach pre-save checkpoint"
		return result
	}
	if err := first.Close(); err != nil {
		result.Detail = fmt.Sprintf("flush battery save: %v", err)
		return result
	}

	second, err := newEmulator(tc, dir)
	if err != nil {
		result.Detail = fmt.Sprintf("load persisted ROM: %v", err)
		return result
	}
	defer second.Close()

	startCycle := second.Cycle()
	for frames := uint64(0); frames < tc.MaxFrames; frames++ {
		second.StepFrame()
		result.Frames++
		result.Cycles += second.Cycle() - startCycle
		startCycle = second.Cycle()
		if tc.Persistence.After.matches(second) {
			result.HighestStage = StagePersistence
			result.Status = StatusPass
			return result
		}
	}

	result.Status = StatusTimeout
	result.Detail = "reopened ROM did not observe persisted battery data"
	return result
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

func newEmulator(tc Case, saveDir string) (*gomeboy.Emulator, error) {
	opts := []gomeboy.Option{gomeboy.Headless(), gomeboy.WithModel(tc.Model)}
	if saveDir != "" {
		opts = append(opts, gomeboy.WithSaveDir(saveDir))
	}
	emu, err := gomeboy.New(opts...)
	if err != nil {
		return nil, err
	}
	if err := emu.LoadROMBytes(tc.ROM, tc.ROMName); err != nil {
		_ = emu.Close()
		return nil, err
	}
	return emu, nil
}

func validateCase(tc Case) error {
	if tc.Name == "" {
		return fmt.Errorf("compatibility: test name is required")
	}
	if len(tc.ROM) == 0 {
		return fmt.Errorf("compatibility: %s has an empty ROM", tc.Name)
	}
	if tc.ROMName == "" {
		return fmt.Errorf("compatibility: %s must declare rom_name", tc.Name)
	}
	if len(tc.ROMSHA256) != 64 {
		return fmt.Errorf("compatibility: %s must pin rom_sha256", tc.Name)
	}
	if _, err := hex.DecodeString(tc.ROMSHA256); err != nil {
		return fmt.Errorf("compatibility: %s has invalid rom_sha256: %w", tc.Name, err)
	}
	if tc.Model == gomeboy.ModelAuto {
		return fmt.Errorf("compatibility: %s must select an explicit model", tc.Name)
	}
	if tc.MaxFrames == 0 {
		return fmt.Errorf("compatibility: %s must declare max_frames", tc.Name)
	}
	if stageRank(tc.TargetStage) == 0 {
		return fmt.Errorf("compatibility: %s has invalid target stage %q", tc.Name, tc.TargetStage)
	}
	if tc.TargetStage == StageInput && (len(tc.Inputs) == 0 || tc.Checkpoint == nil) {
		return fmt.Errorf("compatibility: %s input_progress requires inputs and a checkpoint", tc.Name)
	}
	if tc.TargetStage == StagePersistence && tc.Persistence == nil {
		return fmt.Errorf("compatibility: %s battery_persistence requires persistence checkpoints", tc.Name)
	}
	if tc.Persistence != nil && tc.TargetStage != StagePersistence {
		return fmt.Errorf("compatibility: %s persistence checkpoints require battery_persistence target", tc.Name)
	}
	var previous uint64
	for i, event := range tc.Inputs {
		if i > 0 && event.Frame < previous {
			return fmt.Errorf("compatibility: %s inputs must be ordered by frame", tc.Name)
		}
		previous = event.Frame
	}
	return nil
}

func (c Checkpoint) matches(emu *gomeboy.Emulator) bool {
	if len(c.Value) == 0 {
		return false
	}
	for i, want := range c.Value {
		if emu.Peek8(c.Address+uint16(i)) != want {
			return false
		}
	}
	return true
}

func frameRendered(frame gomeboy.Frame) bool {
	if frame.Width <= 0 || frame.Height <= 0 || len(frame.RGB) < 6 {
		return false
	}
	first := frame.RGB[:3]
	for i := 3; i+2 < len(frame.RGB); i += 3 {
		if frame.RGB[i] != first[0] || frame.RGB[i+1] != first[1] || frame.RGB[i+2] != first[2] {
			return true
		}
	}
	return false
}

func stageRank(stage Stage) int {
	switch stage {
	case StageLoaded:
		return 1
	case StageRendered:
		return 2
	case StageCheckpoint:
		return 3
	case StageInput:
		return 4
	case StagePersistence:
		return 5
	case StageStable:
		return 6
	default:
		return 0
	}
}

func reached(got, want Stage) bool { return stageRank(got) >= stageRank(want) }

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

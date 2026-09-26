package conformance

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestRunnerPassesOnMemorySignature(t *testing.T) {
	const signature uint32 = 0x47424d47
	rom := armROM(
		0xe59f0010, // ldr r0, [pc, #0x10] -> EWRAMStart literal
		0xe59f1010, // ldr r1, [pc, #0x10] -> signature literal
		0xe5801000, // str r1, [r0]
		0xeafffffe, // b .
		0,
		0,
		bus.EWRAMStart,
		signature,
	)

	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:   "memory-signature",
		ROM:    rom,
		Boot:   BootDirect,
		Limits: Limits{Steps: 8},
		PassAll: []Condition{{
			Type:    "memory",
			Address: bus.EWRAMStart,
			Width:   4,
			Value:   signature,
		}},
	})

	if result.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", result.Status, result.Detail)
	}
	if result.Steps != 3 {
		t.Fatalf("steps = %d, want 3", result.Steps)
	}
	if result.Cycles == 0 {
		t.Fatal("passing run reported zero cycles")
	}
}

func TestRunnerRecognizesSelfLoopBeforeExecutingIt(t *testing.T) {
	progress := &Progress{
		FailureRegister: 0,
		Groups: []FailureGroup{
			{Name: "conditions", First: 1, Last: 20},
			{Name: "branches", First: 50, Last: 57},
		},
	}
	runner := Runner{Suite: "upstream", SuiteRevision: "rev", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:     "self-loop",
		ROM:      armROM(0xeafffffe),
		Boot:     BootDirect,
		Limits:   Limits{Steps: 1},
		PassAll:  []Condition{{Type: "self_loop"}},
		Progress: progress,
	})
	if result.Status != StatusPass || result.Steps != 0 {
		t.Fatalf("result = %+v, want zero-step pass", result)
	}
	if result.Checks == nil || *result.Checks != (CheckSummary{Passed: 28, Total: 28}) {
		t.Fatalf("checks = %+v, want 28/28", result.Checks)
	}
}

func TestRunnerCapturesSWIFirstFailureProgress(t *testing.T) {
	progress := &Progress{
		FailureRegister: 0,
		Groups: []FailureGroup{
			{Name: "conditions", First: 1, Last: 20},
			{Name: "branches", First: 50, Last: 57},
		},
	}
	runner := Runner{Suite: "upstream", SuiteRevision: "rev", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:             "swi-failure",
		ROM:              armROM(0xef060000), // swi 0x60000
		Boot:             BootDirect,
		InitialRegisters: []RegisterValue{{Register: 0, Value: 52}},
		Limits:           Limits{Steps: 1},
		PassAll:          []Condition{{Type: "self_loop"}},
		FailAny:          []Condition{{Type: "swi", Value: 0x60000}},
		Progress:         progress,
	})
	if result.Status != StatusFail || result.Steps != 0 {
		t.Fatalf("result = %+v, want zero-step failure", result)
	}
	if result.Failure == nil || result.Failure.TestID != 52 || result.Failure.Group != "branches" || result.Failure.Register != "r0" {
		t.Fatalf("failure = %+v", result.Failure)
	}
	wantChecks := CheckSummary{Passed: 22, Failed: 1, NotRun: 5, Total: 28}
	if result.Checks == nil || *result.Checks != wantChecks {
		t.Fatalf("checks = %+v, want %+v", result.Checks, wantChecks)
	}
}

func TestRunnerAppliesExplicitDirectBootCPUState(t *testing.T) {
	cpsr := uint32(0x1f)
	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:             "initial-sp",
		ROM:              armROM(0xeafffffe),
		Boot:             BootDirect,
		InitialCPSR:      &cpsr,
		InitialRegisters: []RegisterValue{{Register: 13, Value: 0x03007f00}},
		Limits:           Limits{Steps: 1},
		PassAll: []Condition{{
			Type:     "register",
			Register: 13,
			Value:    0x03007f00,
		}},
	})
	if result.Status != StatusPass || result.Steps != 0 {
		t.Fatalf("result = %+v, want explicit boot state to match before first instruction", result)
	}
}

func TestRunnerTimesOutDeterministically(t *testing.T) {
	rom := armROM(0xeafffffe) // b .
	tc := Case{
		Name:   "timeout",
		ROM:    rom,
		Boot:   BootDirect,
		Limits: Limits{Steps: 4},
		PassAll: []Condition{{
			Type:    "memory",
			Address: bus.EWRAMStart,
			Width:   4,
			Value:   1,
		}},
	}
	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}

	first := runner.RunAll([]Case{tc})
	second := runner.RunAll([]Case{tc})
	if first.Results[0].Status != StatusTimeout {
		t.Fatalf("status = %s, want timeout", first.Results[0].Status)
	}
	if first.Results[0].Steps != 4 {
		t.Fatalf("steps = %d, want 4", first.Results[0].Steps)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("same ROM/config produced different reports:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestRunnerRejectsPinnedROMHashMismatch(t *testing.T) {
	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:      "hash",
		ROM:       armROM(0xeafffffe),
		ROMSHA256: "not-the-rom",
		Boot:      BootDirect,
		Limits:    Limits{Steps: 1},
		PassAll: []Condition{{
			Type:  "pc",
			Value: bus.ROM0Start,
		}},
	})
	if result.Status != StatusFail {
		t.Fatalf("status = %s, want fail", result.Status)
	}
	if result.Detail == "" {
		t.Fatal("hash mismatch did not include failure detail")
	}
}

func TestUint32AcceptsQuotedHex(t *testing.T) {
	var value Uint32
	if err := json.Unmarshal([]byte(`"0x08000000"`), &value); err != nil {
		t.Fatal(err)
	}
	if uint32(value) != bus.ROM0Start {
		t.Fatalf("value = %#x, want %#x", uint32(value), bus.ROM0Start)
	}
}

func armROM(words ...uint32) []byte {
	rom := make([]byte, len(words)*4)
	for i, word := range words {
		binary.LittleEndian.PutUint32(rom[i*4:], word)
	}
	return rom
}

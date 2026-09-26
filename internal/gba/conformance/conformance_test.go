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

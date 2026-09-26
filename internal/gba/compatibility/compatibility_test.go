package compatibility

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/conformance"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/system"
)


func TestConfigureBootMatchesDesktopPostBIOSState(t *testing.T) {
	m := system.New(nil, armROM(0xeafffffe))
	tc := Case{Name: "desktop-state", Boot: conformance.BootDirect}
	if err := configureBoot(m, tc); err != nil {
		t.Fatal(err)
	}
	if got := m.CPU.CPSR().Mode(); got != cpu.ModeSystem {
		t.Fatalf("mode = %#x, want system", got)
	}
	if got := m.CPU.PC(); got != bus.ROM0Start {
		t.Fatalf("pc = %#x, want %#x", got, bus.ROM0Start)
	}
	if got := m.CPU.ReadRegister(13); got != 0x03007f00 {
		t.Fatalf("system sp = %#x, want 0x03007f00", got)
	}

	for _, want := range []struct {
		mode cpu.Mode
		sp   uint32
	}{
		{cpu.ModeIRQ, 0x03007fa0},
		{cpu.ModeSupervisor, 0x03007fe0},
	} {
		if err := m.CPU.SetMode(want.mode); err != nil {
			t.Fatal(err)
		}
		if got := m.CPU.ReadRegister(13); got != want.sp {
			t.Fatalf("%v sp = %#x, want %#x", want.mode, got, want.sp)
		}
	}
}

func TestRunnerReachesRenderedCheckpoint(t *testing.T) {
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
		Name:        "rendered-signature",
		ROM:         rom,
		ROMSHA256:   sha256Hex(rom),
		Boot:        conformance.BootDirect,
		Limits:      conformance.Limits{Steps: 200000, Cycles: 400000, Frames: 2},
		TargetStage: StageCheckpoint,
		Checkpoint: &Checkpoint{
			Type:    "memory",
			Address: bus.EWRAMStart,
			Width:   4,
			Value:   signature,
		},
	})

	if result.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", result.Status, result.Detail)
	}
	if result.HighestStage != StageCheckpoint {
		t.Fatalf("highest stage = %s, want %s", result.HighestStage, StageCheckpoint)
	}
	if result.Frames == 0 {
		t.Fatal("checkpoint was credited before a rendered frame")
	}
	if result.Steps <= 3 {
		t.Fatalf("steps = %d, want runner to continue beyond early memory checkpoint to first frame", result.Steps)
	}
}

func TestRunnerTimesOutBeforeFirstFrame(t *testing.T) {
	rom := armROM(0xeafffffe)
	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:        "no-frame-yet",
		ROM:         rom,
		ROMSHA256:   sha256Hex(rom),
		Boot:        conformance.BootDirect,
		Limits:      conformance.Limits{Steps: 1},
		TargetStage: StageFirstFrame,
	})

	if result.Status != StatusTimeout {
		t.Fatalf("status = %s, want timeout", result.Status)
	}
	if result.HighestStage != StageExecuted {
		t.Fatalf("highest stage = %s, want %s", result.HighestStage, StageExecuted)
	}
}

func TestRunnerAwardsBoundedStabilityAtBudget(t *testing.T) {
	rom := armROM(0xeafffffe)
	runner := Runner{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	result := runner.Run(Case{
		Name:        "stable-loop",
		ROM:         rom,
		ROMSHA256:   sha256Hex(rom),
		Boot:        conformance.BootDirect,
		Limits:      conformance.Limits{Steps: 4},
		TargetStage: StageStable,
	})

	if result.Status != StatusPass || result.HighestStage != StageStable {
		t.Fatalf("result = %+v, want bounded stability pass", result)
	}
	if result.Steps != 4 {
		t.Fatalf("steps = %d, want 4", result.Steps)
	}
}

func TestRunnerRejectsUnpinnedROM(t *testing.T) {
	rom := armROM(0xeafffffe)
	result := (Runner{}).Run(Case{
		Name:        "unpinned",
		ROM:         rom,
		Boot:        conformance.BootDirect,
		Limits:      conformance.Limits{Steps: 1},
		TargetStage: StageExecuted,
	})
	if result.Status != StatusFail || !strings.Contains(result.Detail, "rom_sha256") {
		t.Fatalf("result = %+v, want pinning failure", result)
	}
}

func TestMarkdownReportIncludesStagesAndAccuracyWarning(t *testing.T) {
	report := Report{
		SchemaVersion: 1,
		Suite:         "smoke",
		SuiteRevision: "1",
		GomeBoyCommit: "abc",
		Summary:       Summary{Passed: 1, Total: 1},
		Results: []Result{{
			Test:         "demo|rom",
			Status:       StatusPass,
			HighestStage: StageFirstFrame,
			TargetStage:  StageFirstFrame,
			Frames:       1,
			Steps:        42,
			ROMSHA256:    "deadbeef",
		}},
	}
	got := report.Markdown()
	for _, want := range []string{"demo\\|rom", "first_frame", "not hardware-accuracy percentages"} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown missing %q:\n%s", want, got)
		}
	}
}

func armROM(words ...uint32) []byte {
	rom := make([]byte, len(words)*4)
	for i, word := range words {
		binary.LittleEndian.PutUint32(rom[i*4:], word)
	}
	return rom
}

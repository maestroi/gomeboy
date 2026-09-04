package memprobe

import (
	"testing"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

func probeTestROM() []byte {
	rom := make([]byte, 32*1024)
	// JP 0x0100 forever. The ROM itself does not mutate memory, which makes
	// input-triggered interrupt state easy to observe in focused tests.
	rom[0x0100] = 0xC3
	rom[0x0101] = 0x00
	rom[0x0102] = 0x01
	copy(rom[0x0134:0x0144], []byte("MEMPROBE"))
	rom[0x0147] = 0x00 // ROM only
	rom[0x0148] = 0x00 // 32 KiB
	rom[0x0149] = 0x00 // no RAM
	return rom
}

func newProbeTestEmulator(t *testing.T) *gomeboy.Emulator {
	t.Helper()
	e, err := gomeboy.New(
		gomeboy.WithROMBytes(probeTestROM()),
		gomeboy.Headless(),
		gomeboy.WithoutVideo(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestRunUsesCommonBaselineAndRestoresCoreState(t *testing.T) {
	e := newProbeTestEmulator(t)
	beforeIF := e.Peek8(0xFF0F)
	beforeFrame := e.FrameCount()
	beforeCycle := e.Cycle()

	results, err := Run(e,
		[]Region{{Name: "interrupt_flags", Start: 0xFF0F, Length: 1}},
		[]Action{
			Wait("control", 0),
			{
				Name: "press-a",
				Phases: []Phase{{
					Transitions: []InputTransition{{Button: gomeboy.ButtonA, Pressed: true}},
				}},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if len(results[0].Changes) != 0 {
		t.Fatalf("control changes = %+v, want none", results[0].Changes)
	}

	press := results[1]
	if len(press.Changes) != 1 {
		t.Fatalf("press-a changes = %+v, want one IF change", press.Changes)
	}
	change := press.Changes[0]
	if change.Region != "interrupt_flags" || change.Address != 0xFF0F {
		t.Fatalf("unexpected change location: %+v", change)
	}
	if change.Before != beforeIF || change.After != beforeIF|0x10 {
		t.Fatalf("IF change = %02X -> %02X, want %02X -> %02X", change.Before, change.After, beforeIF, beforeIF|0x10)
	}
	if change.Delta != int16(change.After)-int16(change.Before) {
		t.Fatalf("delta = %d, want %d", change.Delta, int16(change.After)-int16(change.Before))
	}

	if got := e.Peek8(0xFF0F); got != beforeIF {
		t.Fatalf("Run left IF changed: got %02X, want baseline %02X", got, beforeIF)
	}
	if got := e.FrameCount(); got != beforeFrame {
		t.Fatalf("Run left frame = %d, want baseline %d", got, beforeFrame)
	}
	if got := e.Cycle(); got != beforeCycle {
		t.Fatalf("Run left cycle = %d, want baseline %d", got, beforeCycle)
	}
}

func TestRunRepeatsTimeDrivenExperimentFromIdenticalState(t *testing.T) {
	e := newProbeTestEmulator(t)
	results, err := Run(e,
		[]Region{{Name: "io", Start: 0xFF04, Length: 0x0C}},
		[]Action{Wait("first", 1), Wait("second", 1)},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Frames != 1 || results[1].Frames != 1 {
		t.Fatalf("frame deltas = %d, %d; want 1, 1", results[0].Frames, results[1].Frames)
	}
	if results[0].Cycles != results[1].Cycles {
		t.Fatalf("cycle deltas differ from same baseline: %d vs %d", results[0].Cycles, results[1].Cycles)
	}
	if len(results[0].Changes) != len(results[1].Changes) {
		t.Fatalf("change counts differ from same baseline: %d vs %d", len(results[0].Changes), len(results[1].Changes))
	}
	for i := range results[0].Changes {
		if results[0].Changes[i] != results[1].Changes[i] {
			t.Fatalf("change %d differs from identical experiment: %+v vs %+v", i, results[0].Changes[i], results[1].Changes[i])
		}
	}
}

func TestRunRejectsWrappingRegion(t *testing.T) {
	e := newProbeTestEmulator(t)
	_, err := Run(e,
		[]Region{{Name: "wrap", Start: 0xFFFF, Length: 2}},
		[]Action{Wait("control", 0)},
	)
	if err == nil {
		t.Fatal("Run accepted a region that wraps past 0xFFFF")
	}
}

func TestRunRejectsInvalidButton(t *testing.T) {
	e := newProbeTestEmulator(t)
	_, err := Run(e,
		[]Region{{Name: "wram", Start: 0xC000, Length: 1}},
		[]Action{{
			Name: "invalid",
			Phases: []Phase{{
				Transitions: []InputTransition{{Button: gomeboy.Button(0xFF), Pressed: true}},
			}},
		}},
	)
	if err == nil {
		t.Fatal("Run accepted an invalid button")
	}
}

func TestTapBuildsPressHoldReleaseSettle(t *testing.T) {
	action := Tap("right", gomeboy.ButtonRight, 2, 7)
	if action.Name != "right" || len(action.Phases) != 2 {
		t.Fatalf("Tap = %+v", action)
	}
	if len(action.Phases[0].Transitions) != 1 || !action.Phases[0].Transitions[0].Pressed || action.Phases[0].Frames != 2 {
		t.Fatalf("press phase = %+v", action.Phases[0])
	}
	if len(action.Phases[1].Transitions) != 1 || action.Phases[1].Transitions[0].Pressed || action.Phases[1].Frames != 7 {
		t.Fatalf("release phase = %+v", action.Phases[1])
	}
}

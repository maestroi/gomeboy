package apu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

func powerCycleNoiseTestAPU(t *testing.T) *APU {
	t.Helper()
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	a.Write(types.NR52, 0x00)
	a.Write(types.NR52, 0x80)
	if a.enableTimer != s.Cycle() {
		t.Fatalf("APU enable phase = %d, scheduler = %d", a.enableTimer, s.Cycle())
	}
	return a
}

func setNoiseNR43(a *APU, value byte) {
	a.Write(types.NR43, value)
}

func TestNoiseEquivalentEncodingsShareFullPeriod(t *testing.T) {
	a := powerCycleNoiseTestAPU(t)

	tests := []struct {
		nr43 byte
		inc  uint64
	}{
		{0x0c, 32}, // divisor 4, shift 0
		{0x1a, 16}, // divisor 2, shift 1
		{0x29, 8},  // divisor 1, shift 2
		{0x38, 4},  // divisor 0, shift 3
	}

	for _, tc := range tests {
		setNoiseNR43(a, tc.nr43)
		if got := a.noiseDivisorIncrement(); got != tc.inc {
			t.Fatalf("NR43=%02x increment=%d, want %d", tc.nr43, got, tc.inc)
		}
		if got := a.noisePeriod(); got != 64 {
			t.Fatalf("NR43=%02x period=%d, want 64", tc.nr43, got)
		}
	}
}

func TestNoiseTriggerKeepsDivisorAlignmentDifferences(t *testing.T) {
	a := powerCycleNoiseTestAPU(t)
	a.Write(types.NR42, 0xf0)

	// These all encode the same 64-T-cycle LFSR period, but hardware does not
	// give them the same startup phase. Divisor >=2 rounds down to the 512 kHz
	// grid, divisor 1 rounds up, and divisor 0 follows the 1 MHz grid.
	tests := []struct {
		nr43    byte
		deadline uint64
	}{
		{0x0c, 36},
		{0x1a, 36},
		{0x29, 44},
		{0x38, 40},
	}

	for _, tc := range tests {
		a.Write(types.NR52, 0x00)
		a.Write(types.NR52, 0x80)
		a.Write(types.NR42, 0xf0)
		a.Write(types.NR43, tc.nr43)
		if got := a.noiseTriggerDeadline(false) - a.s.Cycle(); got != tc.deadline {
			t.Fatalf("NR43=%02x first shift in %d cycles, want %d", tc.nr43, got, tc.deadline)
		}
	}
}

func TestNoiseFrequencyWritePreservesInFlightDivisorCountdown(t *testing.T) {
	a := powerCycleNoiseTestAPU(t)
	a.Write(types.NR42, 0xf0)
	a.Write(types.NR43, 0x18) // divisor 0, shift 1
	a.Write(types.NR44, 0x80)

	// The first divisor-stage increment is 12 T-cycles away for this trigger.
	a.s.Tick(8)
	a.catchupLFSR()
	if got := a.channel4.divCountdown; got != 4 {
		t.Fatalf("countdown before NR43 change=%d, want 4", got)
	}

	// Switch to a much longer divisor. The current four-cycle countdown must
	// finish first; only the subsequent reload uses the new divisor.
	a.Write(types.NR43, 0x3c)
	if got := a.channel4.divCountdown; got != 4 {
		t.Fatalf("NR43 restarted in-flight countdown: got %d, want 4", got)
	}

	a.s.Tick(4)
	a.catchupLFSR()
	if got := a.channel4.divCountdown; got != 32 {
		// The completed increment reloads from the new divisor code (4 => 32 T-cycles).
		t.Fatalf("post-increment countdown=%d, want 32", got)
	}
}

package apu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func configurePulse1TestChannel(a *APU, freq uint16) {
	a.Write(types.NR11, 0x00) // duty 0
	a.Write(types.NR12, 0xf8) // DAC on, volume 15
	a.Write(types.NR13, byte(freq))
	a.Write(types.NR14, 0x80|byte(freq>>8))
}

func TestPulseFrequencyWriteOnReloadUsesNewPeriod(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	configurePulse1TestChannel(a, 0x7ff)

	first := s.Until(scheduler.APUChannel1)
	s.Tick(first)
	if a.channel1.lastStepAt != s.Cycle() {
		t.Fatalf("last step = %d, cycle = %d", a.channel1.lastStepAt, s.Cycle())
	}

	// The write lands exactly after the timer stepped/reloaded. Hardware lets
	// the register write win that reload, so the next deadline uses the new
	// period rather than the old four-cycle period.
	a.Write(types.NR13, 0x00)
	a.Write(types.NR14, 0x07)
	if got, want := s.Until(scheduler.APUChannel1), uint64((2048-0x700)*4); got != want {
		t.Fatalf("next pulse step in %d cycles, want %d", got, want)
	}
}

func TestPulseFrequencyWriteAfterReloadKeepsPendingBoundary(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	configurePulse1TestChannel(a, 0x7ff)
	s.Tick(s.Until(scheduler.APUChannel1))

	// One cycle later is no longer the reload race. The already-pending old
	// period remains intact and the new frequency applies after that step.
	s.Tick(1)
	before := s.Until(scheduler.APUChannel1)
	a.Write(types.NR13, 0x00)
	a.Write(types.NR14, 0x07)
	if got := s.Until(scheduler.APUChannel1); got != before {
		t.Fatalf("off-edge frequency write moved boundary: got %d, want %d", got, before)
	}
}

func TestCGBDEPulseHighFrequencyWriteBackstepsHalfTick(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBDE)
	s.ChangeSpeed(true)
	configurePulse1TestChannel(a, 0x7ff)
	s.Tick(s.Until(scheduler.APUChannel1))
	pos := a.channel1.waveDutyPosition

	// CGB D/E exposes one additional half-APU-tick window after a duty step.
	// Dropping NR14's high frequency bits there backs the phase up by one.
	s.Tick(4)
	a.Write(types.NR14, 0x00)
	if got, want := a.channel1.waveDutyPosition, (pos+7)&7; got != want {
		t.Fatalf("CGB D/E half-tick backstep position=%d, want %d", got, want)
	}
}

func TestEarlyCGBPCM12ZeroGlitchOnDutyEdge(t *testing.T) {
	a, b, s := newWaveTimingTestAPU(t, types.CGBBC)
	configurePulse1TestChannel(a, 0x7ff)

	// Duty 0 is 00000001 in the emulator's phase convention; advance to the
	// edge that replaces a zero with the high sample.
	for i := 0; i < 7; i++ {
		s.Tick(s.Until(scheduler.APUChannel1))
	}
	if a.channel1.waveDutyPosition != 7 {
		t.Fatalf("duty position=%d, want 7", a.channel1.waveDutyPosition)
	}
	if got := b.LazyRead(types.PCM12) & 0x0f; got != 0 {
		t.Fatalf("early-CGB same-cycle PCM12=%x, want glitch zero", got)
	}

	// One cycle later the real high sample is visible.
	s.Tick(1)
	if got := b.LazyRead(types.PCM12) & 0x0f; got != 0x0f {
		t.Fatalf("early-CGB PCM12 after edge=%x, want f", got)
	}
}

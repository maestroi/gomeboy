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

func setupSweepOverflowTest(a *APU) {
	a.channels[0].enabled = true
	a.channels[0].dacEnabled = true
	a.channels[0].frequency = 0x7f0
	a.channel1.frequencyShadow = 0x7f0
	a.channel1.sweepPeriod = 2
	a.channel1.sweepTimer = 6 // next 128 Hz sweep tick performs the calculation
	a.channel1.shift = 7
	a.channel1.sweepEnabled = true
	a.channel1.sweepCheckAt = ^uint64(0)
	a.channel1.sweepStopAt = ^uint64(0)
	a.channel1.sweepLoadAt = ^uint64(0)
}

func TestSweepSecondOverflowCheckIsDelayed(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	setupSweepOverflowTest(a)

	a.clockSweep()
	if got := a.channel1.frequencyShadow; got != 0x7ff {
		t.Fatalf("first sweep writeback shadow=%#x, want 0x7ff", got)
	}
	if got := a.channels[0].frequency; got != 0x7ff {
		t.Fatalf("first sweep writeback frequency=%#x, want 0x7ff", got)
	}
	if got := a.channel1.sweepCheckAt - s.Cycle(); got != 28 {
		t.Fatalf("second overflow check in %d cycles, want 28", got)
	}

	s.Tick(27)
	a.runSweepDue()
	if !a.channels[0].enabled {
		t.Fatal("channel stopped before delayed overflow check")
	}

	s.Tick(1)
	a.runSweepDue()
	if !a.channels[0].enabled {
		t.Fatal("overflow stop became visible on check cycle; want one APU tick delay")
	}
	if got := a.channel1.sweepStopAt - s.Cycle(); got != 4 {
		t.Fatalf("overflow stop in %d cycles, want 4", got)
	}

	s.Tick(4)
	a.runSweepDue()
	if a.channels[0].enabled {
		t.Fatal("channel stayed enabled after delayed sweep overflow stop")
	}
}

func TestSweepDelayedCheckRereadsNR10(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	setupSweepOverflowTest(a)
	a.clockSweep()

	// Disable the shift before the trailing check. Hardware re-reads NR10 at
	// the delayed check, so the otherwise-overflowing second calculation is
	// cancelled.
	s.Tick(24)
	a.Write(types.NR10, 0x00)
	s.Tick(12)
	a.runSweepDue()
	if !a.channels[0].enabled {
		t.Fatal("delayed sweep check ignored updated NR10")
	}
}

func TestSweepTriggerShadowLoadIsDelayed(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	a.Write(types.NR10, 0x11) // period 1, shift 1
	a.Write(types.NR11, 0x00)
	a.Write(types.NR12, 0xf8)
	a.Write(types.NR13, 0x34)
	a.Write(types.NR14, 0x82)

	if a.channel1.frequencyShadow == 0x234 {
		t.Fatal("sweep shadow loaded immediately on trigger")
	}
	loadAt := a.channel1.sweepLoadAt
	if loadAt == ^uint64(0) || loadAt <= s.Cycle() {
		t.Fatalf("invalid delayed sweep load deadline %d at cycle %d", loadAt, s.Cycle())
	}
	s.Tick(loadAt - s.Cycle())
	a.runSweepDue()
	if got := a.channel1.frequencyShadow; got != 0x234 {
		t.Fatalf("delayed sweep shadow=%#x, want 0x234", got)
	}
}

func TestPulseRetriggerCancelsPendingSweepStop(t *testing.T) {
	a, _, s := newWaveTimingTestAPU(t, types.CGBABC)
	configurePulse1TestChannel(a, 0x7ff)
	a.channel1.sweepStopAt = s.Cycle() + 4

	// A restart reloads the sweep unit and cancels a pending delayed stop.
	a.Write(types.NR14, 0x87)
	if a.channel1.sweepStopAt != ^uint64(0) {
		t.Fatalf("pending sweep stop survived retrigger: %d", a.channel1.sweepStopAt)
	}
	s.Tick(8)
	a.runSweepDue()
	if !a.channels[0].enabled {
		t.Fatal("retriggered channel stopped from stale sweep deadline")
	}
}

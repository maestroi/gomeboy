package apu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newWaveTimingTestAPU(t *testing.T, model types.Model) (*APU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()
	s := scheduler.NewScheduler()
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80 // CGB-compatible fixture so CGB-only I/O is mapped normally.
	b := gbio.NewBus(s, rom)
	a := New(b, s)
	b.Map(model)

	// New installs an initial channel-3 event at cycle zero to establish the
	// empty sample buffer. Drain it before configuring a channel in the test.
	s.Tick(0)
	return a, b, s
}

func configureWaveTestChannel(a *APU, freq uint16, firstByte byte) {
	a.waveRAM[0] = firstByte
	a.Write(types.NR30, 0x80)
	a.Write(types.NR32, 0x20) // 100% volume
	a.Write(types.NR33, byte(freq))
	a.Write(types.NR34, 0x80|byte(freq>>8))
}

func TestWaveFrequencyWriteDoesNotMoveCurrentSampleBoundary(t *testing.T) {
	a, b, s := newWaveTimingTestAPU(t, types.CGBABC)

	// 0x7ff gives a two-cycle sample period; trigger adds the already-modelled
	// six-cycle startup/phantom-sample delay, so the first fetch is at cycle 8.
	configureWaveTestChannel(a, 0x7ff, 0xde)
	if got := s.Until(scheduler.APUChannel3); got != 8 {
		t.Fatalf("first wave fetch in %d cycles, want 8", got)
	}

	// Lengthen the programmed period while that first sample is in flight.
	// Hardware applies this to the *next* sample only.
	a.Write(types.NR33, 0x00)
	a.Write(types.NR34, 0x07)

	s.Tick(7)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0 {
		t.Fatalf("PCM before original fetch boundary = %x, want 0", got)
	}
	s.Tick(1)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0x0e {
		t.Fatalf("PCM at original fetch boundary = %x, want e", got)
	}
}

func TestWaveRetriggerKeepsPreviousSampleUntilPhantomPeriodEnds(t *testing.T) {
	a, b, s := newWaveTimingTestAPU(t, types.CGBABC)
	configureWaveTestChannel(a, 0x7ff, 0xff)
	s.Tick(8)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0x0f {
		t.Fatalf("initial PCM = %x, want f", got)
	}

	// Change what the restarted channel will fetch, then retrigger. The old
	// sample stays visible during the phantom first period.
	a.waveRAM[0] = 0xde
	a.Write(types.NR34, 0x87)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0x0f {
		t.Fatalf("PCM immediately after retrigger = %x, want previous sample f", got)
	}
	if got := s.Until(scheduler.APUChannel3); got != 8 {
		t.Fatalf("retrigger fetch in %d cycles, want 8", got)
	}

	s.Tick(7)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0x0f {
		t.Fatalf("PCM before retrigger fetch = %x, want previous sample f", got)
	}
	s.Tick(1)
	if got := b.LazyRead(types.PCM34) & 0x0f; got != 0x0e {
		t.Fatalf("PCM after retrigger fetch = %x, want e", got)
	}
}

func TestWaveTriggerReloadAfterExtraLengthClockUsesFF(t *testing.T) {
	a, _, _ := newWaveTimingTestAPU(t, types.CGBBC)
	a.frameSequencerStep = 1
	a.channels[2].dacEnabled = true
	a.channels[2].lengthCounter = 1
	a.channels[2].lengthCounterEnabled = false

	// On early CGB hardware the trigger write itself may perform the extra
	// length clock even with bit 6 clear. If that expires CH3, trigger reloads
	// the 8-bit length counter to FF rather than 100.
	a.Write(types.NR34, 0x80)
	if got := a.channels[2].lengthCounter; got != 0xff {
		t.Fatalf("wave length after trigger-side extra clock = %#x, want 0xff", got)
	}
	if !a.channels[2].enabled {
		t.Fatal("wave channel not enabled after trigger-side length reload")
	}
}

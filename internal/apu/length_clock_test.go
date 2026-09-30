package apu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newLengthClockTestAPU(t *testing.T, model types.Model) *APU {
	t.Helper()
	s := scheduler.NewScheduler()
	b := io.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	return New(b, s)
}

func TestEarlyCGBExtraLengthClockWithoutEnableTransition(t *testing.T) {
	for _, model := range []types.Model{types.CGB0, types.CGBBC} {
		t.Run(model.String(), func(t *testing.T) {
			a := newLengthClockTestAPU(t, model)
			a.frameSequencerStep = 1

			for _, tc := range []struct {
				name string
				ch   uint16
				reg  uint16
			}{
				{name: "channel1", ch: 0, reg: types.NR14},
				{name: "channel2", ch: 1, reg: types.NR24},
				{name: "channel3", ch: 2, reg: types.NR34},
				{name: "channel4", ch: 3, reg: types.NR44},
			} {
				t.Run(tc.name, func(t *testing.T) {
					a.channels[tc.ch].lengthCounter = 1
					a.channels[tc.ch].lengthCounterEnabled = false
					a.channels[tc.ch].enabled = true
					a.channels[tc.ch].dacEnabled = true

					a.Write(tc.reg, 0x00)

					if got := a.channels[tc.ch].lengthCounter; got != 0 {
						t.Fatalf("length counter = %d, want 0", got)
					}
					if a.channels[tc.ch].enabled {
						t.Fatal("channel stayed enabled after early-CGB extra length clock expired it")
					}
					if a.channels[tc.ch].lengthCounterEnabled {
						t.Fatal("write without bit 6 unexpectedly enabled length counter")
					}
				})
			}
		})
	}
}

func TestLaterCGBRequiresLengthEnableTransitionForExtraClock(t *testing.T) {
	a := newLengthClockTestAPU(t, types.CGBDE)
	a.frameSequencerStep = 1
	a.channels[0].lengthCounter = 1
	a.channels[0].enabled = true
	a.channels[0].dacEnabled = true

	a.Write(types.NR14, 0x00)
	if got := a.channels[0].lengthCounter; got != 1 {
		t.Fatalf("length counter after disabled->disabled write = %d, want 1", got)
	}
	if !a.channels[0].enabled {
		t.Fatal("later-CGB channel disabled without a length-enable transition")
	}

	a.Write(types.NR14, types.Bit6)
	if got := a.channels[0].lengthCounter; got != 0 {
		t.Fatalf("length counter after disabled->enabled write = %d, want 0", got)
	}
	if a.channels[0].enabled {
		t.Fatal("later-CGB channel stayed enabled after valid extra length clock")
	}
}

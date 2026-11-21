package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// CPU read timing is a separate bus observation phase. A PPU event on
// the last T-state of an M-cycle must affect STAT I/O reads, while
// native CGB normal-speed VRAM is sampled on T3 (before that event).
func TestClockedReadSamplesSTATAtEndOfMachineCycle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		model  types.Model
		double bool
	}{
		{"DMG", types.DMGABC, false},
		{"CGB-BC-normal", types.CGBBC, false},
		{"CGB-DE-normal", types.CGBDE, false},
		{"CGB-BC-double", types.CGBBC, true},
		{"CGB-DE-double", types.CGBDE, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			if tc.double {
				s.ChangeSpeed(true)
			}
			stat := byte(0x80)
			b.ReserveLazyReader(types.STAT, func() byte { return stat })
			s.RegisterEvent(scheduler.TimerTIMAReload, func() {
				stat = 0x82
			})
			s.ScheduleEvent(scheduler.TimerTIMAReload, 4)

			if got := b.cpuReadSampleTStates(types.STAT); got != 4 {
				t.Fatalf("STAT CPU read sampled at T%d, want T4", got)
			}
			if got := b.ClockedRead(types.STAT); got != 0x82 {
				t.Errorf("CPU STAT read skipped T4 PPU update: %#02x, want %#02x", got, 0x82)
			}
			if got := s.Cycle(); got != 4 {
				t.Errorf("STAT CPU read M-cycle = %d T-states, want 4", got)
			}
		})
	}
}

func TestClockedReadKeepsIndependentCGBVRAMPhase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		model  types.Model
		double bool
		phase  uint64
	}{
		{"DMG", types.DMGABC, false, 4},
		{"CGB-BC-normal", types.CGBBC, false, 3},
		{"CGB-DE-normal", types.CGBDE, false, 3},
		{"CGB-BC-double", types.CGBBC, true, 4},
		{"CGB-DE-double", types.CGBDE, true, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			if tc.double {
				s.ChangeSpeed(true)
			}
			b.Set(0x8000, 0x11)
			s.RegisterEvent(scheduler.TimerTIMAReload, func() {
				b.Set(0x8000, 0x22)
			})
			s.ScheduleEvent(scheduler.TimerTIMAReload, 4)

			if got := b.cpuReadSampleTStates(0x8000); got != tc.phase {
				t.Fatalf("VRAM CPU read phase = T%d; want T%d", got, tc.phase)
			}
			want := byte(0x22)
			if tc.phase == 3 {
				want = 0x11
			}
			if got := b.ClockedRead(0x8000); got != want {
				t.Errorf("VRAM sample when PPU bus changes at T4 = %#02x; want %#02x", got, want)
			}
			if got := s.Cycle(); got != 4 {
				t.Errorf("CPU VRAM read M-cycle = %d T-states; want 4", got)
			}
		})
	}
}

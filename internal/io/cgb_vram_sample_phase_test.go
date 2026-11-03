package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// CPU-read samples at the end of a four-clock M-cycle currently collide with
// PPU events scheduled at the same cycle. Normal-speed CGB VRAM read timing
// is one T-state earlier; double-speed CPU arbitration remains unchanged.
func TestCGBVRAMClockedReadNormalSpeedSamplePhase(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, speed := range []bool{false, true} {
			name := model.String() + "/" + map[bool]string{false:"normal", true:"double"}[speed]
			t.Run(name+"/close-at-M-end", func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				if speed { s.ChangeSpeed(true) }
				b.Write(0x8000, 0x5a)
				s.RegisterEvent(scheduler.EIHaltDelay, func() { b.RLock(VRAM) })
				s.ScheduleEvent(scheduler.EIHaltDelay, 4)
				start := s.Cycle()
				got := b.ClockedRead(0x8000)
				want := byte(0x5a)
				if speed { want = 0xff }
				if got != want {
					t.Fatalf("read at PPU lock edge: got %02x, want %02x", got, want)
				}
				if s.Cycle() != start + 4 { t.Fatalf("ClockedRead must consume exactly 4 cycles") }
				if v := b.Read(0x8000); v != 0xff {
					t.Fatalf("VRAM must be locked after M-cycle, got %02x", v)
				}
			})
			t.Run(name+"/open-at-M-end", func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				if speed { s.ChangeSpeed(true) }
				b.Write(0x8000, 0x5a)
				b.RLock(VRAM)
				s.RegisterEvent(scheduler.EIHaltDelay, func() { b.RUnlock(VRAM) })
				s.ScheduleEvent(scheduler.EIHaltDelay, 4)
				got := b.ClockedRead(0x8000)
				want := byte(0xff)
				if speed { want = 0x5a }
				if got != want {
					t.Fatalf("read at PPU unlock edge: got %02x, want %02x", got, want)
				}
				if v := b.Read(0x8000); v != 0x5a {
					t.Fatalf("VRAM must be open after M-cycle, got %02x", v)
				}
			})
		}
	}
}

func TestDMGClockedVRAMReadRetainsEndOfCycleSample(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	b.Write(0x8000, 0x5a)
	s.RegisterEvent(scheduler.EIHaltDelay, func() { b.RLock(VRAM) })
	s.ScheduleEvent(scheduler.EIHaltDelay, 4)
	if got := b.ClockedRead(0x8000); got != 0xff {
		t.Fatalf("DMG VRAM read should observe lock at M-cycle end, got %02x", got)
	}
}

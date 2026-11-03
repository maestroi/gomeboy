package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// CPU VRAM sample phase is independent of the total four-cycle M-cycle length.
// The ordinary DMG and other memory regions keep sampling at the end of the
// cycle. These checks intentionally schedule the PPU bus change between the
// two halves of a CGB CPU memory access.
func TestClockedVRAMReadCGBMidcycleBusOwnership(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String()+"/close-after-sample", func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			b.Write(0x8000, 0x5a)
			s.RegisterEvent(scheduler.PPUHandleVisualLine, func() { b.RLock(VRAM) })
			s.ScheduleEvent(scheduler.PPUHandleVisualLine, 4)

			start := s.Cycle()
			if got := b.ClockedRead(0x8000); got != 0x5a {
				t.Fatalf("read should sample before lock at T3, got %02x", got)
			}
			if delta := s.Cycle() - start; delta != 4 {
				t.Fatalf("clocked VRAM read consumed %d instead of 4 cycles", delta)
			}
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("PPU VRAM lock at end of cycle not applied: %02x", got)
			}
		})
		t.Run(model.String()+"/open-after-sample", func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			b.Write(0x8000, 0x5a)
			b.RLock(VRAM)
			s.RegisterEvent(scheduler.PPUHandleVisualLine, func() { b.RUnlock(VRAM) })
			s.ScheduleEvent(scheduler.PPUHandleVisualLine, 4)

			if got := b.ClockedRead(0x8000); got != 0xff {
				t.Fatalf("read should sample before HBlank bus release, got %02x", got)
			}
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM should open after the complete M-cycle: %02x", got)
			}
		})
	}
}

func TestClockedReadDMGVRAMAndCGBWRAMRetainEndOfCycleSample(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		addr  uint16
	}{
		{types.DMGABC, 0x8000},
		{types.CGBBC, 0xc000},
	} {
		s := scheduler.NewScheduler()
		b := NewBus(s, make([]byte, 0x8000))
		b.Map(tc.model)
		b.Write(tc.addr, 0x5a)
		s.RegisterEvent(scheduler.PPUHandleVisualLine, func() {
			if tc.addr == 0x8000 {
				b.RLock(VRAM)
			} else {
				b.RLock(cgbWRAMBus)
			}
		})
		s.ScheduleEvent(scheduler.PPUHandleVisualLine, 3)
		if got := b.ClockedRead(tc.addr); got != 0xff {
			t.Fatalf("model %s addr %04x: expected end-of-cycle locked value, got %02x", tc.model, tc.addr, got)
		}
	}
}

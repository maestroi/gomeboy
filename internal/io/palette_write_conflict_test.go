package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// SameBoy GB_CONFLICT_PALETTE_DMG: DMG drives (new|old) for one T-state
// then the clean value one T early, still within a 4 T write M-cycle.
func TestDMGPaletteWriteORThenSettle(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	b.Set(types.BGP, 0xE4)

	var commits []byte
	b.ReserveAddress(types.BGP, func(v byte) byte {
		commits = append(commits, v)
		return v
	})

	start := s.Cycle()
	b.ClockedWrite(uint16(types.BGP), 0x00)
	if got := s.Cycle() - start; got != 4 {
		t.Fatalf("DMG BGP conflict write advanced %d T, want 4", got)
	}
	if len(commits) != 2 || commits[0] != 0xE4 || commits[1] != 0x00 {
		t.Fatalf("DMG BGP commits = %v, want [E4, 00] (OR then settle)", commits)
	}
}

func TestCGBPaletteWriteUsesDefaultPhase(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		s := scheduler.NewScheduler()
		rom := make([]byte, 0x8000)
		rom[0x143] = 0x80
		b := NewBus(s, rom)
		b.Map(model)
		b.ReserveAddress(types.BGP, func(v byte) byte { return v })
		start := s.Cycle()
		b.ClockedWrite(uint16(types.BGP), 0x1B)
		if got := s.Cycle() - start; got != 4 {
			t.Fatalf("%s advanced %d T, want 4", model, got)
		}
		if b.Get(types.BGP) != 0x1B {
			t.Fatalf("%s BGP = %02x, want 1B", model, b.Get(types.BGP))
		}
	}
}

package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestDMGOBPWriteORThenSettle(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	b.Set(types.OBP0, 0xE4)
	var commits []byte
	b.ReserveAddress(types.OBP0, func(v byte) byte {
		commits = append(commits, v)
		return v
	})
	start := s.Cycle()
	b.ClockedWrite(uint16(types.OBP0), 0x00)
	if got := s.Cycle() - start; got != 4 {
		t.Fatalf("DMG OBP0 conflict write advanced %d T, want 4", got)
	}
	if len(commits) != 2 || commits[0] != 0xE4 || commits[1] != 0x00 {
		t.Fatalf("DMG OBP0 commits = %v, want [E4, 00]", commits)
	}
}

func TestDMGBGPClockedWriteUsesDefaultPhase(t *testing.T) {
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
		t.Fatalf("DMG BGP write advanced %d T, want 4", got)
	}
	if len(commits) != 1 || commits[0] != 0x00 {
		t.Fatalf("DMG BGP commits = %v, want [00]", commits)
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
	}
}

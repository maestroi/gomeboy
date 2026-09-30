package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestOAMDMAPausesWhileCPUHalted(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.MGB)

	b.data[0xc000] = 0x42
	b.data[0xfe00] = 0x11
	b.dmaSource = 0xc000
	b.dmaDestination = 0xfe00
	b.dmaActive = true
	b.dmaEnabled = true

	s.Halted = true
	b.doDMATransfer()
	s.Tick(4)

	if got := b.dmaSource; got != 0xc000 {
		t.Fatalf("DMA source advanced during HALT: got %#04x, want 0xc000", got)
	}
	if got := b.dmaDestination; got != 0xfe00 {
		t.Fatalf("DMA destination advanced during HALT: got %#04x, want 0xfe00", got)
	}
	if got := b.data[0xfe00]; got != 0x11 {
		t.Fatalf("OAM changed during HALT: got %#02x, want 0x11", got)
	}

	s.Halted = false
	s.Tick(4)

	if got := b.dmaSource; got != 0xc001 {
		t.Fatalf("DMA source did not resume after HALT: got %#04x, want 0xc001", got)
	}
	if got := b.dmaDestination; got != 0xfe01 {
		t.Fatalf("DMA destination did not resume after HALT: got %#04x, want 0xfe01", got)
	}
	if got := b.data[0xfe00]; got != 0x42 {
		t.Fatalf("resumed DMA copied %#02x to OAM, want 0x42", got)
	}
}

func TestMGBHaltedDMAExposesMeasuredOAMBusWord(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.MGB)

	b.dmaActive = true
	b.dmaSource = 0x2002
	b.dmaDestination = 0xfe02
	b.data[0x2002] = 0x1a
	b.data[0xfe00] = 0x12
	b.data[0xfe02] = 0x30
	b.data[0xfe03] = 0x40

	// This row is the MGB enable pattern used by the Mooneye hardware test.
	b.data[0xfe04] = 0x9f
	b.data[0xfe05] = 0xa7
	b.data[0xfe06] = 0x9f
	b.data[0xfe07] = 0xa7
	s.Halted = true

	for _, tc := range []struct {
		addr uint16
		want byte
	}{
		{0xfe00, 0x38},
		{0xfe01, 0x5a},
		{0xfe7e, 0x38},
		{0xfe9f, 0x5a},
	} {
		if got := b.PPUReadOAM(tc.addr); got != tc.want {
			t.Errorf("PPU OAM read at %#04x = %#02x, want %#02x", tc.addr, got, tc.want)
		}
	}

	// Without an enabling row the measured MGB profile resolves to an idle
	// high bus instead of producing sprite data.
	b.data[0xfe04] = 0x97
	if got := b.PPUReadOAM(0xfe00); got != 0xff {
		t.Errorf("PPU OAM read without enable pattern = %#02x, want 0xff", got)
	}

	// Leaving HALT restores ordinary raw OAM visibility to this PPU path.
	s.Halted = false
	if got := b.PPUReadOAM(0xfe00); got != 0x12 {
		t.Errorf("PPU OAM read after HALT = %#02x, want raw OAM 0x12", got)
	}
}

func TestHaltedDMAConflictProfileIsMGBSpecific(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.dmaActive = true
	b.dmaSource = 0x2002
	b.dmaDestination = 0xfe02
	b.data[0x2002] = 0x1a
	b.data[0xfe00] = 0x12
	b.data[0xfe02] = 0x30
	b.data[0xfe03] = 0x40
	b.data[0xfe04] = 0x9f
	b.data[0xfe05] = 0xa7
	b.data[0xfe06] = 0x9f
	b.data[0xfe07] = 0xa7
	s.Halted = true

	if got := b.PPUReadOAM(0xfe00); got != 0x12 {
		t.Fatalf("DMG inherited MGB halted-DMA profile: got %#02x, want raw OAM 0x12", got)
	}
}

func TestMGBHaltedDMAUsesExactInFlightOAMByte(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.MGB)

	b.dmaActive = true
	b.dmaSource = 0x2002
	b.dmaDestination = 0xfe03
	b.data[0x2002] = 0x1a
	b.data[0xfe03] = 0x40
	b.data[0xfe04] = 0x9f
	b.data[0xfe05] = 0xa7
	b.data[0xfe06] = 0x9f
	b.data[0xfe07] = 0xa7
	s.Halted = true

	if got := b.PPUReadOAM(0xfe00); got != 0x58 {
		t.Errorf("odd-index frozen DMA Y/tile = %#02x, want 0x58", got)
	}
	if got := b.PPUReadOAM(0xfe01); got != 0x9f {
		t.Errorf("odd-index frozen DMA X/flags = %#02x, want 0x9f", got)
	}
}


func TestActiveOAMDMAHidesEntriesFromPPUScan(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.data[0xfe00] = 0x54
	b.dmaActive = true

	if got := b.PPUReadOAMScan(0xfe00); got != 0xff {
		t.Fatalf("Mode 2 OAM read during active DMA = %#02x, want 0xff", got)
	}

	b.dmaActive = false
	if got := b.PPUReadOAMScan(0xfe00); got != 0x54 {
		t.Fatalf("Mode 2 OAM read after DMA = %#02x, want 0x54", got)
	}
}

func TestActiveOAMDMAExposesCurrentWordToObjectFetcher(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.dmaActive = true
	b.dmaDestination = 0xfe03 // FE02 was the most recently copied byte.
	b.data[0xfe02] = 0x12
	b.data[0xfe03] = 0x34

	if got := b.PPUReadOAMFetch(0xfe42); got != 0x12 {
		t.Fatalf("Mode 3 tile byte during DMA = %#02x, want 0x12", got)
	}
	if got := b.PPUReadOAMFetch(0xfe43); got != 0x34 {
		t.Fatalf("Mode 3 attribute byte during DMA = %#02x, want 0x34", got)
	}
}

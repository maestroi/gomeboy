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

func TestActiveOAMDMABlocksMode2BusRefresh(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.dmaActive = true
	if !b.PPUOAMScanBlockedByDMA() {
		t.Fatal("active OAM DMA did not block Mode 2 bus refresh")
	}

	b.dmaActive = false
	if b.PPUOAMScanBlockedByDMA() {
		t.Fatal("inactive OAM DMA still blocked Mode 2 bus refresh")
	}
}

func TestMGBHaltedDMAKeepsMode2BusReadable(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.MGB)

	b.dmaActive = true
	s.Halted = true
	if b.PPUOAMScanBlockedByDMA() {
		t.Fatal("MGB halted-DMA profile unexpectedly blocked Mode 2 bus refresh")
	}
}

func TestActiveOAMDMAExposesCurrentWordToObjectFetcher(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.dmaActive = true

	// While the next destination is odd, the PPU sees that destination's word.
	b.dmaDestination = 0xfe03
	b.data[0xfe02] = 0x12
	b.data[0xfe03] = 0x34
	if got := b.PPUReadOAMFetch(0xfe42); got != 0x12 {
		t.Fatalf("Mode 3 tile byte during DMA = %#02x, want 0x12", got)
	}
	if got := b.PPUReadOAMFetch(0xfe43); got != 0x34 {
		t.Fatalf("Mode 3 attribute byte during DMA = %#02x, want 0x34", got)
	}

	// Crossing to an even destination advances the exposed 16-bit OAM word.
	// This boundary is what strikethrough.gb relies on while fetching its OBJ.
	b.dmaDestination = 0xfe04
	b.data[0xfe02] = 0xaa
	b.data[0xfe03] = 0xbb
	b.data[0xfe04] = 0x56
	b.data[0xfe05] = 0x78
	if got := b.PPUReadOAMFetch(0xfe42); got != 0x56 {
		t.Fatalf("Mode 3 tile byte after DMA word advance = %#02x, want 0x56", got)
	}
	if got := b.PPUReadOAMFetch(0xfe43); got != 0x78 {
		t.Fatalf("Mode 3 attribute byte after DMA word advance = %#02x, want 0x78", got)
	}
}

func TestCGBROMDMAConflictsOnlyWithCartridgeBus(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBABC)

	b.Write(types.DMA, 0x40)
	if b.dmaConflicted != cgbCartBus {
		t.Fatalf("ROM DMA conflict mask = %#04x, want %#04x", b.dmaConflicted, cgbCartBus)
	}

	b.dmaActive = true
	b.dmaConflict = 0x42
	b.data[0x1234] = 0x99
	b.data[0xc123] = 0x77

	if got := b.Read(0x1234); got != 0x42 {
		t.Fatalf("cartridge-bus read during ROM DMA = %#02x, want DMA byte 0x42", got)
	}
	if got := b.Read(0xc123); got != 0x77 {
		t.Fatalf("WRAM read during ROM DMA = %#02x, want 0x77", got)
	}

	b.Write(0xc123, 0x56)
	if got := b.Read(0xc123); got != 0x56 {
		t.Fatalf("WRAM write during ROM DMA = %#02x, want 0x56", got)
	}
}

func TestCGBDMASelectsSourceBus(t *testing.T) {
	tests := []struct {
		name string
		src  byte
		want uint16
	}{
		{name: "ROM", src: 0x40, want: cgbCartBus},
		{name: "VRAM", src: 0x80, want: VRAM},
		{name: "cartridge RAM", src: 0xa0, want: cgbCartBus},
		{name: "WRAM", src: 0xc0, want: cgbWRAMBus},
		{name: "echo WRAM", src: 0xe0, want: cgbWRAMBus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := NewBus(s, make([]byte, 0x8000))
			b.Map(types.CGBABC)
			b.Write(types.DMA, tt.src)
			if got := b.dmaConflicted; got != tt.want {
				t.Fatalf("DMA %#02x conflict mask = %#04x, want %#04x", tt.src, got, tt.want)
			}
		})
	}
}

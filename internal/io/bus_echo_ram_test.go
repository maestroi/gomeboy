package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// TestEchoRAMMirrorsWorkRAM pins the hardware contract for the E000-FDFF echo
// window: work RAM is one set of cells readable at both C000-DDFF and the
// address that mirrors it. Boxxle's tile-run copy stores to EE00 and re-reads
// the same address to confirm each byte before advancing, so a store the CPU
// cannot read back hangs the game forever on the first byte it checks.
func TestEchoRAMMirrorsWorkRAM(t *testing.T) {
	b := NewBus(scheduler.NewScheduler(), make([]byte, 0x8000))

	cases := []struct {
		name string
		echo uint16
		wram uint16
	}{
		{"base", 0xE000, 0xC000},
		{"low byte bit 9 clear", 0xEDFA, 0xCDFA},
		{"low byte bit 9 set", 0xEE00, 0xCE00},
		{"top of the mirror", 0xFDFF, 0xDDFF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b.Write(tc.echo, 0x5A)
			if got := b.Read(tc.echo); got != 0x5A {
				t.Errorf("read back %#04x after writing it: got %#02x, want 0x5a", tc.echo, got)
			}
			if got := b.Read(tc.wram); got != 0x5A {
				t.Errorf("write to echo %#04x not visible at %#04x: got %#02x, want 0x5a", tc.echo, tc.wram, got)
			}

			b.Write(tc.wram, 0xA5)
			if got := b.Read(tc.wram); got != 0xA5 {
				t.Errorf("read back %#04x after writing it: got %#02x, want 0xa5", tc.wram, got)
			}
			if got := b.Read(tc.echo); got != 0xA5 {
				t.Errorf("write to work RAM %#04x not visible at echo %#04x: got %#02x, want 0xa5", tc.wram, tc.echo, got)
			}
		})
	}
}

// TestEchoRAMStopsAtDDFF checks the other half of the contract: DE00-DFFF has no
// echo, because the addresses that would mirror it belong to OAM and IO.
func TestEchoRAMStopsAtDDFF(t *testing.T) {
	b := NewBus(scheduler.NewScheduler(), make([]byte, 0x8000))

	b.Write(0xFE00, 0x00)
	b.Write(0xDE00, 0x11)
	if got := b.Read(0xFE00); got != 0x00 {
		t.Errorf("non-echoed work RAM write to 0xDE00 leaked into OAM at 0xFE00: got %#02x, want 0x00", got)
	}
}

// TestEchoRAMFollowsWRAMBankSwitch checks that the echo window still mirrors
// work RAM after the CGB switches the D000-DFFF bank, which is echoed at
// F000-FDFF.
func TestEchoRAMFollowsWRAMBankSwitch(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x0143] = 0x80 // CGB-enhanced cartridge, so the CGB registers exist
	b := NewBus(scheduler.NewScheduler(), rom)
	b.Map(types.CGB0)

	b.Set(types.SVBK, 0xF8) // bank 1: SVBK values 0 and 1 both select bank 1
	b.Write(0xD123, 0x11)   // lands in bank 1
	if got := b.Read(0xF123); got != 0x11 {
		t.Fatalf("echo of a banked work RAM write: 0xF123 = %#02x, want 0x11", got)
	}

	b.Write(types.SVBK, 0xFA) // bank 2
	if got := b.Read(0xF123); got == 0x11 {
		t.Errorf("echo window still shows bank 1 at 0xF123 after switching to bank 2")
	}

	b.Write(types.SVBK, 0xF8) // back to bank 1
	if got := b.Read(0xF123); got != 0x11 {
		t.Errorf("echo window lost bank 1 at 0xF123 after switching away and back: got %#02x, want 0x11", got)
	}
}

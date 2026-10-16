package io

import (
	"errors"
	"testing"
)

func TestAdvanceRTCSecondsPreservesLatchAndEmulatorCycle(t *testing.T) {
	b, now := newMBC3RTCTestBus(t)
	writeRTCRegister(b, 0x08, 50)
	writeRTCRegister(b, 0x09, 59)
	writeRTCRegister(b, 0x0a, 3)
	latchRTC(b)
	before := b.Snapshot()
	*now = rtcCyclesPerSecond / 2

	if err := b.c.AdvanceRTCSeconds(20); err != nil {
		t.Fatal(err)
	}
	if got := readRTCRegister(b, 0x0a); got != 3 {
		t.Fatalf("old latched hour = %d, want 3", got)
	}
	if got, err := b.c.RTCState(); err != nil || got.Hours != 4 || got.Minutes != 0 || got.Seconds != 10 {
		t.Fatalf("advanced clock = %+v, %v", got, err)
	}
	latchRTC(b)
	if got := readRTCRegister(b, 0x0a); got != 4 {
		t.Fatalf("new latched hour = %d, want 4", got)
	}
	if *now != rtcCyclesPerSecond/2 {
		t.Fatalf("RTC advance touched emulated timebase: %d", *now)
	}

	// The existing checkpoint captures live and latched registers, plus the
	// last-update cycle. Restoring it must also restore the original clock.
	b.Restore(before)
	if got, err := b.c.RTCState(); err != nil || got.Hours != 3 || got.Minutes != 59 || got.Seconds != 50 {
		t.Fatalf("restored clock = %+v, %v", got, err)
	}
}

func TestAdvanceRTCSecondsCarriesDayAndIsDeterministic(t *testing.T) {
	b, _ := newMBC3RTCTestBus(t)
	writeRTCRegister(b, 0x08, 59)
	writeRTCRegister(b, 0x09, 59)
	writeRTCRegister(b, 0x0a, 23)
	writeRTCRegister(b, 0x0b, 255)
	writeRTCRegister(b, 0x0c, 1) // day 511
	start := b.Snapshot()

	advance := func() RTCState {
		t.Helper()
		b.Restore(start)
		if err := b.c.AdvanceRTCSeconds(1); err != nil {
			t.Fatal(err)
		}
		state, err := b.c.RTCState()
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	a, d := advance(), advance()
	if a != d || a.Hours != 0 || a.Minutes != 0 || a.Seconds != 0 || a.Days != 0 || !a.Carry {
		t.Fatalf("advance across day 511 = %+v, repeat = %+v", a, d)
	}
}

func TestAdvanceRTCSecondsUnavailableAndHalted(t *testing.T) {
	if err := (*Cartridge)(nil).AdvanceRTCSeconds(60); !errors.Is(err, ErrRTCUnavailable) {
		t.Fatalf("nil cartridge = %v", err)
	}
	b, _ := newMBC3RTCTestBus(t)
	writeRTCRegister(b, 0x0c, 0x40)
	if err := b.c.AdvanceRTCSeconds(60); !errors.Is(err, ErrRTCHalted) {
		t.Fatalf("halted clock = %v", err)
	}
	state, _ := b.c.RTCState()
	if !state.Halted {
		t.Fatal("halt bit lost")
	}
}

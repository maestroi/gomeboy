package gomeboy

import (
	"bytes"
	"testing"
)

func TestPeekMatchesReadWhenQuiet(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrames(10)

	check := func(lo, hi uint16) {
		t.Helper()
		for addr := lo; addr <= hi; addr++ {
			if got, want := e.Peek8(addr), e.Read8(addr); got != want {
				t.Fatalf("Peek8(0x%04X) = 0x%02X, Read8 = 0x%02X", addr, got, want)
			}
		}
	}
	check(0xC000, 0xC0FF)
	check(0xFF80, 0xFFFE)
}

func TestPeek16LittleEndian(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrames(10)

	const addr = uint16(0xC000)
	want := uint16(e.Peek8(addr)) | uint16(e.Peek8(addr+1))<<8
	if got := e.Peek16(addr); got != want {
		t.Fatalf("Peek16(0x%04X) = 0x%04X, want 0x%04X", addr, got, want)
	}
}

func TestPeekIntoMatchesPeek8(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrames(10)

	const addr = uint16(0xC000)
	buf := make([]byte, 256)
	e.PeekInto(addr, buf)
	for i := range buf {
		if buf[i] != e.Peek8(addr+uint16(i)) {
			t.Fatalf("PeekInto[%d] = 0x%02X, Peek8 = 0x%02X", i, buf[i], e.Peek8(addr+uint16(i)))
		}
	}
}

func TestPeekDoesNotAdvanceState(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()
	e.StepFrames(20)

	before := e.snapshot()
	for i := 0; i < 500; i++ {
		e.Peek8(uint16(i * 33))
	}
	if !bytes.Equal(before, e.snapshot()) {
		t.Fatal("frame changed after Peek calls")
	}

	ref := newTestEmulator(t)
	defer ref.Close()
	ref.StepFrames(21)

	e.StepFrame()
	if !bytes.Equal(e.snapshot(), ref.snapshot()) {
		t.Fatal("frame after peek+step does not match an emulator stepped without peeks")
	}
}

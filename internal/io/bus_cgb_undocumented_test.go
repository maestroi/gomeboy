package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBUndocumentedScratchRegisters(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBABC)

	for _, addr := range []uint16{uint16(types.FF72), uint16(types.FF73), uint16(types.FF74)} {
		if got := b.Read(addr); got != 0x00 {
			t.Errorf("%#04x initial value = %#02x, want 0x00", addr, got)
		}
		b.Write(addr, 0xff)
		if got := b.Read(addr); got != 0xff {
			t.Errorf("%#04x after write = %#02x, want 0xff", addr, got)
		}
	}

	if got := b.Read(uint16(types.FF75)); got != 0x8f {
		t.Fatalf("FF75 initial value = %#02x, want 0x8f", got)
	}
	b.Write(uint16(types.FF75), 0x70)
	if got := b.Read(uint16(types.FF75)); got != 0xff {
		t.Errorf("FF75 writable-bit mask = %#02x, want 0xff", got)
	}
}

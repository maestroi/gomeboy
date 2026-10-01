package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newExtraOAMTestBus(t *testing.T, model types.Model) *Bus {
	t.Helper()
	rom := make([]byte, 0x8000)
	s := scheduler.NewScheduler()
	b := NewBus(s, rom)
	b.Map(model)
	return b
}

func TestCGBABCExtraOAMAliasesAddressBitsThreeAndFour(t *testing.T) {
	b := newExtraOAMTestBus(t, types.CGBABC)

	b.Write(0xFEA0, 0x55)
	b.Write(0xFEB8, 0x44)

	if got := b.Read(0xFEA0); got != 0x44 {
		t.Fatalf("FEA0 after aliased FEB8 write = %#02x, want 0x44", got)
	}
	if got := b.Read(0xFEB8); got != 0x44 {
		t.Fatalf("FEB8 aliased read = %#02x, want 0x44", got)
	}
}

func TestLaterCGBExtraOAMUsesAddressPattern(t *testing.T) {
	b := newExtraOAMTestBus(t, types.CGBDE)

	b.Write(0xFEB8, 0x44)
	if got := b.Read(0xFEB8); got != 0xBB {
		t.Fatalf("FEB8 read = %#02x, want 0xbb", got)
	}
}

func TestDMGExtraOAMReadsZero(t *testing.T) {
	b := newExtraOAMTestBus(t, types.DMGABC)

	b.Write(0xFEA0, 0x55)
	if got := b.Read(0xFEA0); got != 0 {
		t.Fatalf("FEA0 read = %#02x, want 0x00", got)
	}
}

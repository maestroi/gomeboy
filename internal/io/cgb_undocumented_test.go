package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func cgbRegisterTestROM(cgb bool) []byte {
	rom := make([]byte, 0x8000)
	if cgb {
		rom[0x143] = 0x80
	}
	return rom
}

func TestFF74CGBModeStartsZeroAndIsReadWrite(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, cgbRegisterTestROM(true))
	b.Map(types.CGBABC)

	if got := b.Read(types.FF74); got != 0x00 {
		t.Fatalf("initial FF74 = %#02x, want 0x00", got)
	}

	b.Write(types.FF74, 0x5a)
	if got := b.Read(types.FF74); got != 0x5a {
		t.Fatalf("FF74 after write = %#02x, want 0x5a", got)
	}
}

func TestFF74LockedInCGBDMGCompatibilityMode(t *testing.T) {
	s := scheduler.NewScheduler()
	b := NewBus(s, cgbRegisterTestROM(false))
	b.Map(types.CGBABC)

	if got := b.Read(types.FF74); got != 0xff {
		t.Fatalf("compatibility-mode FF74 = %#02x, want 0xff", got)
	}

	b.Write(types.FF74, 0x5a)
	if got := b.Read(types.FF74); got != 0xff {
		t.Fatalf("compatibility-mode FF74 after write = %#02x, want 0xff", got)
	}
}

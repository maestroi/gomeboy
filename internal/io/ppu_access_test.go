package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newPPUAccessTestBus(t *testing.T) *Bus {
	t.Helper()
	b := NewBus(scheduler.NewScheduler(), make([]byte, 0x8000))
	b.Map(types.CGBBC)
	b.Set(0x8000, 0x5a)
	b.Set(0xfe00, 0x66)
	return b
}

func TestPPUAccessHooksPreserveDefaultLocks(t *testing.T) {
	b := newPPUAccessTestBus(t)
	b.Lock(VRAM | OAM)

	// A bus with no hooks must behave exactly like the old coarse-lock model.
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("locked VRAM read: got %#02x, want 0xff", got)
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("locked OAM read: got %#02x, want 0xff", got)
	}
	b.Write(0xfe00, 0x77)
	if got := b.Get(0xfe00); got != 0x66 {
		t.Fatalf("locked OAM write changed memory to %#02x", got)
	}

	// Passing nil explicitly must also preserve and restore the old behavior.
	b.RegisterPPUAccessHandlers(nil, nil, nil)
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("nil VRAM hook bypassed lock: got %#02x", got)
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("nil OAM hook bypassed lock: got %#02x", got)
	}
	b.Unlock(VRAM | OAM)
	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("unlocked VRAM read: got %#02x, want 0x5a", got)
	}
	b.Write(0xfe00, 0x77)
	if got := b.Read(0xfe00); got != 0x77 {
		t.Fatalf("unlocked OAM write: got %#02x, want 0x77", got)
	}
}

func TestPPUAccessHooksCanRefineReadAndWriteSeparately(t *testing.T) {
	b := newPPUAccessTestBus(t)
	b.Lock(VRAM | OAM)
	var vramSeen, readSeen, writeSeen bool
	b.RegisterPPUAccessHandlers(
		func(defaultOpen bool) bool {
			vramSeen = true
			if defaultOpen {
				t.Fatal("expected locked VRAM default")
			}
			return true
		},
		func(defaultOpen bool) bool {
			readSeen = true
			if defaultOpen {
				t.Fatal("expected locked OAM read default")
			}
			return true
		},
		func(defaultOpen bool) bool {
			writeSeen = true
			if defaultOpen {
				t.Fatal("expected locked OAM write default")
			}
			return true
		},
	)

	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("refined VRAM read: got %#02x", got)
	}
	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("refined OAM read: got %#02x", got)
	}
	b.Write(0xfe00, 0x88)
	if got := b.Get(0xfe00); got != 0x88 {
		t.Fatalf("refined OAM write: got %#02x", got)
	}
	if !vramSeen || !readSeen || !writeSeen {
		t.Fatalf("callbacks missed accesses: VRAM=%t OAM-read=%t OAM-write=%t", vramSeen, readSeen, writeSeen)
	}

	// Callbacks must also be able to close an otherwise unlocked window.
	b.Unlock(VRAM | OAM)
	b.RegisterPPUAccessHandlers(
		func(defaultOpen bool) bool {
			if !defaultOpen {
				t.Fatal("expected open VRAM default")
			}
			return false
		},
		func(defaultOpen bool) bool {
			if !defaultOpen {
				t.Fatal("expected open OAM read default")
			}
			return false
		},
		func(defaultOpen bool) bool {
			if !defaultOpen {
				t.Fatal("expected open OAM write default")
			}
			return false
		},
	)
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("refined VRAM close: got %#02x", got)
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("refined OAM read close: got %#02x", got)
	}
	b.Write(0xfe00, 0x99)
	if got := b.Get(0xfe00); got != 0x88 {
		t.Fatalf("refined OAM write close changed data: got %#02x", got)
	}
}

func TestPPUAccessHooksRespectDMAAndUnusableOAM(t *testing.T) {
	b := newPPUAccessTestBus(t)
	called := 0
	alwaysOpen := func(bool) bool { called++; return true }
	b.RegisterPPUAccessHandlers(alwaysOpen, alwaysOpen, alwaysOpen)

	// The unusable OAM window shares the same CPU read/write lock, and
	// CGB B/C aliases its backing bytes when access is permitted.
	b.Lock(OAM)
	b.Write(0xfeb8, 0x35)
	if got := b.Read(0xfea0); got != 0x35 {
		t.Fatalf("refined unusable-OAM alias: got %#02x", got)
	}
	if called != 2 {
		t.Fatalf("OAM alias access did not invoke read/write hooks twice: got %d", called)
	}

	// OAM DMA source-bus conflicts are resolved before PPU predicates.
	called = 0
	b.dmaActive = true
	b.dmaConflicted = VRAM
	b.dmaConflict = 0xa5
	if got := b.Read(0x8000); got != 0xa5 {
		t.Fatalf("DMA VRAM conflict value: got %#02x", got)
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("DMA OAM conflict value: got %#02x", got)
	}
	b.Write(0xfe00, 0x99)
	if got := b.Get(0xfe00); got != 0x66 {
		t.Fatalf("DMA OAM write bypassed arbitration: got %#02x", got)
	}
	if called != 0 {
		t.Fatalf("DMA conflict should not consult PPU hooks: called %d times", called)
	}
}

package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newLine153RevisionTest(t *testing.T, model types.Model, doubleSpeed bool) (*PPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	p := New(b, s)
	b.Map(model)
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	p.ly = 153
	p.offscreenLineState = StartVBlankLastLine
	p.handleOffscreenLine()
	return p, b, s
}

func TestLine153LYTimingCGBBCNormalSpeed(t *testing.T) {
	p, b, s := newLine153RevisionTest(t, types.CGBBC, false)

	s.Tick(2) // dot 2: LY becomes 153
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("CGB B/C LY at dot 2 = %d, want 153", got)
	}

	s.Tick(3) // dot 5: still 153
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("CGB B/C LY at dot 5 = %d, want 153", got)
	}

	s.Tick(1) // dot 6: B/C normal-speed path exposes 0
	if got := b.Get(types.LY); got != 0 {
		t.Fatalf("CGB B/C LY at dot 6 = %d, want 0", got)
	}
	if p.lyForComparison != 153 {
		t.Fatalf("CGB B/C comparison at dot 6 = %d, want 153", p.lyForComparison)
	}

	s.Tick(2) // dot 8: comparison is disabled until dot 12
	if p.lyForComparison != 0xffff {
		t.Fatalf("CGB B/C comparison at dot 8 = %#x, want 0xffff", p.lyForComparison)
	}
	s.Tick(4)
	if p.lyForComparison != 0 {
		t.Fatalf("CGB B/C comparison at dot 12 = %d, want 0", p.lyForComparison)
	}
}

func TestLine153LYTimingCGBDENormalSpeed(t *testing.T) {
	p, b, s := newLine153RevisionTest(t, types.CGBDE, false)

	s.Tick(2) // dot 2
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("CGB D/E LY at dot 2 = %d, want 153", got)
	}

	s.Tick(2) // dot 4: comparison updates, visible LY remains 153
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("CGB D/E LY at dot 4 = %d, want 153", got)
	}
	if p.lyForComparison != 153 {
		t.Fatalf("CGB D/E comparison at dot 4 = %d, want 153", p.lyForComparison)
	}

	s.Tick(3) // dot 7
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("CGB D/E LY at dot 7 = %d, want 153", got)
	}

	s.Tick(1) // dot 8: visible LY becomes zero, comparison still sees 153
	if got := b.Get(types.LY); got != 0 {
		t.Fatalf("CGB D/E LY at dot 8 = %d, want 0", got)
	}
	if p.lyForComparison != 153 {
		t.Fatalf("CGB D/E comparison at dot 8 = %d, want 153", p.lyForComparison)
	}

	s.Tick(4) // dot 12
	if p.lyForComparison != 0 {
		t.Fatalf("CGB D/E comparison at dot 12 = %d, want 0", p.lyForComparison)
	}
}

func TestLine153LYTimingCGBBCDoubleSpeedUsesLatePath(t *testing.T) {
	p, b, s := newLine153RevisionTest(t, types.CGBBC, true)

	// PPU dot delays are doubled on the CPU scheduler in double-speed mode.
	s.Tick(4) // dot 2
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("double-speed CGB B/C LY at dot 2 = %d, want 153", got)
	}
	s.Tick(4) // dot 4
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("double-speed CGB B/C LY at dot 4 = %d, want 153", got)
	}
	if p.lyForComparison != 153 {
		t.Fatalf("double-speed CGB B/C comparison at dot 4 = %d, want 153", p.lyForComparison)
	}
	s.Tick(6) // dot 7
	if got := b.Get(types.LY); got != 153 {
		t.Fatalf("double-speed CGB B/C LY at dot 7 = %d, want 153", got)
	}
	s.Tick(2) // dot 8
	if got := b.Get(types.LY); got != 0 {
		t.Fatalf("double-speed CGB B/C LY at dot 8 = %d, want 0", got)
	}
	if p.lyForComparison != 153 {
		t.Fatalf("double-speed CGB B/C comparison at dot 8 = %d, want 153", p.lyForComparison)
	}
}

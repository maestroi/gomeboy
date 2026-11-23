package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Mealybug m3_scx_low_3_bits clears SCX during Mode 2, then writes SCX=2
// around line-dot 91 (early rows, still in the lx < 8 junk phase) or 95
// (late rows, after visible pixels started). Fine discard must stay
// mutable while lx < 8 so the early write still shifts the background.
func TestFineSCXMutableDuringJunkPhaseAfterMode2Clear(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Write(types.LCDC, types.Bit7)

	for i := 0; i < 4000; i++ {
		s.Tick(1)
		if p.ly == 1 && p.mode == ModeOAM {
			b.Write(types.SCX, 0)
			break
		}
	}
	if p.mode != ModeOAM || p.ly != 1 {
		t.Fatal("did not reach Mode 2 on line 1")
	}

	// Advance into Mode 3 while SCX is still 0, stopping while the
	// junk/offscreen phase is still open (lx < 8).
	for i := 0; i < 200; i++ {
		s.Tick(1)
		if p.mode == ModeVRAM && p.lx > 0 && p.lx < 8 {
			break
		}
	}
	if p.mode != ModeVRAM || p.lx == 0 || p.lx >= 8 {
		t.Fatalf("expected Mode 3 junk phase 0<lx<8, got mode=%v lx=%d", p.mode, p.lx)
	}

	b.Write(types.SCX, 2)
	for i := 0; i < 24; i++ {
		s.Tick(1)
	}
	if p.scxToDiscard != 2 {
		t.Fatalf("SCX=2 during lx=%d junk phase should set fine discard; got scxToDiscard=%d discarded=%d",
			p.lx, p.scxToDiscard, p.scxDiscarded)
	}
}

func TestFineSCXFrozenAfterVisiblePixelsStart(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Write(types.LCDC, types.Bit7)

	for i := 0; i < 4000; i++ {
		s.Tick(1)
		if p.ly == 1 && p.mode == ModeOAM {
			b.Write(types.SCX, 0)
			break
		}
	}
	for i := 0; i < 200; i++ {
		s.Tick(1)
		if p.mode == ModeVRAM && p.lx >= 8 {
			break
		}
	}
	if p.mode != ModeVRAM || p.lx < 8 {
		t.Fatalf("expected Mode 3 visible phase lx>=8, got mode=%v lx=%d", p.mode, p.lx)
	}

	b.Write(types.SCX, 2)
	for i := 0; i < 24; i++ {
		s.Tick(1)
	}
	if p.scxToDiscard != 0 {
		t.Fatalf("SCX=2 after lx=%d must not reopen fine discard; got scxToDiscard=%d",
			p.lx, p.scxToDiscard)
	}
}

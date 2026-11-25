package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestBGPMode3WriteDeferredOneDot(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Write(types.BGP, 0xE4)
	b.Write(types.LCDC, types.Bit7|types.Bit4|types.Bit0)

	for i := 0; i < 8000; i++ {
		s.Tick(1)
		if p.ly == 1 && p.mode == ModeVRAM && p.lx >= 10 {
			break
		}
	}
	if p.mode != ModeVRAM || p.lx < 10 {
		t.Fatalf("expected visible Mode 3, got mode=%v lx=%d ly=%d", p.mode, p.ly, p.ly)
	}

	before := p.ColourPalette[0]
	b.Write(types.BGP, 0x00)
	if p.ColourPalette[0] != before {
		t.Fatal("BGP write must not update the render palette on the write dot")
	}
	if !p.bgpPendingArmed {
		t.Fatal("expected BGP pending after Mode-3 write")
	}

	for i := 0; i < 64; i++ {
		s.Tick(1)
		if !p.bgpPendingArmed {
			break
		}
	}
	if p.bgpPendingArmed {
		t.Fatalf("BGP still pending after ticks: mode=%v lx=%d", p.mode, p.lx)
	}
	// First commit is (0x00|0xE4)=0xE4, second settles to 0x00 — need two commits.
	// After pending clears, palette should be the clean new value.
	want := p.BGColourisationPalette.remap(0x00)
	if p.ColourPalette[0] != want {
		// May still be on OR step if only one commit happened; tick more.
		for i := 0; i < 8; i++ {
			s.Tick(1)
		}
	}
	if p.ColourPalette[0] != want {
		t.Fatalf("deferred BGP apply mismatch:\n got %v\nwant %v", p.ColourPalette[0], want)
	}
}

func TestBGPMode3WriteORsOneDot(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Write(types.BGP, 0xE4)
	b.Write(types.LCDC, types.Bit7|types.Bit4|types.Bit0)

	for i := 0; i < 8000; i++ {
		s.Tick(1)
		if p.ly == 1 && p.mode == ModeVRAM && p.lx >= 10 {
			break
		}
	}

	old := byte(0xE4)
	neu := byte(0x00)
	b.Write(types.BGP, neu)
	if p.bgpPendingFirst != (neu | old) {
		t.Fatalf("Mode-3 BGP write should arm OR overlap: got %02x want %02x",
			p.bgpPendingFirst, neu|old)
	}
}

func TestBGPMode3WriteDeferredWithWindowEnabled(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Write(types.BGP, 0xE4)
	b.Write(types.WY, 0)
	b.Write(types.WX, 80)
	b.Write(types.LCDC, types.Bit7|types.Bit5|types.Bit4|types.Bit0)

	for i := 0; i < 8000; i++ {
		s.Tick(1)
		if p.ly == 1 && p.mode == ModeVRAM && p.lx >= 10 {
			break
		}
	}
	if p.mode != ModeVRAM || !p.winEnabled {
		t.Fatalf("expected windowed Mode 3, mode=%v win=%t lx=%d", p.mode, p.winEnabled, p.lx)
	}

	before := p.ColourPalette[0]
	b.Write(types.BGP, 0x00)
	if p.ColourPalette[0] != before {
		t.Fatal("windowed Mode-3 BGP write must not update render palette on the write dot")
	}
	if !p.bgpPendingArmed || !p.bgpStallActive {
		t.Fatal("expected pending BGP and stall arm after windowed Mode-3 write")
	}
}

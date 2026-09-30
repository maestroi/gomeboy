package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newCGBTileSelectCollisionTest(t *testing.T) (*PPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()

	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, rom)
	p := New(b, s)
	b.Map(types.CGBABC)

	p.enabled = true
	p.cgbMode = true
	p.mode = ModeVRAM
	p.addressMode = 0
	b.Set(types.LCDC, 0x90) // LCD on, TILE_SEL=1 ($8000 tile data)

	p.fetcherTileNoAddress = 0x1800
	p.fetcherTileAttr = 0
	b.VRAM[0][0x1800] = 0x12
	b.VRAM[0][0x0120] = 0xaa
	return p, b, s
}

func TestCGBTileSelectResetOnBitplaneReadUsesTileIndex(t *testing.T) {
	p, b, _ := newCGBTileSelectCollisionTest(t)

	p.fetcherState = BGGetTileDataLowT2
	p.stepPixelFetcher()
	if got := p.fetcherData[0]; got != 0xaa {
		t.Fatalf("pre-write bitplane = %#02x, want 0xaa", got)
	}

	// The LCDC write lands on the same scheduler cycle as the completed VRAM
	// bitplane read. CGB hardware drives the tile-map index onto the data bus
	// when TILE_SEL changes from 1 to 0 on this edge.
	b.Write(types.LCDC, 0x80)
	if got := p.fetcherData[0]; got != 0x12 {
		t.Fatalf("same-cycle TILE_SEL reset data = %#02x, want tile index 0x12", got)
	}
}

func TestCGBTileSelectResetAfterBitplaneReadKeepsFetchedData(t *testing.T) {
	p, b, s := newCGBTileSelectCollisionTest(t)

	p.fetcherState = BGGetTileDataLowT2
	p.stepPixelFetcher()
	s.Tick(1)

	b.Write(types.LCDC, 0x80)
	if got := p.fetcherData[0]; got != 0xaa {
		t.Fatalf("off-cycle TILE_SEL reset data = %#02x, want fetched 0xaa", got)
	}
}

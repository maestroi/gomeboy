package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newCGBTileSelectCollisionTest(t *testing.T) (*PPU, *gbio.Bus) {
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
	b.VRAM[0][0x0121] = 0x5a
	return p, b
}

func TestCGBTileSelectResetGlitchesNextHighBitplaneFetch(t *testing.T) {
	p, b := newCGBTileSelectCollisionTest(t)

	// T1 selects the high-bitplane fetch. The CPU write becomes visible at
	// the end of its machine cycle, so the gomeboy scheduler carries this
	// same-edge conflict into the following high-data fetch step.
	p.fetcherState = BGGetTileDataHighT1
	p.stepPixelFetcher()
	b.Write(types.LCDC, 0x80)
	p.stepPixelFetcher()

	if got := p.fetcherData[1]; got != 0x12 {
		t.Fatalf("TILE_SEL reset high bitplane = %#02x, want tile index 0x12", got)
	}
	if p.tileSelectGlitch {
		t.Fatal("TILE_SEL conflict remained pending after high-bitplane fetch")
	}
}

func TestCGBTileSelectUnchangedKeepsHighBitplaneData(t *testing.T) {
	p, b := newCGBTileSelectCollisionTest(t)

	p.fetcherState = BGGetTileDataHighT1
	p.stepPixelFetcher()
	b.Write(types.LCDC, 0x90)
	p.stepPixelFetcher()

	if got := p.fetcherData[1]; got != 0x5a {
		t.Fatalf("unchanged TILE_SEL high bitplane = %#02x, want VRAM data 0x5a", got)
	}
}

func TestCGBTileSelectResetDoesNotSubstituteSignedTileID(t *testing.T) {
	p, b := newCGBTileSelectCollisionTest(t)

	b.VRAM[0][0x1800] = 0x92
	b.VRAM[0][0x0921] = 0x66

	p.fetcherState = BGGetTileDataHighT1
	p.stepPixelFetcher()
	b.Write(types.LCDC, 0x80)
	p.stepPixelFetcher()

	if got := p.fetcherData[1]; got != 0x66 {
		t.Fatalf("signed tile high bitplane = %#02x, want VRAM data 0x66", got)
	}
	if p.tileSelectGlitch {
		t.Fatal("TILE_SEL conflict remained pending after signed-tile high fetch")
	}
}

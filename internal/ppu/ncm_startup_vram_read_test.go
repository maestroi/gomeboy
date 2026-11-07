package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// NCM uses the physical CGB VRAM read port during the short LCD-enabled
// startup scanline, despite using a DMG cartridge and background fetcher.
func TestNCMStartupVRAMReadBoundary(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
			b.Map(model)
			p := New(b, s)
			b.Set(0x8000, 0x5a)
			b.Write(types.LCDC, types.Bit7)
			if p.cgbMode || !p.enabled {
				t.Fatalf("fixture not in NCM: colour=%t enabled=%t", p.cgbMode, p.enabled)
			}

			// One initial PPU dot, then 76 scan dots + 2 more to
			// the early Mode-3 handover: T79 after LCDC enable.
			s.Tick(79)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("NCM read closed too early at T79: %#02x", got)
			}
			b.Write(0x8000, 0xaa)
			if got := b.Get(0x8000); got != 0x5a {
				t.Fatalf("NCM VRAM write should already be blocked: %#02x", got)
			}
			s.Tick(1)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("NCM read closed before the next dot: %#02x", got)
			}
			s.Tick(1) // read-port ownership changes at T81
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("NCM read port failed to close at T81: %#02x", got)
			}
			s.Tick(3) // pixel fetch begins at T84
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("NCM VRAM unexpectedly open during pixel fetch: %#02x", got)
			}
		})
	}
}

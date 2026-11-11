package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// TestLCDEnableBusLocks covers the shortened first scanline after LCDC.7 is
// enabled. On DMG-compatible startup the PPU acquires the OAM write bus
// before the OAM read bus and subsequently closes VRAM. These are observable
// CPU access boundaries, not merely changes in the STAT mode value.
//
// Keep this test independent of experimental AGE timing predicates: it
// exercises the existing glitched-line events and actual bus read/write API.
func TestLCDEnableBusLocks(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			p := New(b, s)

			b.Set(0x8000, 0x5a)
			b.Set(0xfe00, 0x66)
			b.Write(types.LCDC, types.Bit7)
			if !p.enabled {
				t.Fatal("LCD did not enable")
			}
			if got := b.Read(0xfe00); got != 0x66 {
				t.Fatalf("OAM read was blocked before the startup scan: got %#02x", got)
			}

			// The fixture has no CGB cartridge, so LCD startup uses the
			// DMG-compatible path: one initial dot plus 76 scan dots.
			s.Tick(77)
			b.Write(0xfe00, 0x77)
			if got := b.Get(0xfe00); got != 0x66 {
				t.Fatalf("OAM write bypassed the startup write lock: got %#02x", got)
			}
			if got := b.Read(0xfe00); got != 0x66 {
				t.Fatalf("OAM read closed at the earlier write-only edge: got %#02x", got)
			}

			s.Tick(2)
			if got := b.Read(0xfe00); got != 0xff {
				t.Fatalf("OAM read lock missing at startup Mode-3 edge: got %#02x", got)
			}
			// On a physical CGB the CPU VRAM read bus stays available
			// across the early Mode-3 handover even in DMG compatibility
			// mode; the VRAM write bus still closes at this boundary.
			want := byte(0xff)
			if model.IsCGB() {
				want = 0x5a
			}
			if got := b.Read(0x8000); got != want {
				t.Fatalf("VRAM read access at startup Mode-3 edge: got %#02x, want %#02x", got, want)
			}
			b.Write(0x8000, 0xaa)
			if got := b.Get(0x8000); got != 0x5a {
				t.Fatalf("VRAM write bypassed startup write lock: got %#02x", got)
			}
			s.Tick(5)
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM reopened during startup pixel transfer: got %#02x", got)
			}

			// LCD disable releases both buses immediately, regardless of
			// the point within the shortened first scanline.
			b.Write(types.LCDC, 0)
			if p.enabled {
				t.Fatal("LCD did not disable")
			}
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM remained locked after LCD disable: got %#02x", got)
			}
			b.Write(0xfe00, 0x88)
			if got := b.Read(0xfe00); got != 0x88 {
				t.Fatalf("OAM remained locked after LCD disable: got %#02x", got)
			}
		})
	}
}

func TestCGBCartridgeDoubleSpeedStartupVRAMReadEdge(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			rom := make([]byte, 0x8000)
			rom[0x143] = 0x80 // CGB-capable cartridge
			b := gbio.NewBus(s, rom)
			b.Map(model)
			p := New(b, s)
			s.ChangeSpeed(true)
			b.Set(0x8000, 0x5a)
			b.Set(0xfe00, 0x66)

			b.Write(types.LCDC, types.Bit7)
			if !p.enabled || !p.cgbMode {
				t.Fatalf("expected CGB colour-mode LCD startup, enabled=%t CGB=%t", p.enabled, p.cgbMode)
			}
			// CGB colour mode starts directly at dot zero. OAM and VRAM
			// writes lock at dot 78. VRAM reads remain available until
			// the separate double-speed CPU read edge at dot 80.
			s.Tick(156) // 78 PPU dots at double speed
			// Both physical CGB revision families sample CPU OAM reads
			// during the first dot-78 handover; OAM writes and VRAM
			// writes are still locked by their independent bus phases.
			if got := b.Read(0xfe00); got != 0x66 {
				t.Fatalf("OAM read at early startup edge: got %02x, want 66", got)
			}
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM should remain readable at early CGB Mode-3 edge, got %02x", got)
			}
			b.Write(0x8000, 0x77)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM write should already be blocked at dot 78, got %02x", got)
			}
			s.Tick(2) // dot 79: native CGB OAM CPU read grace has expired
			if got := b.Read(0xfe00); got != 0xff {
				t.Fatalf("OAM read stayed open into pixel transfer: %02x", got)
			}
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM read closed before dot 80, got %02x", got)
			}
			s.Tick(2) // dot 80
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM read should close at dot 80, got %02x", got)
			}
			s.Tick(6) // dot 83
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM must stay closed through startup fetch, got %02x", got)
			}
		})
	}
}

func TestCGBDMGCompatibilityDoubleSpeedStartupStillLocksVRAMEarly(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
	b.Map(types.CGBBC)
	p := New(b, s)
	s.ChangeSpeed(true)
	b.Set(0x8000, 0x5a)
	b.Write(types.LCDC, types.Bit7)
	if !p.enabled || p.cgbMode {
		t.Fatalf("expected DMG compatibility on physical CGB")
	}
	s.Tick(158) // one initial PPU dot, then 78 more in double speed
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("DMG-compatibility VRAM lock should remain at early edge, got %02x", got)
	}
}

package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Native-colour CGB D/E at double speed reserves the CPU OAM read port
// just before Mode 2 begins scanning, while B/C waits until its first
// OAM-scan event. The early CPU-read closure must not change the actual
// PPU OAM lock, CPU OAM writes or the preceding HBlank release.
func TestCGBDENativeDoubleSpeedOAMReadBeforeMode2(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, scx := range []uint8{0, 2, 4, 6} {
			t.Run(fmt.Sprintf("%s/scx=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				rom[0x143] = 0x80
				b := gbio.NewBus(s, rom)
				b.Map(model)
				p := New(b, s)
				s.ChangeSpeed(true)
				b.Set(0xfe00, 0x5a)
				b.Set(0x8000, 0x66)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if !p.enabled || !p.cgbMode {
					t.Fatal("fixture must be native CGB colour double speed")
				}

				for _, line := range []uint8{0, 1} {
					// First HBlank event releases both coarse OAM and
					// VRAM locks normally on both hardware revisions.
					found := false
					for ticks := 0; ticks < 2200; ticks++ {
						s.Tick(1)
						if p.ly == line && p.lineState == HBlankUpdateLY &&
							(line == 0 && p.lx == 0 || line == 1 && p.lx == 168) {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("line %d did not reach HBlank", line)
					}
					if got := b.Read(0x8000); got != 0x66 {
						t.Fatalf("line %d VRAM read locked after HBlank: %02x", line, got)
					}
					b.Write(0xfe00, 0x7c)
					if got := b.Get(0xfe00); got != 0x7c {
						t.Fatalf("line %d OAM write locked after HBlank: %02x", line, got)
					}
					for clock := 0; clock < 2; clock++ {
						if got := b.Read(0xfe00); got != 0x7c {
							t.Fatalf("line %d HBlank clock %d OAM read: %02x", line, clock, got)
						}
						s.Tick(1)
					}

					// The final HBlank dot advertises the next Mode 2
					// before the next scheduled per-entry OAM scan.
					found = false
					for ticks := 0; ticks < 1000; ticks++ {
						s.Tick(1)
						if p.lineState == StartOAMScan && p.oamScanIndex == 0 &&
							p.ly == line+1 {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("line %d never reached pre-Mode-2 read handover", line)
					}
					want := byte(0x7c)
					if model == types.CGBDE {
						want = 0xff
					}
					if got := b.Read(0xfe00); got != want {
						t.Fatalf("line %d pre-Mode-2 OAM read got %02x want %02x",
							line, got, want)
					}
					b.Write(0xfe00, 0x3d)
					if got := b.Get(0xfe00); got != 0x3d {
						t.Fatalf("line %d CPU OAM write locked before scan: %02x", line, got)
					}
					s.Tick(1) // real OAM scan acquires the coarse bus
					if got := b.Read(0xfe00); got != 0xff {
						t.Fatalf("line %d CPU OAM read not locked by scan: %02x", line, got)
					}
				}
			})
		}
	}
}

func TestCGBDENativeDoubleSpeedEarlyMode2ReadClearedOnLCDDisable(t *testing.T) {
	s := scheduler.NewScheduler()
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	b := gbio.NewBus(s, rom)
	b.Map(types.CGBDE)
	p := New(b, s)
	s.ChangeSpeed(true)
	b.Set(0xfe00, 0x55)
	b.Write(types.LCDC, types.Bit7)
	found := false
	for clock := 0; clock < 1300; clock++ {
		s.Tick(1)
		if p.lineState == StartOAMScan && p.oamScanIndex == 0 &&
			p.ly == 1 && p.glitchedLineState == GlitchedLineStartPixelTransfer {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("D/E double-speed Mode-2 read handover not reached")
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("early Mode-2 CPU OAM read should be blocked: %02x", got)
	}
	b.Write(types.LCDC, 0)
	if p.enabled {
		t.Fatal("LCD remained enabled")
	}
	if got := b.Read(0xfe00); got != 0x55 {
		t.Fatalf("LCD disable retained early Mode-2 OAM read lock: %02x", got)
	}
}

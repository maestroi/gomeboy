package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The CGB D/E double-speed CPU OAM read port remains PPU-owned for the
// first two CPU T-states after the HBlank event releases the coarse OAM
// lock. This is independent of OAM writes and VRAM reads. Exercise both
// the abbreviated LCD-on scanline and an actual visible scanline.
func TestCGBDENativeDoubleSpeedOAMReadHBlankRelease(t *testing.T) {
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
					found := false
					for ticks := 0; ticks < 2000; ticks++ {
						s.Tick(1)
						if p.ly != line || p.lineState != HBlankUpdateLY {
							continue
						}
						if line == 0 && p.lx == 0 || line == 1 && p.lx == 168 {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("line %d never reached scheduled HBlank unlock", line)
					}
					if got := b.Read(0x8000); got != 0x66 {
						t.Fatalf("line %d VRAM read held past HBlank: %02x", line, got)
					}
					b.Write(0xfe00, 0x7c)
					if got := b.Get(0xfe00); got != 0x7c {
						t.Fatalf("line %d OAM writes blocked past HBlank: %02x", line, got)
					}
					want := byte(0x7c)
					if model == types.CGBDE {
						want = 0xff
					}
					for clock := 0; clock < 2; clock++ {
						if got := b.Read(0xfe00); got != want {
							t.Fatalf("line %d clock %d OAM read: got %02x want %02x",
								line, clock, got, want)
						}
						s.Tick(1)
					}
					if got := b.Read(0xfe00); got != 0x7c {
						t.Fatalf("line %d OAM read not released after one PPU dot: %02x", line, got)
					}
					// The read-only delayed release cannot reopen OAM
					// during the next Mode-2 scan.
				}
			})
		}
	}
}

func TestCGBDENativeDoubleSpeedOAMReadHoldClearedOnLCDDisable(t *testing.T) {
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
	for ticks := 0; ticks < 1000; ticks++ {
		s.Tick(1)
		if p.ly == 0 && p.lx == 0 && p.lineState == HBlankUpdateLY {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("first CGB D/E double-speed HBlank never reached")
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("OAM should be read-blocked at HBlank handover: %02x", got)
	}
	b.Write(types.LCDC, 0)
	if p.enabled {
		t.Fatal("LCD remained enabled")
	}
	if got := b.Read(0xfe00); got != 0x55 {
		t.Fatalf("LCD disable retained OAM read hold: %02x", got)
	}
}

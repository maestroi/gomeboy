package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The shortened first native-colour scanline has a physical revision
// difference: CGB D/E keeps CPU OAM reads blocked for one dot, or
// two dots at fine-scroll residues 1/5, after the HBlank bus unlock.
// CGB B/C does not. Check the real
// scheduled event for all SCX phases without changing OAM writes or VRAM.
func TestCGBDENativeColourFirstHBlankOAMReadRelease(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/scx=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				rom[0x143] = 0x80 // native CGB colour
				b := gbio.NewBus(s, rom)
				b.Map(model)
				p := New(b, s)
				b.Set(0xfe00, 0x5a)
				b.Set(0x8000, 0x66)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if !p.enabled || !p.cgbMode {
					t.Fatal("fixture must run in native CGB colour mode")
				}

				// Detect the first real HBlank unlock event rather than
				// assuming the same startup duration at every SCX.
				found := false
				for dot := 0; dot < 460; dot++ {
					s.Tick(1)
					if p.ly == 0 && p.lx == 0 &&
						p.lineState == HBlankUpdateLY &&
						p.glitchedLineState == GlitchedLineStartPixelTransfer {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("native CGB startup never reached first HBlank unlock")
				}
				if got := b.Read(0x8000); got != 0x66 {
					t.Fatalf("VRAM CPU read was delayed with OAM: %02x", got)
				}
				b.Write(0xfe00, 0x7c)
				if got := b.Get(0xfe00); got != 0x7c {
					t.Fatalf("OAM CPU write remained locked at HBlank: %02x", got)
				}
				want := byte(0x7c)
				if model == types.CGBDE {
					want = 0xff
				}
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("CPU OAM read on HBlank edge: got %02x want %02x", got, want)
				}
				s.Tick(1)
				want = 0x7c
				if model == types.CGBDE && scx&3 == 1 {
					want = 0xff
				}
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("CPU OAM read one dot after HBlank: got %02x want %02x", got, want)
				}
				s.Tick(1)
				if got := b.Read(0xfe00); got != 0x7c {
					t.Fatalf("CPU OAM read not open two dots after HBlank: %02x", got)
				}
				// The absolute read-release clock cannot be moved by
				// a later fine-scroll write, or survive LCD disable.
				b.Write(types.SCX, scx^7)
				if got := b.Read(0xfe00); got != 0x7c {
					t.Fatalf("later SCX write changed OAM access: %02x", got)
				}
			})
		}
	}
}

func TestCGBDENativeOAMStartupReadHoldClearedOnLCDDisable(t *testing.T) {
	s := scheduler.NewScheduler()
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	b := gbio.NewBus(s, rom)
	b.Map(types.CGBDE)
	p := New(b, s)
	b.Set(0xfe00, 0x3c)
	b.Write(types.LCDC, types.Bit7)
	found := false
	for clock := 0; clock < 460; clock++ {
		s.Tick(1)
		if p.ly == 0 && p.lineState == HBlankUpdateLY &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("native colour first HBlank never began")
	}
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("D/E first HBlank CPU OAM read should still be locked: %02x", got)
	}
	b.Write(types.LCDC, 0)
	if p.enabled {
		t.Fatal("LCD did not disable")
	}
	if got := b.Read(0xfe00); got != 0x3c {
		t.Fatalf("LCD disable retained CGB D/E CPU OAM read lock: %02x", got)
	}
}

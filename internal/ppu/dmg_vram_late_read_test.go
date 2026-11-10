package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The DMG-C CPU VRAM port can reopen between the completed final
// pixel and the scheduled HBlank bus unlock, but only at fine-scroll
// residues 3 and 7. Other models, writes and OAM keep their own edges.
func TestDMGLastPixelVRAMReadOnlyHandover(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.MGB, types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/scx=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.mode = ModeVRAM
				p.scx = scx
				p.cgbMode = false
				p.lineState = PixelTransferLX
				p.lx = 167
				b.Set(0x8000, 0x5a)
				b.Set(0xfe00, 0x66)
				b.Lock(gbio.OAM | gbio.VRAM)
				p.bgFIFO.Size = 8

				// An in-flight/buffered last pixel is *not* enough to
				// release DMG-C VRAM (prior experimental candidate).
				want := byte(0xff)
				if model.IsCGB() {
					want = 0x5a // physical CGB in NCM has its own edge
				}
				if got := b.Read(0x8000); got != want {
					t.Fatalf("pre-final-pixel read=%02x want=%02x", got, want)
				}

				p.lx = 168
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("last LX before HBlank state unexpectedly opened VRAM: %02x", got)
				}

				p.lineState = EnterHBlank
				want = 0xff
				if model.IsCGB() || model == types.DMGABC && scx&3 == 3 {
					want = 0x5a
				}
				if got := b.Read(0x8000); got != want {
					t.Fatalf("completed-pixel VRAM read=%02x want=%02x", got, want)
				}
				b.Write(0x8000, 0xaa)
				if got := b.Get(0x8000); got != 0x5a {
					t.Fatalf("VRAM write bypassed lock: %02x", got)
				}

				if (model == types.DMGABC || model == types.CGBBC && !p.cgbMode) && scx&3 == 3 {
					if got := b.Read(0xfe00); got != 0x66 {
						t.Fatalf("DMG/CGB B/C compatibility OAM read phase changed: %02x", got)
					}
				} else if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("OAM read unexpectedly opened: %02x", got)
				}
				b.Write(0xfe00, 0xaa)
				if got := b.Get(0xfe00); got != 0x66 {
					t.Fatalf("OAM write bypassed lock: %02x", got)
				}

				p.lx = 0 // first LCD-on line is a dummy transfer
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("LCD startup VRAM lock bypassed: %02x", got)
				}
			})
		}
	}
}

func TestDMGLastPixelVRAMReadOnRealScanline(t *testing.T) {
	for _, scx := range []uint8{0, 2, 3, 6, 7} {
		t.Run(fmt.Sprintf("scx=%d", scx), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(types.DMGABC)
			p := New(b, s)
			b.Set(0x8000, 0x5a)
			b.Write(types.SCX, scx)
			b.Write(types.LCDC, types.Bit7)

			// Startup does not draw pixels. Inspect the first ordinary
			// scanline before EnterHBlank releases the coarse bus locks.
			found := false
			for i := 0; i < 1400; i++ {
				s.Tick(1)
				if p.ly == 1 && p.lineState == EnterHBlank && p.lx == 168 {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("ordinary DMG scanline did not reach last-pixel handover")
			}
			want := byte(0xff)
			if scx&3 == 3 {
				want = 0x5a
			}
			if got := b.Read(0x8000); got != want {
				t.Fatalf("CPU VRAM read before HBlank bus release: got %02x want %02x", got, want)
			}
			b.Write(0x8000, 0xaa)
			if got := b.Get(0x8000); got != 0x5a {
				t.Fatalf("VRAM write opened too early: %02x", got)
			}
			s.Tick(p.enterHBlankDelay())
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM did not open normally in HBlank: %02x", got)
			}
		})
	}
}

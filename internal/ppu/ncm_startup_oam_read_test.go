package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// NCM uses the DMG pixel fetcher on physical CGB hardware. On the
// abbreviated LCD-enable line, D/E retains the CPU OAM read port for
// two dots longer than B/C, without delaying VRAM reads or OAM writes.
// Exercise the actual scheduled startup transitions (not a forged PPU state).
func TestCGBRevisionNCMStartupOAMReadRelease(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only ROM
				b.Map(model)
				p := New(b, s)
				b.Set(0xfe00, 0x5a)
				b.Set(0x8000, 0x66)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if !p.enabled || p.cgbMode {
					t.Fatal("fixture must be NCM on physical CGB")
				}

				// The first dummy line's existing HBlank event starts
				// at T250+SCX. This is the VRAM release edge, and it
				// also makes OAM *writes* accessible.
				openClock := uint64(250) + uint64(scx)
				s.Tick(openClock - 1)
				if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("OAM read opened before startup HBlank: %02x", got)
				}
				s.Tick(1)
				if got := b.Read(0x8000); got != 0x66 {
					t.Fatalf("VRAM release moved with OAM read: %02x", got)
				}
				b.Write(0xfe00, 0x7c)
				if got := b.Get(0xfe00); got != 0x7c {
					t.Fatalf("OAM write stayed blocked at HBlank: %02x", got)
				}

				want := byte(0x7c)
				if model == types.CGBDE {
					want = 0xff
				}
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("OAM read at startup HBlank got %02x want %02x", got, want)
				}

				// The extra dot is a distinct read-only ownership
				// interval on D/E; CGB B/C reads are already open.
				s.Tick(1)
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("OAM read one dot after HBlank got %02x want %02x", got, want)
				}
				s.Tick(1)
				if got := b.Read(0xfe00); got != 0x7c {
					t.Fatalf("OAM read did not reopen two dots later: %02x", got)
				}

				// A later SCX write cannot postpone the already
				// established bus handover.
				b.Write(types.SCX, scx^7)
				if got := b.Read(0xfe00); got != 0x7c {
					t.Fatalf("post-handover SCX write changed OAM ownership: %02x", got)
				}

				// OAM access is locked again by the next real scan,
				// regardless of the first-line read-release latch.
				s.Tick(460 - s.Cycle())
				if p.ly != 1 || p.lineState != StartOAMScan {
					t.Fatalf("normal scan did not start: LY=%d state=%s", p.ly, p.lineState)
				}
				if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("startup hold incorrectly unlocked real OAM scan: %02x", got)
				}
			})
		}
	}
}

func TestCGBDENCMStartupOAMReadHoldDoesNotOverrideLCDDisable(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBDE)
	p := New(b, s)
	b.Set(0xfe00, 0x39)
	b.Write(types.LCDC, types.Bit7)
	s.Tick(250)
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("D/E OAM should still be blocked in first HBlank: %02x", got)
	}
	b.Write(types.LCDC, 0)
	if p.enabled {
		t.Fatal("LCD did not turn off")
	}
	if got := b.Read(0xfe00); got != 0x39 {
		t.Fatalf("OAM stayed blocked after disabling LCD: %02x", got)
	}
}

package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// This is a real scheduler-driven line rather than a manufactured LX state:
// DMG-C and physical CGB B/C in either colour or compatibility mode
// release CPU OAM reads after the last pixel at fine-scroll phases 3/7.
// The CGB compatibility-mode write port has its own final-pixel SCX 3/7
// handover; DMG and MGB writes remain locked here. D/E reads stay locked.
func TestCGBBCNCMOAMReadAtEndOfRealScanline(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		colour bool
	}{
		{types.DMGABC, false},
		{types.MGB, false},
		{types.CGBBC, false},
		{types.CGBDE, false},
		{types.CGBBC, true},
		{types.CGBDE, true},
	} {
		for _, scx := range []uint8{0, 2, 3, 6, 7} {
			name := fmt.Sprintf("%s/colour=%t/scx=%d", tc.model, tc.colour, scx)
			t.Run(name, func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				if tc.colour {
					rom[0x143] = 0x80
				}
				b := gbio.NewBus(s, rom)
				b.Map(tc.model)
				p := New(b, s)
				if p.cgbMode {
					t.Fatal("LCD not yet enabled; colour mode should not be latched")
				}
				b.Set(0xfe00, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if p.cgbMode != (tc.colour && tc.model.IsCGB()) {
					t.Fatalf("unexpected PPU colour mode: %t", p.cgbMode)
				}

				// Skip the shortened startup line and stop just after the
				// first ordinary visible line outputs pixel 160.
				found := false
				for i := 0; i < 1500; i++ {
					s.Tick(1)
					if p.ly == 1 && p.lineState == EnterHBlank && p.lx == 168 {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("ordinary PPU scanline never reached last-pixel handover")
				}

				want := byte(0xff)
				if scx&3 == 3 && (tc.model == types.DMGABC ||
					tc.model == types.CGBBC) {
					want = 0x5a
				}
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("final-pixel CPU OAM read: got %02x want %02x", got, want)
				}
				b.Write(0xfe00, 0x99)
				wantOAM := byte(0x5a)
				if tc.model.IsCGB() &&
					(tc.colour && scx&2 != 0 || !tc.colour && scx&3 == 3) {
					wantOAM = 0x99
				}
				if got := b.Get(0xfe00); got != wantOAM {
					t.Fatalf("completed-pixel OAM write: got %02x want %02x", got, wantOAM)
				}
				s.Tick(p.enterHBlankDelay())
				if got := b.Read(0xfe00); got != wantOAM {
					t.Fatalf("ordinary HBlank CPU OAM read got %02x want %02x", got, wantOAM)
				}
			})
		}
	}
}

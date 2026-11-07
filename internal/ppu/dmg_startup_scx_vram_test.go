package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The first LCD-enabled scanline does not render pixels, but still consumes
// the SCX fine-scroll discard phase before its VRAM bus becomes accessible.
// Keep this independent of CGB-colour and NCM (physical-CGB) release edges.
func TestDMGStartupHBlankVRAMReleaseAccountsForSCX(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.MGB} {
		for scx := uint8(0); scx < 8; scx++ {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				b.Set(0x8000, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)
				if p.cgbMode || !p.enabled {
					t.Fatal("expected LCD-enabled DMG hardware")
				}
				// DMG startup begins one dot after LCDC is enabled.
				// The short dummy line's HBlank event is at T252+SCX.
				openClock := uint64(252) + uint64(scx)
				s.Tick(openClock - 1)
				if got := b.Read(0x8000); got != 0xff {
					t.Fatalf("VRAM read reopened early at T%d: got %02x", openClock-1, got)
				}
				s.Tick(1)
				if got := b.Read(0x8000); got != 0x5a {
					t.Fatalf("VRAM read still blocked after SCX startup transfer: got %02x", got)
				}
			})
		}
	}
}

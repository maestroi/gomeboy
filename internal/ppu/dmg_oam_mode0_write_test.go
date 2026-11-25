package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// DMG/MGB reopen the CPU OAM write port when the final Mode-3 pixel is
// pushed, one enterHBlankDelay dot before EnterHBlank's coarse Unlock.
// SameBoy clears oam_write_blocked on that same non-double-speed edge.
// AGE oam-write-dmgC probe 2/3 Mode-0 samples depend on it.
func TestDMGOAMWriteOpensBeforeCoarseHBlankUnlock(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.MGB} {
		for _, scx := range []uint8{0, 3, 7} {
			t.Run(fmt.Sprintf("%s/scx=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				b.Set(0xfe00, 0x5a)
				b.Write(types.SCX, scx)
				b.Write(types.LCDC, types.Bit7)

				found := false
				for i := 0; i < 1500; i++ {
					s.Tick(1)
					if p.ly == 1 && p.lineState == EnterHBlank && p.lx == 168 {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("ordinary scanline did not reach last-pixel handover")
				}

				b.Write(0xfe00, 0x99)
				if got := b.Get(0xfe00); got != 0x99 {
					t.Fatalf("OAM write before coarse Unlock: got %02x want 99", got)
				}

				// VRAM must stay locked until the coarse HBlank unlock.
				b.Set(0x8000, 0x3c)
				b.Write(0x8000, 0xaa)
				if got := b.Get(0x8000); got != 0x3c {
					t.Fatalf("VRAM write opened too early: %02x", got)
				}

				s.Tick(p.enterHBlankDelay())
				b.Write(0xfe00, 0x77)
				if got := b.Get(0xfe00); got != 0x77 {
					t.Fatalf("OAM write still blocked in HBlank: %02x", got)
				}
			})
		}
	}
}

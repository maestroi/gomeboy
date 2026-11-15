package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The physical CGB write port (unlike its read port) can close before the
// coarse OAM scan lock. Test both colour- and DMG-compatibility CPU modes,
// normal/double speeds, and surrounding scheduler phases independently.
func TestCGBNCMOAMWriteBeforeMode2(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		for _, colour := range []bool{false, true} {
			for _, doubleSpeed := range []bool{false, true} {
				for _, state := range []LineState{HBlankUpdateVisibleLY, HBlankEnd, StartOAMScan} {
					name := fmt.Sprintf("%s/colour=%t/double=%t/state=%d", model, colour, doubleSpeed, state)
					t.Run(name, func(t *testing.T) {
						s := scheduler.NewScheduler()
						b := gbio.NewBus(s, make([]byte, 0x8000))
						b.Map(model)
						p := New(b, s)
						s.ChangeSpeed(doubleSpeed)
						p.enabled = true
						p.cgbMode = colour
						p.glitchedLineState = GlitchedLineStartPixelTransfer
						p.lineState = state
						p.lx = 168 // completed ordinary visible scanline
						p.oamScanIndex = 0
						b.Set(0xfe00, 0x5a)
						b.Write(0xfe00, 0x7b)
						want := byte(0x7b)
						if (model.IsCGB() &&
							(colour && doubleSpeed || !colour && !doubleSpeed) ||
							model == types.DMGABC && !doubleSpeed) &&
							(state == HBlankEnd || state == StartOAMScan) {
							want = 0x5a
						}
						if got := b.Get(0xfe00); got != want {
							t.Fatalf("CPU OAM write got %02x want %02x", got, want)
						}
					})
				}
			}
		}
	}
}

// The compatibility-mode read/write handover is an actual PPU event,
// not a synthetic early-release shortcut or an LCD-startup lock.
func TestCGBNCMOAMWriteMode2RealScanline(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
			b.Map(model)
			p := New(b, s)
			b.Set(0xfe00, 0x5a)
			b.Write(types.LCDC, types.Bit7)
			if !p.enabled || p.cgbMode {
				t.Fatal("fixture requires physical CGB in DMG compatibility mode")
			}
			advance := func(state LineState) {
				t.Helper()
				for clocks := 0; clocks < 1500; clocks++ {
					if p.ly == 1 && p.lineState == state &&
						p.glitchedLineState == GlitchedLineStartPixelTransfer {
						return
					}
					s.Tick(1)
				}
				t.Fatalf("line 1 did not reach PPU state %d", state)
			}
			advance(HBlankUpdateVisibleLY)
			b.Write(0xfe00, 0x7b)
			if got := b.Get(0xfe00); got != 0x7b {
				t.Fatalf("late HBlank write was prematurely blocked: %02x", got)
			}
			advance(HBlankEnd)
			b.Write(0xfe00, 0x6c)
			if got := b.Get(0xfe00); got != 0x7b {
				t.Fatalf("HBlankEnd write escaped CPU OAM port lock: %02x", got)
			}
			advance(StartOAMScan)
			b.Write(0xfe00, 0x3c)
			if got := b.Get(0xfe00); got != 0x7b {
				t.Fatalf("pre-Mode-2 write escaped CPU OAM port lock: %02x", got)
			}
			b.Write(types.LCDC, 0)
			b.Write(0xfe00, 0x55)
			if got := b.Get(0xfe00); got != 0x55 {
				t.Fatalf("LCD-off CPU OAM writes remained locked: %02x", got)
			}
		})
	}
}

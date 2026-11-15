package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Physical DMG-C closes the CPU-facing OAM write port before the next
// Mode-2 scan takes the PPU's coarse OAM bus. The Pocket retains its
// independently measured timing. Neither path changes CPU read locks.
func TestDMGCOAMWriteBeforeMode2(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.MGB, types.CGBBC, types.CGBDE} {
		for _, state := range []LineState{HBlankUpdateVisibleLY, HBlankEnd, StartOAMScan} {
			t.Run(fmt.Sprintf("%s/state=%d", model, state), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = false
				p.glitchedLineState = GlitchedLineStartPixelTransfer
				p.lineState = state
				p.lx = 168 // completed ordinary visible scanline
				p.oamScanIndex = 0
				b.Set(0xfe00, 0x5a)
				b.Write(0xfe00, 0x7b)
				want := byte(0x7b)
				if model != types.MGB && (state == HBlankEnd || state == StartOAMScan) {
					want = 0x5a
				}
				if got := b.Get(0xfe00); got != want {
					t.Fatalf("OAM write state %d on %s got %02x want %02x",
						state, model, got, want)
				}
			})
		}
	}
}

func TestDMGCOAMWriteMode2RealScanline(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	p := New(b, s)
	b.Set(0xfe00, 0x5a)
	b.Write(types.LCDC, types.Bit7)

	// The shortened LCD-enable line must preserve its own write
	// handover: this rule applies only after a complete visible line.
	for clock := 0; clock < 600; clock++ {
		if p.ly == 1 && p.lineState == HBlankEnd {
			b.Write(0xfe00, 0x66)
			if got := b.Get(0xfe00); got != 0x66 {
				t.Fatalf("DMG startup HBlank write closed prematurely: %02x", got)
			}
			break
		}
		s.Tick(1)
	}
	advance := func(state LineState) {
		t.Helper()
		for clocks := 0; clocks < 1600; clocks++ {
			if p.ly == 2 && p.lineState == state &&
				p.glitchedLineState == GlitchedLineStartPixelTransfer {
				return
			}
			s.Tick(1)
		}
		t.Fatalf("line 2 did not reach PPU state %d", state)
	}
	advance(HBlankUpdateVisibleLY)
	b.Write(0xfe00, 0x7b)
	if got := b.Get(0xfe00); got != 0x7b {
		t.Fatalf("late HBlank OAM write was blocked: %02x", got)
	}
	advance(HBlankEnd)
	b.Write(0xfe00, 0x6c)
	if got := b.Get(0xfe00); got != 0x7b {
		t.Fatalf("HBlankEnd OAM write escaped early lock: %02x", got)
	}
	advance(StartOAMScan)
	b.Write(0xfe00, 0x3c)
	if got := b.Get(0xfe00); got != 0x7b {
		t.Fatalf("pre-Mode-2 OAM write escaped early lock: %02x", got)
	}
	b.Write(types.LCDC, 0)
	b.Write(0xfe00, 0x55)
	if got := b.Get(0xfe00); got != 0x55 {
		t.Fatalf("LCD-off OAM write remained blocked: %02x", got)
	}
}

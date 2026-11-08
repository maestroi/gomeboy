package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBCartridgeDoubleSpeedStartupOAMWritePhase(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			rom := make([]byte, 0x8000)
			rom[0x143] = 0x80 // CGB-capable cartridge
			b := gbio.NewBus(s, rom)
			b.Map(model)
			p := New(b, s)
			s.ChangeSpeed(true)
			b.Set(0xfe00, 0x5a)
			b.Write(types.LCDC, types.Bit7)
			if !p.enabled || !p.cgbMode {
				t.Fatal("expected CGB-colour LCD startup")
			}

			s.Tick(151) // dot 75 + one CPU T-state
			b.Write(0xfe00, 0xa0)
			if got := b.Get(0xfe00); got != 0xa0 {
				t.Fatalf("OAM should still accept writes before lock: %02x", got)
			}
			s.Tick(1) // dot 76: PPU write lock acquired
			if p.glitchedLineState != GlitchedLineEndOAM {
				t.Fatalf("unexpected startup phase at dot 76: %v", p.glitchedLineState)
			}
			b.Write(0xfe00, 0xb0)
			if got := b.Get(0xfe00); got != 0xb0 {
				t.Fatalf("CPU OAM write blocked on first handover clock: %02x", got)
			}
			s.Tick(1) // second CPU T-state of dot 76
			b.Write(0xfe00, 0xc0)
			if got := b.Get(0xfe00); got != 0xc0 {
				t.Fatalf("CPU OAM write blocked too soon in startup grace: %02x", got)
			}
			s.Tick(1) // dot 77
			b.Write(0xfe00, 0xd0)
			if got := b.Get(0xfe00); got != 0xc0 {
				t.Fatalf("CPU OAM write lock was not restored after one PPU dot: %02x", got)
			}
			s.Tick(2) // dot 78, actual PPU Mode-3 OAM lock
			b.Write(0xfe00, 0xe0)
			if got := b.Get(0xfe00); got != 0xc0 {
				t.Fatalf("Mode-3 OAM write was incorrectly accepted: %02x", got)
			}
		})
	}
}

func TestStartupOAMWriteGraceIsCGBColourDoubleSpeedOnly(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		cgbColour bool
		doubleSpeed bool
	}{
		{types.DMGABC, false, false},
		{types.CGBBC, true, false},
		{types.CGBDE, true, false},
		{types.CGBBC, false, true},
		{types.CGBDE, false, true},
	} {
		t.Run(fmt.Sprintf("%s/colour=%t/double=%t",tc.model,tc.cgbColour,tc.doubleSpeed),func(t *testing.T){
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte,0x8000))
			b.Map(tc.model)
			p := New(b,s)
			if tc.doubleSpeed {s.ChangeSpeed(true)}
			p.enabled = true
			p.cgbMode = tc.cgbColour
			p.glitchedLineState = GlitchedLineEndOAM
			p.lineDot = s.Cycle()
			b.Set(0xfe00,0x5a)
			s.Tick(152)
			b.WLock(gbio.OAM)
			b.Write(0xfe00,0xa0)
			if got:=b.Get(0xfe00);got!=0x5a {
				t.Fatalf("unexpected startup OAM write in non-double-speed colour mode: %02x",got)
			}
		})
	}
}

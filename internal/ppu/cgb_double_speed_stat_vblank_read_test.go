package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Native double-speed CGB CPU STAT reads enter VBlank one PPU dot before
// the internal Mode-1/IRQ transition. This is only a read-port phase:
// the PPU's mode, IRQ source, and VBlank event stay at dot 5.
func TestDoubleSpeedCGBVBlankEntrySTATReadPort(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		colour bool
		double bool
		early bool
	}{
		{types.CGBBC, true, true, true},
		{types.CGBDE, true, true, true},
		{types.CGBBC, true, false, false},
		{types.CGBDE, true, false, false},
		{types.CGBBC, false, true, false},
		{types.DMGABC, false, false, false},
	} {
		t.Run(fmt.Sprintf("%s/colour=%t/double=%t", tc.model, tc.colour, tc.double),func(t *testing.T) {
			s:=scheduler.NewScheduler()
			b:=gbio.NewBus(s,make([]byte,0x8000))
			b.Map(tc.model)
			// The VBlank event raises an interrupt even in a headless PPU test.
			b.InterruptCallback = func(uint8) {}
			p:=New(b,s)
			p.enabled=true
			p.cgbMode=tc.colour
			p.ly=144
			p.mode,p.modeToInt=ModeHBlank,ModeHBlank
			p.offscreenLineState=StartVBlank
			if tc.double {s.ChangeSpeed(true)}
			p.handleOffscreenLine() // VBlank line dot 0

			clocksPerDot:=uint64(1)
			if tc.double {clocksPerDot=2}
			s.Tick(4*clocksPerDot-1)
			if got:=b.Read(types.STAT)&3; got!=ModeHBlank {
				t.Fatalf("read before dot 4 = %d, want Mode 0", got)
			}
			s.Tick(1) // dot 4, LYC compare, before PPU Mode 1
			want:=byte(ModeHBlank)
			if tc.early {want=ModeVBlank}
			if got:=b.Read(types.STAT)&3; got!=want {
				t.Fatalf("read at dot 4 = %d, want %d",got,want)
			}
			if p.mode!=ModeHBlank || p.modeToInt!=ModeHBlank {
				t.Fatalf("read port changed PPU/IRQ modes before dot 5: %d/%d",p.mode,p.modeToInt)
			}
			s.Tick(clocksPerDot) // dot 5, internal Mode 1 and VBlank IRQ
			if p.mode!=ModeVBlank || p.modeToInt!=ModeVBlank {
				t.Fatalf("PPU did not enter internal VBlank at dot 5: %d/%d",p.mode,p.modeToInt)
			}
			if got:=b.Read(types.STAT)&3;got!=ModeVBlank {
				t.Fatalf("CPU STAT after dot 5 = %d, want Mode 1",got)
			}
		})
	}
}

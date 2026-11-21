package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// On the first shortened line after enabling LCD, native CGB in double
// speed changes the internal mode to 3 at dot 78 but the CPU-facing STAT
// register retains Mode 0 through that dot. The normal-speed and NCM
// profiles retain their earlier CPU-visible transition.
func TestDoubleSpeedCGBStartupMode3STATReadHandover(t *testing.T) {
	for _, tc := range []struct {
		model  types.Model
		colour bool
		double bool
		early  bool
	}{
		{types.CGBBC, true, true, true},
		{types.CGBDE, true, true, true},
		{types.CGBBC, true, false, false},
		{types.CGBDE, true, false, false},
		{types.CGBBC, false, true, false},
		{types.CGBDE, false, true, false},
	} {
		for _, scx := range []byte{0, 7} {
			t.Run(fmt.Sprintf("%s/colour=%t/double=%t/SCX=%d",tc.model,tc.colour,tc.double,scx),func(t *testing.T){
				s:=scheduler.NewScheduler()
				b:=gbio.NewBus(s,make([]byte,0x8000))
				b.Map(tc.model)
				p:=New(b,s)
				p.enabled=true
				p.cgbMode=tc.colour
				if tc.double {s.ChangeSpeed(true)}
				b.Write(types.SCX,scx)
				p.glitchedLineState=StartGlitchedLine
				p.handleGlitchedLine0()

				clocksPerDot:=uint64(1)
				if tc.double {clocksPerDot=2}
				s.Tick(78*clocksPerDot-1)
				if got:=b.Read(types.STAT)&3;got!=ModeHBlank {
					t.Fatalf("before internal Mode 3 STAT=%d, want Mode 0",got)
				}
				s.Tick(1) // dot 78, internal pixel transfer takes Mode 3
				if p.mode!=ModeVRAM || p.modeToInt!=ModeVRAM {
					t.Fatalf("internal Mode 3 not entered at dot 78: %d/%d",p.mode,p.modeToInt)
				}
				want:=byte(ModeVRAM)
				if tc.early {want=ModeHBlank}
				if got:=b.Read(types.STAT)&3;got!=want {
					t.Fatalf("first CPU read at dot 78=%d, want %d",got,want)
				}
				if tc.double {
					s.Tick(1) // other CPU clock within dot 78
					if got:=b.Read(types.STAT)&3;got!=want {
						t.Fatalf("second CPU read at dot 78=%d, want %d",got,want)
					}
				}
				s.Tick(1) // dot 79, Mode 3 must be visible for all
				if got:=b.Read(types.STAT)&3;got!=ModeVRAM {
					t.Fatalf("CPU read at dot 79=%d, want Mode 3",got)
				}
				if p.mode!=ModeVRAM || p.modeToInt!=ModeVRAM {
					t.Fatalf("STAT read shifted internal PPU/IRQ mode: %d/%d",p.mode,p.modeToInt)
				}
			})
		}
	}
}

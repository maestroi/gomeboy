package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBClockedOAMWriteNormalSpeedSamplesBeforeFinalTState(t *testing.T) {
	for _,model:=range []types.Model{types.CGBBC,types.CGBDE,types.DMGABC} {
		for _,double:=range []bool{false,true} {
			s:=scheduler.NewScheduler()
			rom:=make([]byte,0x8000)
			rom[0x143]=0x80 // CGB-colour-capable cartridge
			b:=NewBus(s,rom)
			b.Map(model)
			if double {s.ChangeSpeed(true)}
			s.RegisterEvent(scheduler.TimerTIMAReload,func(){b.WLock(OAM)})
			s.ScheduleEvent(scheduler.TimerTIMAReload,4)
			start:=s.Cycle()
			b.ClockedWrite(0xfe00,0x5a)
			want:=byte(0)
			if model.IsCGB() && !double {want=0x5a}
			if got:=b.Get(0xfe00);got!=want {
				t.Fatalf("%s double=%t: OAM sample at T4 lock got %02x want %02x",model,double,got,want)
			}
			if s.Cycle()!=start+4 {t.Fatalf("CPU write must consume exactly 4 cycles")}
			b.ClockedWrite(0xfe00,0xa5)
			if got:=b.Get(0xfe00);got!=want {
				t.Fatalf("locked OAM write incorrectly committed: %02x",got)
			}
		}
	}
}


// CGB CPU write timing is a property of the hardware revision, not
// whether the inserted ROM requests CGB colour features. The next
// scheduler event locks OAM at T4: normal-speed CGB must commit at T3
// in both native and DMG-compatibility cartridge modes.
func TestCGBCompatibilityClockedOAMWriteSamplesBeforeFinalTState(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE, types.CGBABC} {
		for _, colour := range []bool{false, true} {
			for _, double := range []bool{false, true} {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				if colour {
					rom[0x143] = 0x80
				}
				b := NewBus(s, rom)
				b.Map(model)
				if double {
					s.ChangeSpeed(true)
				}
				s.RegisterEvent(scheduler.TimerTIMAReload, func() { b.WLock(OAM) })
				s.ScheduleEvent(scheduler.TimerTIMAReload, 4)

				b.ClockedWrite(0xfe00, 0x5a)
				want := byte(0)
				if !double {
					want = 0x5a
				}
				if got := b.Get(0xfe00); got != want {
					t.Fatalf("model=%s colour=%t double=%t: committed %02x, want %02x",
						model, colour, double, got, want)
				}
				if s.Cycle() != 4 {
					t.Fatalf("model=%s colour=%t double=%t: CPU write took %d clocks, want 4",
						model, colour, double, s.Cycle())
				}
				b.ClockedWrite(0xfe00, 0xa5)
				if got := b.Get(0xfe00); got != want {
					t.Fatalf("model=%s colour=%t double=%t: locked OAM write changed byte to %02x",
						model, colour, double, got)
				}
			}
		}
	}
}

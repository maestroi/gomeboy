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
			b:=NewBus(s,make([]byte,0x8000))
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

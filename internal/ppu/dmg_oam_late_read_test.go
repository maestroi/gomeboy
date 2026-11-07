package ppu

import (
	"testing"
	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)
func TestDMGOAMReadLateHBlankPhaseDoesNotUnlockWrites(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC,types.MGB,types.CGBBC,types.CGBDE} {
		for _, scx := range []uint8{0,3,7} {
			s:=scheduler.NewScheduler()
			b:=gbio.NewBus(s,make([]byte,0x8000))
			b.Map(model)
			p:=New(b,s)
			p.enabled=true
			p.mode=ModeVRAM
			p.lineState=PixelTransferLX
			p.lx=167
			p.scx=scx
			b.Set(0xfe00,0x5a)
			b.Lock(gbio.OAM)
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("%s SCX=%d read before final-pixel event: %02x",model,scx,got)}
			p.lineState=EnterHBlank
			p.lx=168
			want:=byte(0xff)
			if model==types.DMGABC && scx&3==3 { want=0x5a }
			if got:=b.Read(0xfe00);got!=want {t.Fatalf("%s SCX=%d late OAM read: got %02x want %02x",model,scx,got,want)}
			b.Write(0xfe00,0xaa)
			if got:=b.Get(0xfe00);got!=0x5a {t.Fatalf("%s SCX=%d OAM write lock bypassed: %02x",model,scx,got)}
			p.lineState=StartOAMScan
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("%s OAM read unlocked during scan: %02x",model,got)}
		}
	}
}

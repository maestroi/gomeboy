package ppu

import (
	"testing"
	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)
func TestCPUOAMReadFinalPixelHandoverPreservesWrites(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(),func(t *testing.T){
			s:=scheduler.NewScheduler()
			b:=gbio.NewBus(s,make([]byte,0x8000))
			b.Map(model)
			p:=New(b,s)
			p.enabled=true
			p.mode=ModeVRAM
			p.lineState=PixelTransferLX
			p.lx=166
			b.Set(0xfe00,0x5a)
			b.Lock(gbio.OAM)
			p.bgFIFO.Size=1
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("early read: %02x",got)}
			p.lx=167
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("last buffered pixel must retain OAM read lock: %02x",got)}
			p.lineState=EnterHBlank
			p.lx=168
			if got:=b.Read(0xfe00);got!=0x5a {t.Fatalf("finished last pixel OAM read: %02x",got)}
			b.Write(0xfe00,0xaa)
			if got:=b.Get(0xfe00);got!=0x5a {t.Fatalf("OAM write unexpectedly opened: %02x",got)}
			// A final pixel that has not been committed to HBlank
			// cannot open the CPU OAM read bus, even with buffered data.
			p.lineState=PixelTransferLX
			p.lx=167
			p.fetcherObj=true
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("OBJ fetch read: %02x",got)}
			p.fetcherObj=false
			p.bgFIFO.Size=0
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("empty FIFO read: %02x",got)}
			p.bgFIFO.Size=1
			p.lineState=StartOAMScan
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("OAM scan read: %02x",got)}
			p.lineState=EnterHBlank
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("LX167 cannot reopen the CPU OAM read bus: %02x",got)}
		})
	}
}

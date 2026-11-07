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
			if got:=b.Read(0xfe00);got!=0x5a {t.Fatalf("final buffered pixel read: %02x",got)}
			b.Write(0xfe00,0xaa)
			if got:=b.Get(0xfe00);got!=0x5a {t.Fatalf("OAM write unexpectedly opened: %02x",got)}
			p.fetcherObj=true
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("OBJ fetch read: %02x",got)}
			p.fetcherObj=false
			p.bgFIFO.Size=0
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("empty FIFO read: %02x",got)}
			p.bgFIFO.Size=1
			p.lineState=StartOAMScan
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("OAM scan read: %02x",got)}
			p.lineState=EnterHBlank
			if got:=b.Read(0xfe00);got!=0xff {t.Fatalf("coarse HBlank lock unexpectedly released: %02x",got)}
		})
	}
}

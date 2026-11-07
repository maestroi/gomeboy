package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestDMGBufferedLastPixelVRAMReadIsReadOnly(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.MGB} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			p := New(b, s)
			p.enabled = true
			if model == types.DMGABC {
				p.scx = 3 // measured late-HBlank ownership phase
			}
			p.mode = ModeVRAM
			p.lineState = PixelTransferLX
			p.lx = 167
			b.Set(0x8000, 0x5a)
			b.Lock(gbio.VRAM | gbio.OAM)
			p.bgFIFO.Size = 1

			want := byte(0xff)
			if model == types.DMGABC { want = 0x5a }
			if got := b.Read(0x8000); got != want {
				t.Fatalf("read while final pixel buffered: got %02x want %02x",got,want)
			}
			b.Write(0x8000,0xaa)
			if got := b.Get(0x8000); got != 0x5a {
				t.Fatalf("VRAM write bypassed PPU lock: got %02x",got)
			}
			if got := b.Read(0xfe00); got != 0xff {
				t.Fatalf("OAM read lock unexpectedly released: %02x",got)
			}

			p.fetcherObj = true
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("OBJ fetch prematurely released VRAM read: %02x",got)
			}
			p.fetcherObj = false
			p.bgFIFO.Size = 0
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("empty FIFO prematurely released VRAM read: %02x",got)
			}
			p.bgFIFO.Size = 1
			p.lx = 166
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("read port opened before last pixel: %02x",got)
			}
			p.lx = 167
			p.lineState = EnterHBlank
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("DMG model should use ordinary coarse HBlank bus-unlock event: %02x",got)
			}
		})
	}
}

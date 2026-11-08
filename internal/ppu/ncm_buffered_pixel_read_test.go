package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestNCMBufferedLastPixelCPUReadOwnership(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = false
			p.mode = ModeVRAM
			p.lineState = PixelTransferLX
			p.lx = 167
			b.Set(0x8000, 0x5a)
			b.Lock(gbio.VRAM | gbio.OAM)
			p.bgFIFO.Size = 1

			want := byte(0xff)
			if model.IsCGB() {
				want = 0x5a
			}
			if got := b.Read(0x8000); got != want {
				t.Fatalf("NCM final buffered pixel VRAM read: got %#02x, want %#02x", got, want)
			}
			b.Write(0x8000, 0xaa)
			if got := b.Get(0x8000); got != 0x5a {
				t.Fatalf("NCM buffered pixel unexpectedly released VRAM writes: %#02x", got)
			}
			if got := b.Read(0xfe00); got != 0xff {
				t.Fatalf("NCM buffered pixel unexpectedly released OAM: %#02x", got)
			}

			// An active OBJ fetch still owns the pixel pipeline.
			p.fetcherObj = true
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("OBJ fetch released VRAM read prematurely: %#02x", got)
			}
			p.fetcherObj = false
			p.bgFIFO.Size = 0
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("empty BG FIFO released VRAM read prematurely: %#02x", got)
			}
			p.bgFIFO.Size = 1
			p.lx = 166
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM opened before final visible pixel: %#02x", got)
			}
			p.lx = 167
			p.cgbMode = model.IsCGB()
			want = 0xff
			if got := b.Read(0x8000); got != want {
				t.Fatalf("final pixel mode transition: got %#02x, want %#02x", got, want)
			}
		})
	}
}

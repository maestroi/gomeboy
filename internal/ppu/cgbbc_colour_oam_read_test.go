package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// At double speed the native CGB CPU OAM read port can complete a read
// during the first LCD-enabled line's dot-78 handover on both B/C and
// D/E. OAM writes and the PPU OAM scan remain locked.
func TestCGBColourDoubleSpeedStartupOAMReadPhase(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			name := fmt.Sprintf("%s/double=%t", model, doubleSpeed)
			t.Run(name, func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				rom[0x143] = 0x80 // native CGB-colour cartridge
				b := gbio.NewBus(s, rom)
				b.Map(model)
				p := New(b, s)
				if doubleSpeed {
					s.ChangeSpeed(true)
				}
				b.Set(0xfe00, 0x5a)
				b.Write(types.LCDC, types.Bit7)
				if !p.enabled || !p.cgbMode {
					t.Fatal("expected native CGB-colour LCD startup")
				}
				if !doubleSpeed {
					s.Tick(78)
					if got := b.Read(0xfe00); got != 0xff {
						t.Fatalf("normal-speed OAM read bypassed Mode-3 lock: %02x", got)
					}
					return
				}
				s.Tick(155)
				if got := b.Read(0xfe00); got != 0x5a {
					t.Fatalf("early startup OAM read closed before dot 78: %02x", got)
				}
				s.Tick(1) // CPU clock 156: Mode-3 coarse read lock
				want := byte(0x5a)
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("CPU OAM read at clock 156: got %02x want %02x", got, want)
				}
				b.Write(0xfe00, 0xa7)
				if got := b.Get(0xfe00); got != 0x5a {
					t.Fatalf("OAM write escaped startup lock: %02x", got)
				}
				s.Tick(1)
				if got := b.Read(0xfe00); got != want {
					t.Fatalf("CPU OAM read at clock 157: got %02x want %02x", got, want)
				}
				s.Tick(1) // CPU clock 158: read grace expires
				if got := b.Read(0xfe00); got != 0xff {
					t.Fatalf("CPU OAM read remained open during Mode 3: %02x", got)
				}
			})
		}
	}
}

// The CGB double-speed pixel buffer gives the CPU an earlier read-only
// OAM window than the PPU HBlank/STAT event. Restrict it to a ready BG
// pixel, no OBJ fetch, and native colour mode on physical CGB.
func TestCGBColourBufferedFinalPixelOAMReadPhase(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			for scx := uint8(0); scx < 8; scx++ {
				name := fmt.Sprintf("%s/double=%t/scx=%d", model, doubleSpeed, scx)
				t.Run(name, func(t *testing.T) {
					s := scheduler.NewScheduler()
					rom := make([]byte, 0x8000)
					rom[0x143] = 0x80
					b := gbio.NewBus(s, rom)
					b.Map(model)
					p := New(b, s)
					if doubleSpeed {
						s.ChangeSpeed(true)
					}
					p.enabled = true
					p.cgbMode = true
					p.lineState = PixelTransferLX
					p.lx = 166
					p.scx = scx
					p.bgFIFO.Size = 1
					b.Set(0xfe00, 0x5a)
					b.Lock(gbio.OAM)
					if got := b.Read(0xfe00); got != 0xff {
						t.Fatalf("pre-final-pixel read unexpectedly open: %02x", got)
					}
					p.lx = 167
					want := byte(0xff)
					if model.IsCGB() && doubleSpeed && scx&1 == 0 {
						want = 0x5a
					}
					if got := b.Read(0xfe00); got != want {
						t.Fatalf("buffered final-pixel OAM read: got %02x want %02x", got, want)
					}
					b.Write(0xfe00, 0xaa)
					if got := b.Get(0xfe00); got != 0x5a {
						t.Fatalf("buffered final-pixel OAM write unlocked: %02x", got)
					}
					p.fetcherObj = true
					if got := b.Read(0xfe00); got != 0xff {
						t.Fatalf("in-flight OBJ fetch released OAM: %02x", got)
					}
					p.fetcherObj = false
					p.bgFIFO.Size = 0
					if got := b.Read(0xfe00); got != 0xff {
						t.Fatalf("empty BG FIFO released OAM: %02x", got)
					}
					p.bgFIFO.Size = 1
					p.lx = 168
					p.lineState = EnterHBlank
					want = 0xff
					if model == types.CGBBC && !doubleSpeed && scx&3 == 3 ||
						model.IsCGB() && doubleSpeed && scx&1 == 0 {
						want = 0x5a
					}
					if got := b.Read(0xfe00); got != want {
						t.Fatalf("completed final-pixel read: got %02x want %02x", got, want)
					}
				})
			}
		}
	}
}

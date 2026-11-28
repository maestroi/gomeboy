package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// At CGB double speed the CPU-visible Mode-3-to-0 edge depends on the
// fine-scroll parity. Even SCX transitions when the final pixel is
// buffered, while odd SCX waits for the scheduled HBlank entry.
// This is a STAT read-port phase, not an early PPU/bus/IRQ transition.
func TestDoubleSpeedCGBHBlankSTATReadFineScrollParity(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for scx := uint8(0); scx < 10; scx++ {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = true
				s.ChangeSpeed(true)
				b.Write(types.SCX, scx)

				// The buffered final-pixel event has completed, but
				// internal Mode 3 and bus ownership have not changed.
				p.mode, p.modeToInt = ModeVRAM, ModeVRAM
				p.lineState = EnterHBlank
				p.lx = 168
				s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())

				for clock := 0; clock < 2; clock++ {
					want := byte(ModeVRAM)
					if scx&1 == 0 {
						want = ModeHBlank
					}
					if got := b.Read(types.STAT) & 3; got != want {
						t.Fatalf("CPU clock %d STAT mode=%d, want %d", clock, got, want)
					}
					if p.mode != ModeVRAM || p.modeToInt != ModeVRAM {
						t.Fatalf("CPU clock %d changed internal/IRQ mode: %d/%d", clock, p.mode, p.modeToInt)
					}
					s.Tick(1)
				}
				if got := b.Read(types.STAT) & 3; got != ModeHBlank {
					t.Fatalf("STAT after HBlank entry=%d, want Mode 0", got)
				}
				if p.mode != ModeHBlank || p.modeToInt != ModeHBlank {
					t.Fatalf("internal/IRQ mode after HBlank entry=%d/%d, want 0/0", p.mode, p.modeToInt)
				}
			})
		}
	}
}

// WX=0 under double speed keeps Mode 3 through enterHBlankDelay at even
// SCX when fine scroll is nonzero (window pixels absorb the discard).
// Fine SCX 0 and ordinary WX opens still expose Mode 0 at the buffered
// final-pixel edge.
func TestDoubleSpeedWX0WindowSTATFineScrollHBlank(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, tc := range []struct {
			name       string
			wx, scx    uint8
			winOpen    bool
			wantMode3  bool
		}{
			{name: "WX0/SCX0", wx: 0, scx: 0, winOpen: true, wantMode3: false},
			{name: "WX0/SCX2", wx: 0, scx: 2, winOpen: true, wantMode3: true},
			{name: "WX0/SCX4", wx: 0, scx: 4, winOpen: true, wantMode3: true},
			{name: "WX0/SCX6", wx: 0, scx: 6, winOpen: true, wantMode3: true},
			{name: "WX0/SCX1", wx: 0, scx: 1, winOpen: true, wantMode3: true},
			{name: "WX7/SCX2", wx: 7, scx: 2, winOpen: true, wantMode3: false},
			{name: "WX0-closed/SCX2", wx: 0, scx: 2, winOpen: false, wantMode3: false},
		} {
			t.Run(fmt.Sprintf("%s/%s", model, tc.name), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = true
				s.ChangeSpeed(true)
				p.wx = tc.wx
				p.scx = tc.scx
				p.winTriggerWx = tc.winOpen
				p.mode, p.modeToInt = ModeVRAM, ModeVRAM
				p.lineState = EnterHBlank
				p.lx = 168
				s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())

				want := byte(ModeHBlank)
				if tc.wantMode3 || tc.scx&1 != 0 {
					want = ModeVRAM
				}
				if got := b.Read(types.STAT) & 3; got != want {
					t.Fatalf("STAT mode=%d, want %d", got, want)
				}
				if p.mode != ModeVRAM {
					t.Fatalf("internal mode changed early: %d", p.mode)
				}
			})
		}
	}
}

// The initial shortened LCD-on line enters the same EnterHBlank state
// before it has generated any pixels, and waits for its entire startup
// Mode-3 interval. It must not expose Mode 0 early at even SCX.
func TestDoubleSpeedCGBStartupTransferKeepsSTATMode3(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		for _, scx := range []byte{0, 2, 4, 6, 8} {
			t.Run(fmt.Sprintf("%s/SCX=%d", model, scx), func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(model)
				p := New(b, s)
				p.enabled = true
				p.cgbMode = true
				s.ChangeSpeed(true)
				b.Write(types.SCX, scx)
				p.ly = 0
				p.lx = 0
				p.glitchedLineState = GlitchedLineStartPixelTransfer
				p.mode, p.modeToInt = ModeVRAM, ModeVRAM
				p.lineState = EnterHBlank
				s.ScheduleEvent(scheduler.PPUHandleVisualLine, 168+uint64(scx&7))

				// AGE's first sample can fall inside this startup transfer;
				// STAT must remain Mode 3 despite the state name.
				for clock := 0; clock < 4; clock++ {
					if got := b.Read(types.STAT) & 3; got != ModeVRAM {
						t.Fatalf("startup clock %d STAT=%d, want Mode 3", clock, got)
					}
					s.Tick(1)
				}
			})
		}
	}
}

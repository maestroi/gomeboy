package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// At the first scanline after VBlank, CGB D/E in DMG compatibility mode
// exposes the prior Mode 1 for one extra CPU clock even though the PPU has
// already switched its internal mode to HBlank. This is a STAT read-port
// phase, not a bus or interrupt timing adjustment.
func TestCGBDECompatibilityFrameStartSTATModeHandover(t *testing.T) {
	for _, tc := range []struct {
		model   types.Model
		cgbMode bool
		want    uint8
	}{
		{types.DMGABC, false, ModeHBlank},
		{types.CGBBC, false, ModeHBlank},
		{types.CGBDE, false, ModeVBlank},
	} {
		t.Run(fmt.Sprintf("%s/cgb=%t", tc.model, tc.cgbMode), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = tc.cgbMode
			p.mode, p.modeToInt = ModeVBlank, ModeVBlank
			p.lineState = HBlankEnd
			p.ly = 0

			p.handleVisualLine()
			if p.mode != ModeHBlank {
				t.Fatalf("PPU delayed its internal HBlank mode: %d", p.mode)
			}
			if got := b.Read(types.STAT) & 3; got != tc.want {
				t.Fatalf("CPU STAT at frame HBlank handover = %d, want %d", got, tc.want)
			}
			s.Tick(1)
			if got := b.Read(types.STAT) & 3; got != ModeOAM {
				t.Fatalf("CPU STAT on first Mode-2 clock = %d, want %d", got, ModeOAM)
			}
		})
	}
}

func TestCGBDECompatibilityFrameSTATLatchResetAndSnapshot(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBDE)
	p := New(b, s)
	p.enabled = true
	p.mode = ModeHBlank
	p.frameSTATMode1Until = s.Cycle() + 1

	snapshot := p.Snapshot()
	p.frameSTATMode1Until = 0
	p.Restore(snapshot)
	if got := b.Read(types.STAT) & 3; got != ModeVBlank {
		t.Fatalf("restored frame transition STAT = %d, want Mode 1", got)
	}
	b.Write(types.LCDC, 0)
	if p.frameSTATMode1Until != 0 {
		t.Fatal("LCD disable retained VBlank STAT read handover")
	}
	if got := b.Read(types.STAT) & 3; got != ModeHBlank {
		t.Fatalf("LCD-off STAT after reset = %d, want Mode 0", got)
	}
}

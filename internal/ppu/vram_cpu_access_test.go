package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newMode3VRAMReadFixture(t *testing.T, model types.Model, doubleSpeed bool) (*PPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()

	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(model)
	p := New(b, s)
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	p.enabled = true
	p.cgbMode = model.IsCGB()
	p.mode = ModeVRAM
	p.lineState = PixelTransferDummy

	b.Write(0x8000, 0x5a)
	s.Tick(200)
	p.lineDot = s.Cycle() - 80
	if doubleSpeed {
		p.lineDot = s.Cycle() - 160
	}
	b.RLock(gbio.VRAM)
	return p, b, s
}

func TestCGBVRAMMode3ReadCloseEdge(t *testing.T) {
	for _, tc := range []struct {
		name        string
		model       types.Model
		doubleSpeed bool
		openCycles  uint64
	}{
		{name: "CGB-BC normal", model: types.CGBBC, openCycles: 4},
		{name: "CGB-DE normal", model: types.CGBDE, openCycles: 4},
		{name: "CGB-BC double", model: types.CGBBC, doubleSpeed: true, openCycles: 4},
		{name: "CGB-DE double", model: types.CGBDE, doubleSpeed: true, openCycles: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, b, s := newMode3VRAMReadFixture(t, tc.model, tc.doubleSpeed)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("CGB VRAM read blocked at Mode-3 start: got %#02x", got)
			}
			s.Tick(tc.openCycles - 1)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("CGB VRAM read blocked before close edge: got %#02x", got)
			}
			s.Tick(1)
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("CGB VRAM read stayed open at close edge: got %#02x", got)
			}
		})
	}
}

func TestVRAMReadEdgeUsesPhysicalCGBHardwareInCompatibilityMode(t *testing.T) {
	p, b, _ := newMode3VRAMReadFixture(t, types.CGBBC, false)
	p.cgbMode = false // DMG-only cartridge on physical CGB
	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("NCM cartridge lost physical CGB VRAM read window: got %#02x", got)
	}
}

func TestVRAMReadEdgeLeavesDMGAndOtherModesUnchanged(t *testing.T) {
	p, b, _ := newMode3VRAMReadFixture(t, types.DMGABC, false)
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("DMG bypassed coarse VRAM read lock: got %#02x", got)
	}

	p, b, _ = newMode3VRAMReadFixture(t, types.CGBBC, false)
	p.lineState = EnterHBlank // LCD startup/glitched scanline
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("glitched line lost its separate VRAM read lock: got %#02x", got)
	}
	p.lineState = StartOAMScan
	p.mode = ModeOAM
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("Mode-2 lock changed outside targeted window: got %#02x", got)
	}
	p.mode = ModeHBlank
	p.lineState = HBlankUpdateLY
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("preexisting locked HBlank edge was overridden: got %#02x", got)
	}
	b.RUnlock(gbio.VRAM)
	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("unlocked HBlank VRAM read was blocked: got %#02x", got)
	}
}

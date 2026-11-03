package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBVRAMReadDelayedHBlankRelease(t *testing.T) {
	for _, tc := range []struct {
		model types.Model
		doubleSpeed bool
		delay uint64
	}{
		{types.CGBBC, false, 1},
		{types.CGBDE, false, 1},
		{types.CGBBC, true, 2},
		{types.CGBDE, true, 2},
	} {
		t.Run(tc.model.String()+"/"+map[bool]string{false: "normal", true: "double"}[tc.doubleSpeed], func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			p := New(b, s)
			if tc.doubleSpeed {
				s.ChangeSpeed(true)
			}
			b.Write(0x8000, 0x5a)
			p.enabled = true
			p.mode = ModeHBlank
			p.lineState = HBlankUpdateLY

			p.scheduleVRAMCPUReadRelease()
			if got := p.vramCPUReadReadyAt - s.Cycle(); got != tc.delay {
				t.Fatalf("HBlank read-release delay=%d, expected %d", got, tc.delay)
			}
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM read should remain blocked at HBlank entry, got %02x", got)
			}
			s.Tick(tc.delay - 1)
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("VRAM read opened before release edge, got %02x", got)
			}
			s.Tick(1)
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("VRAM read remained blocked at release edge, got %02x", got)
			}
		})
	}
}

func TestVRAMReadReleaseDoesNotOverrideExistingPPULocks(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.DMGABC} {
		s := scheduler.NewScheduler()
		b := gbio.NewBus(s, make([]byte, 0x8000))
		b.Map(model)
		p := New(b, s)
		b.Write(0x8000, 0x5a)
		p.enabled = true
		p.mode = ModeHBlank
		p.lineState = HBlankUpdateLY
		p.scheduleVRAMCPUReadRelease()

		b.RLock(gbio.VRAM)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("%s: coarse VRAM lock bypassed: %02x", model, got)
		}
		b.RUnlock(gbio.VRAM)
		if model == types.DMGABC {
			if got := b.Read(0x8000); got != 0x5a {
				t.Fatalf("DMG HBlank read unexpectedly blocked: %02x", got)
			}
			continue
		}
		p.mode = ModeVRAM
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("CGB normal Mode-3 ownership changed: %02x", got)
		}
		p.mode = ModeHBlank
		p.lineState = StartOAMScan
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("CGB pre-HBlank scan state was affected: %02x", got)
		}
		p.lineState = HBlankUpdateLY
		p.enabled = false
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("LCD-off VRAM read unexpectedly blocked: %02x", got)
		}
	}
}

func TestVRAMCPUReadReleaseSurvivesPPUSnapshot(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.CGBBC)
	p := New(b, s)
	p.vramCPUReadReadyAt = 123
	if got := p.Snapshot().VRAMCPUReadReadyAt; got != 123 {
		t.Fatalf("Snapshot: got %d", got)
	}
	var checkpoint State
	p.SnapshotInto(&checkpoint)
	if got := checkpoint.VRAMCPUReadReadyAt; got != 123 {
		t.Fatalf("SnapshotInto: got %d", got)
	}
	p.vramCPUReadReadyAt = 0
	p.Restore(checkpoint)
	if got := p.vramCPUReadReadyAt; got != 123 {
		t.Fatalf("Restore: got %d", got)
	}
}

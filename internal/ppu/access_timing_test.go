package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newAccessTimingTest(t *testing.T, model types.Model, doubleSpeed bool) (*PPU, *gbio.Bus, *scheduler.Scheduler) {
	t.Helper()

	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	p := New(b, s)
	b.Map(model)
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	p.enabled = true
	return p, b, s
}

func TestCGBCompatibilityModeKeepsCGBVRAMReadTail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model types.Model
		open  bool
	}{
		{name: "CGB hardware in compatibility mode", model: types.CGBBC, open: true},
		{name: "DMG hardware", model: types.DMGABC, open: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, b, _ := newAccessTimingTest(t, tc.model, false)
			p.cgbMode = false
			b.Write(0x8000, 0x5a)
			b.RLock(gbio.VRAM)

			p.lineState = ReleaseOAMBus
			p.oamScanIndex = 38
			p.handleVisualLine()

			got := b.Read(0x8000)
			if tc.open && got != 0x5a {
				t.Fatalf("VRAM read at OAM tail = %#02x, want %#02x", got, 0x5a)
			}
			if !tc.open && got != 0xff {
				t.Fatalf("VRAM read at OAM tail = %#02x, want blocked %#02x", got, 0xff)
			}
		})
	}
}

func TestCGBVRAMReadCloseLagsMode3(t *testing.T) {
	t.Run("normal speed", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, false)
		p.cgbMode = false // NCM still uses CGB bus timing.
		b.Write(0x8000, 0x5a)
		p.lineState = StartPixelTransfer
		p.handleVisualLine()

		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM closed at Mode-3 edge: got %#02x", got)
		}
		s.Tick(2)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM closed before three-dot CGB edge: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("VRAM still open at three-dot CGB edge: got %#02x", got)
		}
	})

	t.Run("double speed", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, true)
		b.Write(0x8000, 0x5a)
		p.lineState = StartPixelTransfer
		p.handleVisualLine()

		// Two PPU dots map to four scheduler/CPU clocks in double speed.
		s.Tick(3)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM closed before double-speed edge: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("VRAM still open at double-speed edge: got %#02x", got)
		}
	})
}

func TestMode0ReadOpenEdgesIncludeCGBERevisionDelay(t *testing.T) {
	t.Run("CGB B/C", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, false)
		b.Write(0x8000, 0x5a)
		b.Write(0xfe00, 0x66)
		b.RLock(gbio.VRAM | gbio.OAM)

		p.scheduleMode0AccessRelease(0)
		s.Tick(1)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("VRAM opened one dot early: got %#02x", got)
		}
		if got := b.Read(0xfe00); got != 0xff {
			t.Fatalf("OAM opened one dot early: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM did not open at two-dot edge: got %#02x", got)
		}
		if got := b.Read(0xfe00); got != 0x66 {
			t.Fatalf("OAM did not open at two-dot edge: got %#02x", got)
		}
	})

	t.Run("CGB D/E", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBDE, false)
		b.Write(0x8000, 0x5a)
		b.Write(0xfe00, 0x66)
		b.RLock(gbio.VRAM | gbio.OAM)

		p.scheduleMode0AccessRelease(0)
		s.Tick(2)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM did not open at common two-dot edge: got %#02x", got)
		}
		if got := b.Read(0xfe00); got != 0xff {
			t.Fatalf("CGB D/E OAM opened before revision edge: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0xfe00); got != 0x66 {
			t.Fatalf("CGB D/E OAM did not open at delayed edge: got %#02x", got)
		}
	})
}

func TestDoubleSpeedCGBBCOAMReadMode2Lag(t *testing.T) {
	p, b, s := newAccessTimingTest(t, types.CGBBC, true)
	b.Write(0xfe00, 0x66)
	p.lineState = StartOAMScan
	p.oamScanIndex = 0
	p.handleVisualLine()

	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("OAM read closed at double-speed Mode-2 edge: got %#02x", got)
	}
	s.Tick(3)
	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("OAM read closed before two PPU dots: got %#02x", got)
	}
	s.Tick(1)
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("OAM read still open after two PPU dots: got %#02x", got)
	}

	pE, bE, _ := newAccessTimingTest(t, types.CGBDE, true)
	bE.Write(0xfe00, 0x66)
	pE.lineState = StartOAMScan
	pE.oamScanIndex = 0
	pE.handleVisualLine()
	if got := bE.Read(0xfe00); got != 0xff {
		t.Fatalf("CGB D/E OAM read should close immediately: got %#02x", got)
	}
}

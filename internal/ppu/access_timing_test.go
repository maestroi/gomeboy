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
	b.Map(model)
	p := New(b, s)
	if doubleSpeed {
		s.ChangeSpeed(true)
	}
	p.enabled = true
	return p, b, s
}

func beginAccessMode3(p *PPU, s *scheduler.Scheduler, firstLine bool) {
	if s.Cycle() < 100 {
		s.Tick(100 - s.Cycle())
	}
	p.accessFirstLine = firstLine
	p.accessMode0Cycle = 0
	p.accessMode3Cycle = s.Cycle()
	p.lineDot = s.Cycle() - 80
	p.mode = ModeVRAM
}

func TestVRAMReadAccessEdges(t *testing.T) {
	t.Run("CGB single-speed close trails Mode 3 by four dots", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, false)
		b.Write(0x8000, 0x5a)
		beginAccessMode3(p, s, false)
		b.RLock(gbio.VRAM) // coarse lock is deliberately overridden by the access predicate

		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM closed on the CGB Mode-3 edge: got %#02x", got)
		}
		s.Tick(3)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM closed before four-dot edge: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("VRAM still open at four-dot edge: got %#02x", got)
		}
	})

	t.Run("DMG closes on live Mode 3", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.DMGABC, false)
		b.Write(0x8000, 0x5a)
		beginAccessMode3(p, s, false)
		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("DMG VRAM read stayed open in Mode 3: got %#02x", got)
		}
	})

	t.Run("CGB compatibility mode keeps physical CGB timing", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, false)
		p.cgbMode = false
		b.Write(0x8000, 0x5a)
		beginAccessMode3(p, s, false)
		b.RLock(gbio.VRAM)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("NCM VRAM read used DMG timing: got %#02x", got)
		}
	})
}

func TestMode0ReadOpenEdges(t *testing.T) {
	t.Run("CGB B/C VRAM and OAM open after two dots", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBBC, false)
		b.Write(0x8000, 0x5a)
		b.Write(0xfe00, 0x66)
		beginAccessMode3(p, s, false)
		s.Tick(10)
		p.mode = ModeHBlank
		p.accessMode0Cycle = s.Cycle()
		b.RLock(gbio.VRAM | gbio.OAM)

		if got := b.Read(0x8000); got != 0xff {
			t.Fatalf("VRAM opened on Mode-0 edge: got %#02x", got)
		}
		if got := b.Read(0xfe00); got != 0xff {
			t.Fatalf("OAM opened on Mode-0 edge: got %#02x", got)
		}
		s.Tick(2)
		if got := b.Read(0x8000); got != 0x5a {
			t.Fatalf("VRAM did not open after two dots: got %#02x", got)
		}
		if got := b.Read(0xfe00); got != 0x66 {
			t.Fatalf("OAM did not open after two dots: got %#02x", got)
		}
	})

	t.Run("CGB D/E OAM read opens one dot later", func(t *testing.T) {
		p, b, s := newAccessTimingTest(t, types.CGBDE, false)
		b.Write(0xfe00, 0x66)
		beginAccessMode3(p, s, false)
		s.Tick(10)
		p.mode = ModeHBlank
		p.accessMode0Cycle = s.Cycle()
		b.RLock(gbio.OAM)

		s.Tick(2)
		if got := b.Read(0xfe00); got != 0xff {
			t.Fatalf("CGB D/E OAM opened at B/C edge: got %#02x", got)
		}
		s.Tick(1)
		if got := b.Read(0xfe00); got != 0x66 {
			t.Fatalf("CGB D/E OAM did not open at delayed edge: got %#02x", got)
		}
	})
}

func TestOAMMode2ReadRevisionEdge(t *testing.T) {
	p, b, s := newAccessTimingTest(t, types.CGBBC, true)
	b.Write(0xfe00, 0x66)
	s.Tick(100)
	p.accessFirstLine = false
	p.lineDot = s.Cycle()
	p.mode = ModeOAM
	b.RLock(gbio.OAM)

	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("CGB B/C double-speed OAM read closed at Mode-2 edge: got %#02x", got)
	}
	s.Tick(2) // one PPU dot in double speed
	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("CGB B/C double-speed OAM read closed one dot early: got %#02x", got)
	}
	s.Tick(2)
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("CGB B/C double-speed OAM read remained open after two dots: got %#02x", got)
	}

	pE, bE, sE := newAccessTimingTest(t, types.CGBDE, true)
	bE.Write(0xfe00, 0x66)
	sE.Tick(100)
	pE.accessFirstLine = false
	pE.lineDot = sE.Cycle()
	pE.mode = ModeOAM
	bE.RLock(gbio.OAM)
	if got := bE.Read(0xfe00); got != 0xff {
		t.Fatalf("CGB D/E double-speed OAM read should close immediately: got %#02x", got)
	}
}

func TestLCDEnableLineAccessCloseEdges(t *testing.T) {
	p, b, s := newAccessTimingTest(t, types.CGBBC, false)
	b.Write(0x8000, 0x5a)
	b.Write(0xfe00, 0x66)
	beginAccessMode3(p, s, true)
	b.Lock(gbio.VRAM | gbio.OAM)

	if got := b.Read(0xfe00); got != 0x66 {
		t.Fatalf("first-line OAM read closed on Mode-3 flag edge: got %#02x", got)
	}
	b.Write(0xfe00, 0x77)
	if got := b.Get(0xfe00); got != 0x77 {
		t.Fatalf("first-line OAM write closed on Mode-3 flag edge: got %#02x", got)
	}
	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("first-line VRAM read closed on Mode-3 flag edge: got %#02x", got)
	}

	s.Tick(5)
	if got := b.Read(0xfe00); got != 0xff {
		t.Fatalf("first-line OAM read still open at five-dot edge: got %#02x", got)
	}
	b.Set(0xfe00, 0x77)
	b.Write(0xfe00, 0x88)
	if got := b.Get(0xfe00); got != 0x77 {
		t.Fatalf("first-line OAM write still open at five-dot edge: got %#02x", got)
	}

	// CGB VRAM reads keep two additional dots of LCD-enable-line grace.
	if got := b.Read(0x8000); got != 0x5a {
		t.Fatalf("first-line VRAM read closed at OAM edge: got %#02x", got)
	}
	s.Tick(2)
	if got := b.Read(0x8000); got != 0xff {
		t.Fatalf("first-line VRAM read still open at seven-dot edge: got %#02x", got)
	}
}

func TestDMGOAMWriteMode2AndMode3Gaps(t *testing.T) {
	p, b, s := newAccessTimingTest(t, types.DMGABC, false)
	b.Set(0xfe00, 0x11)
	s.Tick(100)
	p.accessFirstLine = false
	p.lineDot = s.Cycle()
	p.mode = ModeOAM
	b.WLock(gbio.OAM)

	b.Write(0xfe00, 0x22)
	if got := b.Get(0xfe00); got != 0x22 {
		t.Fatalf("DMG OAM write blocked at start of Mode 2: got %#02x", got)
	}

	// At the Mode-3 edge the DMG exposes a four-dot write gap even though
	// the coarse PPU lock is already closed.
	s.Tick(80)
	p.mode = ModeVRAM
	p.accessMode3Cycle = s.Cycle()
	b.Write(0xfe00, 0x33)
	if got := b.Get(0xfe00); got != 0x33 {
		t.Fatalf("DMG OAM write blocked at Mode-3 gap: got %#02x", got)
	}
	s.Tick(4)
	b.Write(0xfe00, 0x44)
	if got := b.Get(0xfe00); got != 0x33 {
		t.Fatalf("DMG OAM write remained open after Mode-3 gap: got %#02x", got)
	}
}

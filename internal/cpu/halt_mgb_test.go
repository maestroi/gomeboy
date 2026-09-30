package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestMGBHaltFreezesOAMDMAAfterFinalPrefetch(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.MGB)
	c := NewCPU(b, s)

	// Source byte 2 and OAM bytes 2/3 are the values used by the Mooneye
	// hardware test. Distinct first bytes make the exact freeze phase visible.
	b.Set(0x2000, 0x11)
	b.Set(0x2001, 0x22)
	b.Set(0x2002, 0x1a)
	b.Set(0xfe00, 0xff)
	b.Set(0xfe01, 0xff)
	b.Set(0xfe02, 0x30)
	b.Set(0xfe03, 0x40)
	b.Set(0xfe04, 0x9f)
	b.Set(0xfe05, 0xa7)
	b.Set(0xfe06, 0x9f)
	b.Set(0xfe07, 0xa7)

	// Writing DMA starts the controller two M-cycles later. The ROM executes a
	// NOP and fetches HALT in those two cycles, so byte 0 has committed when
	// the HALT instruction itself begins executing.
	b.Write(types.DMA, 0x20)
	s.Tick(8)
	if got := b.Get(0xfe00); got != 0x11 {
		t.Fatalf("DMA byte 0 before HALT = %#02x, want 0x11", got)
	}

	// Give skipHALT a frame boundary to return through. It must keep the
	// scheduler clock-gated because no enabled interrupt wakes the CPU.
	s.RegisterEvent(scheduler.PPUHandleVisualLine, func() {
		c.hasFrame = true
	})
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, 8)

	InstructionSet[0x76].fn(c)

	if c.DebugBreakpoint {
		t.Fatal("MGB HALT still used the old debug-break workaround")
	}
	if !c.Halted || !s.Halted {
		t.Fatalf("MGB HALT did not gate CPU/DMA clock: cpu=%v scheduler=%v", c.Halted, s.Halted)
	}
	if !c.skippingHalt {
		t.Fatal("frame boundary did not leave MGB latched in HALT")
	}

	// The final prefetch M-cycle commits byte 1, then the DMA freezes with
	// source byte 2 ($1A) aimed at OAM byte 2 ($30). Byte 2 must remain old.
	if got := b.Get(0xfe00); got != 0x11 {
		t.Errorf("DMA byte 0 after HALT = %#02x, want 0x11", got)
	}
	if got := b.Get(0xfe01); got != 0x22 {
		t.Errorf("DMA byte 1 after HALT = %#02x, want 0x22", got)
	}
	if got := b.Get(0xfe02); got != 0x30 {
		t.Errorf("in-flight DMA byte committed while halted: %#02x, want old 0x30", got)
	}

	// The MGB PPU sees the frozen bus tuple on every OAM entry:
	// Y/tile = ($30 | $1A) & $FC = $38, X/flags = $40 | $1A = $5A.
	if got := b.PPUReadOAM(0xfe00); got != 0x38 {
		t.Errorf("frozen MGB OAM Y/tile = %#02x, want 0x38", got)
	}
	if got := b.PPUReadOAM(0xfe01); got != 0x5a {
		t.Errorf("frozen MGB OAM X/flags = %#02x, want 0x5a", got)
	}
}

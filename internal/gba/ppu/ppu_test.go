package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func newTestPPU(t *testing.T, hooks Hooks) (*PPU, *bus.Bus) {
	t.Helper()
	b := bus.New(nil, nil)
	p := New(b, hooks)
	return p, b
}

func TestLCDInitialState(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	if got := p.VCount(); got != 0 {
		t.Fatalf("VCOUNT = %d, want 0", got)
	}
	if got := p.LineCycle(); got != 0 {
		t.Fatalf("line cycle = %d, want 0", got)
	}
	if p.InHBlank() || p.InVBlank() {
		t.Fatalf("initial blank flags H=%v V=%v", p.InHBlank(), p.InVBlank())
	}

	status, _ := b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})
	if status&(1<<2) == 0 {
		t.Fatalf("initial VCounter flag clear: DISPSTAT=%04x", status)
	}
	vcount, _ := b.Read16(bus.IOStart+vcountOffset, bus.Access{})
	if vcount != 0 {
		t.Fatalf("mapped VCOUNT = %d, want 0", vcount)
	}
}

func TestScanlineTimingAndHBlankFlagDelay(t *testing.T) {
	var hblankSignals, hblankIRQs int
	p, b := newTestPPU(t, Hooks{
		HBlank: func() { hblankSignals++ },
		IRQ: func(source IRQSource) {
			if source == IRQHBlank {
				hblankIRQs++
			}
		},
	})

	// Enable HBlank IRQ.
	b.Write16(bus.IOStart+dispSTATOffset, 1<<4, bus.Access{})

	p.Advance(VisibleCycles - 1)
	if hblankSignals != 0 || p.InHBlank() {
		t.Fatalf("before HBlank: signals=%d flag=%v", hblankSignals, p.InHBlank())
	}

	p.Advance(1)
	if hblankSignals != 1 {
		t.Fatalf("HBlank signal count = %d, want 1", hblankSignals)
	}
	if p.InHBlank() {
		t.Fatal("DISPSTAT HBlank flag asserted at visible boundary; want delayed flag")
	}

	p.Advance(HBlankFlagCycle - VisibleCycles)
	if !p.InHBlank() {
		t.Fatal("HBlank flag not set at HBlankFlagCycle")
	}
	if hblankIRQs != 1 {
		t.Fatalf("HBlank IRQ count = %d, want 1", hblankIRQs)
	}

	p.Advance(CyclesPerLine - HBlankFlagCycle)
	if p.InHBlank() {
		t.Fatal("HBlank flag still set after scanline rollover")
	}
	if got := p.VCount(); got != 1 {
		t.Fatalf("VCOUNT = %d, want 1", got)
	}
	if got := p.LineCycle(); got != 0 {
		t.Fatalf("line cycle = %d, want 0", got)
	}
}

func TestVBlankRangeAndFrameLength(t *testing.T) {
	var vblankSignals, vblankIRQs, hblankIRQs int
	p, b := newTestPPU(t, Hooks{
		VBlank: func() { vblankSignals++ },
		IRQ: func(source IRQSource) {
			switch source {
			case IRQVBlank:
				vblankIRQs++
			case IRQHBlank:
				hblankIRQs++
			}
		},
	})
	b.Write16(bus.IOStart+dispSTATOffset, (1<<3)|(1<<4), bus.Access{})

	p.Advance(uint32(VBlankStartLine) * CyclesPerLine)
	if !p.InVBlank() || p.VCount() != VBlankStartLine {
		t.Fatalf("at VBlank start: line=%d vblank=%v", p.VCount(), p.InVBlank())
	}
	if vblankSignals != 1 || vblankIRQs != 1 {
		t.Fatalf("VBlank signals=%d IRQs=%d, want 1/1", vblankSignals, vblankIRQs)
	}

	status, _ := b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})
	if status&1 == 0 {
		t.Fatalf("VBlank status bit clear at line %d", p.VCount())
	}

	before := hblankIRQs
	p.Advance(CyclesPerLine)
	if hblankIRQs != before {
		t.Fatalf("HBlank IRQ fired during VBlank: before=%d after=%d", before, hblankIRQs)
	}

	remainingTo227 := uint32(VBlankEndLine-VBlankStartLine-1) * CyclesPerLine
	p.Advance(remainingTo227)
	if p.VCount() != VBlankEndLine || p.InVBlank() {
		t.Fatalf("line 227 state: line=%d vblank=%v", p.VCount(), p.InVBlank())
	}

	p.Advance(CyclesPerLine)
	if p.VCount() != 0 || p.FrameCount() != 1 {
		t.Fatalf("frame wrap: line=%d frames=%d, want 0/1", p.VCount(), p.FrameCount())
	}
}

func TestVCountMatchIRQ(t *testing.T) {
	var countIRQs int
	p, b := newTestPPU(t, Hooks{
		IRQ: func(source IRQSource) {
			if source == IRQVCount {
				countIRQs++
			}
		},
	})

	// Compare against line 2 and enable VCount IRQ.
	b.Write16(bus.IOStart+dispSTATOffset, (2<<8)|(1<<5), bus.Access{})
	status, _ := b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})
	if status&(1<<2) != 0 {
		t.Fatalf("VCount flag unexpectedly set on line 0: %04x", status)
	}

	p.Advance(2 * CyclesPerLine)
	if countIRQs != 1 {
		t.Fatalf("VCount IRQs = %d, want 1", countIRQs)
	}
	status, _ = b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})
	if status&(1<<2) == 0 {
		t.Fatalf("VCount match flag clear at line 2: %04x", status)
	}

	p.Advance(CyclesPerLine)
	status, _ = b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})
	if status&(1<<2) != 0 {
		t.Fatalf("VCount match flag still set at line 3: %04x", status)
	}
}

func TestDISPSTATStatusBitsAreReadOnly(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	_ = p

	b.Write16(bus.IOStart+dispSTATOffset, 0xffff, bus.Access{})
	got, _ := b.Read16(bus.IOStart+dispSTATOffset, bus.Access{})

	if got&0x00c0 != 0 {
		t.Fatalf("unused DISPSTAT bits persisted: %04x", got)
	}
	if got&(1<<0) != 0 || got&(1<<1) != 0 {
		t.Fatalf("software set blank status bits: %04x", got)
	}
	if got&0xff38 != 0xff38 {
		t.Fatalf("writable DISPSTAT bits = %04x, want ff38 mask", got&0xff38)
	}
}

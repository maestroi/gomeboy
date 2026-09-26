package dma_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/dma"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
)

func TestPPUHBlankAndVBlankDriveDMAStartConditions(t *testing.T) {
	b := bus.New(nil, nil)

	var stall uint32
	d := dma.New(b, nil, dma.Hooks{
		Stall: func(cycles uint32) { stall += cycles },
	})

	hSource := uint32(bus.EWRAMStart + 0x100)
	hDest := uint32(bus.IWRAMStart + 0x100)
	vSource := uint32(bus.EWRAMStart + 0x200)
	vDest := uint32(bus.IWRAMStart + 0x200)

	b.Write16(hSource, 0x1111, bus.Access{})
	b.Write16(hSource+2, 0x2222, bus.Access{})
	b.Write16(vSource, 0xaaaa, bus.Access{})

	// DMA0: one halfword each HBlank, repeated, destination reload.
	b.Write32(bus.IOStart+0x0b0, hSource, bus.Access{})
	b.Write32(bus.IOStart+0x0b4, hDest, bus.Access{})
	b.Write16(bus.IOStart+0x0b8, 1, bus.Access{})
	b.Write16(bus.IOStart+0x0ba,
		(3<<5)|(1<<9)|(2<<12)|(1<<15),
		bus.Access{})

	// DMA1: one-shot VBlank.
	b.Write32(bus.IOStart+0x0bc, vSource, bus.Access{})
	b.Write32(bus.IOStart+0x0c0, vDest, bus.Access{})
	b.Write16(bus.IOStart+0x0c4, 1, bus.Access{})
	b.Write16(bus.IOStart+0x0c6,
		(1<<12)|(1<<15),
		bus.Access{})

	p := ppu.New(b, ppu.Hooks{
		HBlank: func() { d.Trigger(dma.StartHBlank) },
		VBlank: func() { d.Trigger(dma.StartVBlank) },
	})

	p.Advance(ppu.VisibleCycles)
	if got, _ := b.Read16(hDest, bus.Access{}); got != 0x1111 {
		t.Fatalf("line0 HBlank DMA = %04x, want 1111", got)
	}
	if got, _ := b.Read16(vDest, bus.Access{}); got != 0 {
		t.Fatalf("VBlank DMA ran during line0 HBlank: %04x", got)
	}

	p.Advance(ppu.CyclesPerLine)
	if got, _ := b.Read16(hDest, bus.Access{}); got != 0x2222 {
		t.Fatalf("line1 HBlank repeated DMA = %04x, want 2222", got)
	}

	// We are at line1 HBlank (cycle 960). Finish line1, then advance through
	// the remaining visible lines to the line160 VBlank entry.
	p.Advance(ppu.CyclesPerLine - ppu.VisibleCycles)
	p.Advance(uint32(ppu.VBlankStartLine-2) * ppu.CyclesPerLine)
	if got, _ := b.Read16(vDest, bus.Access{}); got != 0xaaaa {
		t.Fatalf("VBlank DMA result = %04x, want aaaa", got)
	}

	if stall == 0 {
		t.Fatal("PPU-driven DMA did not report CPU stall cycles")
	}
}

func TestPPUScanlineStartDrivesDMA3VideoCapture(t *testing.T) {
	b := bus.New(nil, nil)
	d := dma.New(b, nil, dma.Hooks{})

	source := uint32(bus.IWRAMStart + 0x400)
	dest := uint32(bus.VRAMStart + 0x400)
	for i := 0; i < 160; i++ {
		b.Write16(source+uint32(i*2), uint16(i+1), bus.Access{})
	}

	// DMA3: one halfword per capture scanline, repeated from VCOUNT 2 through
	// VCOUNT 161. The final line auto-clears Enable.
	b.Write32(bus.IOStart+0x0d4, source, bus.Access{})
	b.Write32(bus.IOStart+0x0d8, dest, bus.Access{})
	b.Write16(bus.IOStart+0x0dc, 1, bus.Access{})
	b.Write16(bus.IOStart+0x0de,
		(1<<9)|(3<<12)|(1<<15),
		bus.Access{})

	p := ppu.New(b, ppu.Hooks{
		ScanlineStart: func(vcount uint16) { d.TriggerVideoCapture(vcount) },
	})

	// Line starts 0 and 1 are outside the capture window. The second complete
	// scanline advances VCOUNT to 2 and starts the first transfer.
	p.Advance(2 * ppu.CyclesPerLine)
	if p.VCount() != 2 {
		t.Fatalf("VCOUNT after two lines = %d, want 2", p.VCount())
	}
	if got, _ := b.Read16(dest, bus.Access{}); got != 1 {
		t.Fatalf("first PPU-driven capture = %04x, want 0001", got)
	}

	// Advance line starts 3..161. That produces 159 more bursts, for 160 total.
	p.Advance(159 * ppu.CyclesPerLine)
	if p.VCount() != 161 {
		t.Fatalf("VCOUNT at final capture = %d, want 161", p.VCount())
	}
	if got, _ := b.Read16(dest+159*2, bus.Access{}); got != 160 {
		t.Fatalf("last PPU-driven capture = %04x, want 00a0", got)
	}
	if d.Control(3)&(1<<15) != 0 {
		t.Fatal("DMA3 video capture remained enabled after VCOUNT 161")
	}

	// Entering VCOUNT 162 cannot start another capture burst.
	p.Advance(ppu.CyclesPerLine)
	if p.VCount() != 162 {
		t.Fatalf("VCOUNT after capture window = %d, want 162", p.VCount())
	}
	if got, _ := b.Read16(dest+160*2, bus.Access{}); got != 0 {
		t.Fatalf("capture continued into VCOUNT 162: %04x", got)
	}
}

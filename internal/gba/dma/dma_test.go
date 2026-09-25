package dma

import (
	"encoding/binary"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

func dmaBase(index int) uint32 {
	return bus.IOStart + firstDMAOffset + uint32(index*dmaStride)
}

func programDMA(b *bus.Bus, index int, source, dest uint32, count uint16, control uint16) {
	base := dmaBase(index)
	b.Write32(base, source, bus.Access{})
	b.Write32(base+4, dest, bus.Access{})
	b.Write16(base+8, count, bus.Access{})
	b.Write16(base+10, control, bus.Access{})
}

func TestDMARegisterMasks(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	for index := 0; index < 4; index++ {
		base := dmaBase(index)
		b.Write32(base, 0xffffffff, bus.Access{})
		b.Write32(base+4, 0xffffffff, bus.Access{})
		b.Write16(base+8, 0xffff, bus.Access{})
		// Timing=Special prevents an immediate transfer while still testing
		// all readable control bits including Enable.
		b.Write16(base+10, 0xffff, bus.Access{})

		if got := d.Source(index); got != sourceMask(index) {
			t.Fatalf("DMA%d SAD = %08x, want %08x", index, got, sourceMask(index))
		}
		if got := d.Destination(index); got != destMask(index) {
			t.Fatalf("DMA%d DAD = %08x, want %08x", index, got, destMask(index))
		}
		if got := d.Count(index); got != countMask(index) {
			t.Fatalf("DMA%d count = %04x, want %04x", index, got, countMask(index))
		}
		if got := d.Control(index); got != controlMask(index) {
			t.Fatalf("DMA%d control = %04x, want %04x", index, got, controlMask(index))
		}
	}
}

func TestDMAWriteOnlyRegistersReadOpenBus(t *testing.T) {
	b := bus.New(nil, nil)
	_ = New(b, nil, Hooks{})

	for _, offset := range []uint32{0x0b0, 0x0b2, 0x0b4, 0x0b6, 0x0b8} {
		b.SetOpenBus(0x44332211)
		got, _ := b.Read16(bus.IOStart+offset, bus.Access{})
		want := uint16(0x2211)
		if offset&2 != 0 {
			want = 0x4433
		}
		if got != want {
			t.Fatalf("read %03x = %04x, want open-bus %04x", offset, got, want)
		}
	}
}

func TestDMAByteWritesMergeAgainstProgrammedLatches(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	base := dmaBase(1)
	b.Write32(base, 0x01234567, bus.Access{})
	b.Write8(base+1, 0xaa, bus.Access{})
	b.Write8(base+3, 0x0b, bus.Access{})
	if got := d.Source(1); got != 0x0b23aa67 {
		t.Fatalf("byte-written SAD = %08x, want 0b23aa67", got)
	}

	b.Write16(base+8, 0x1234, bus.Access{})
	b.Write8(base+8, 0xcd, bus.Access{})
	b.Write8(base+9, 0x2a, bus.Access{})
	if got := d.Count(1); got != 0x2acd {
		t.Fatalf("byte-written count = %04x, want 2acd", got)
	}
}

func TestDMAImmediate16BitIncrementCopy(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x100)
	dest := uint32(bus.IWRAMStart + 0x200)
	for i, value := range []uint16{0x1111, 0x2222, 0x3333} {
		b.Write16(source+uint32(i*2), value, bus.Access{})
	}

	programDMA(b, 0, source, dest, 3, controlEnable)

	for i, want := range []uint16{0x1111, 0x2222, 0x3333} {
		got, _ := b.Read16(dest+uint32(i*2), bus.Access{})
		if got != want {
			t.Fatalf("DMA copy word%d = %04x, want %04x", i, got, want)
		}
	}

	if d.Control(0)&controlEnable != 0 {
		t.Fatal("immediate DMA did not auto-clear enable")
	}
	if d.Source(0) != source || d.Destination(0) != dest || d.Count(0) != 3 {
		t.Fatalf("programmed registers changed SAD=%08x DAD=%08x count=%04x",
			d.Source(0), d.Destination(0), d.Count(0))
	}
}

func TestDMAImmediate32BitAlignsSourceAndDestination(t *testing.T) {
	b := bus.New(nil, nil)
	_ = New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x300)
	dest := uint32(bus.IWRAMStart + 0x400)
	b.Write32(source, 0x44332211, bus.Access{})

	// DMA word addresses ignore bits 0-1; unlike CPU LDR, there is no rotated
	// result from the unaligned source.
	programDMA(b, 3, source+2, dest+2, 1, controlEnable|controlWord)

	got, _ := b.Read32(dest, bus.Access{})
	if got != 0x44332211 {
		t.Fatalf("aligned DMA32 value = %08x, want 44332211", got)
	}
}

func TestDMASourceFixedAndDestinationFixedModes(t *testing.T) {
	t.Run("source fixed", func(t *testing.T) {
		b := bus.New(nil, nil)
		_ = New(b, nil, Hooks{})
		source := uint32(bus.EWRAMStart + 0x500)
		dest := uint32(bus.IWRAMStart + 0x500)
		b.Write16(source, 0x5a5a, bus.Access{})

		programDMA(b, 0, source, dest, 3, controlEnable|(2<<7))
		for i := 0; i < 3; i++ {
			got, _ := b.Read16(dest+uint32(i*2), bus.Access{})
			if got != 0x5a5a {
				t.Fatalf("fixed-source word%d = %04x", i, got)
			}
		}
	})

	t.Run("destination fixed", func(t *testing.T) {
		b := bus.New(nil, nil)
		_ = New(b, nil, Hooks{})
		source := uint32(bus.EWRAMStart + 0x540)
		dest := uint32(bus.IWRAMStart + 0x540)
		for i, value := range []uint16{1, 2, 3} {
			b.Write16(source+uint32(i*2), value, bus.Access{})
		}

		programDMA(b, 0, source, dest, 3, controlEnable|(2<<5))
		got, _ := b.Read16(dest, bus.Access{})
		if got != 3 {
			t.Fatalf("fixed-destination final value = %04x, want 0003", got)
		}
	})
}

func TestDMADecrementAddressing(t *testing.T) {
	b := bus.New(nil, nil)
	_ = New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x608)
	dest := uint32(bus.IWRAMStart + 0x708)
	for i, value := range []uint16{0xaaaa, 0xbbbb, 0xcccc} {
		b.Write16(source-4+uint32(i*2), value, bus.Access{})
	}

	programDMA(b, 0, source, dest, 3, controlEnable|(1<<7)|(1<<5))
	for i, want := range []uint16{0xaaaa, 0xbbbb, 0xcccc} {
		got, _ := b.Read16(dest-4+uint32(i*2), bus.Access{})
		if got != want {
			t.Fatalf("decrement DMA at slot%d = %04x, want %04x", i, got, want)
		}
	}
}

func TestDMADestinationIncrementReloadIncrementsDuringImmediateRun(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x800)
	dest := uint32(bus.IWRAMStart + 0x800)
	for i, value := range []uint16{0x1001, 0x1002, 0x1003} {
		b.Write16(source+uint32(i*2), value, bus.Access{})
	}

	programDMA(b, 0, source, dest, 3, controlEnable|(3<<5))
	for i, want := range []uint16{0x1001, 0x1002, 0x1003} {
		got, _ := b.Read16(dest+uint32(i*2), bus.Access{})
		if got != want {
			t.Fatalf("inc/reload destination word%d = %04x, want %04x", i, got, want)
		}
	}
	if d.Destination(0) != dest {
		t.Fatalf("programmed DAD changed = %08x, want %08x", d.Destination(0), dest)
	}
}

func TestDMAZeroCountUsesChannelMaximum(t *testing.T) {
	for index := 0; index < 4; index++ {
		want := uint32(0x4000)
		if index == 3 {
			want = 0x10000
		}
		if got := effectiveCount(index, 0); got != want {
			t.Fatalf("DMA%d zero count = %d, want %d", index, got, want)
		}
	}
}

func TestDMAImmediateRepeatStillCompletesOneShot(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})
	source := uint32(bus.EWRAMStart + 0x900)
	dest := uint32(bus.IWRAMStart + 0x900)
	b.Write16(source, 0xbeef, bus.Access{})

	programDMA(b, 0, source, dest, 1, controlEnable|controlRepeat)
	if d.Control(0)&controlEnable != 0 {
		t.Fatalf("immediate repeat left enable set: control=%04x", d.Control(0))
	}
	if d.Control(0)&controlRepeat == 0 {
		t.Fatalf("repeat configuration bit was lost: control=%04x", d.Control(0))
	}
}

func TestNonImmediateDMAArmsWithoutTransferring(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})
	source := uint32(bus.EWRAMStart + 0xa00)
	dest := uint32(bus.IWRAMStart + 0xa00)
	b.Write16(source, 0x1234, bus.Access{})

	programDMA(b, 0, source, dest, 1, controlEnable|(1<<12)) // VBlank timing
	got, _ := b.Read16(dest, bus.Access{})
	if got != 0 {
		t.Fatalf("VBlank-armed DMA transferred immediately: %04x", got)
	}
	if d.Control(0)&controlEnable == 0 {
		t.Fatal("non-immediate DMA did not remain armed")
	}
	if d.ch[0].sourceCurrent != source || d.ch[0].destCurrent != dest || d.ch[0].countCurrent != 1 {
		t.Fatalf("non-immediate DMA did not latch internal state: %+v", d.ch[0])
	}
}

func TestDMASequentialGamePakTimingAndCompletionHook(t *testing.T) {
	rom := make([]byte, 4)
	binary.LittleEndian.PutUint16(rom[0:2], 0x1111)
	binary.LittleEndian.PutUint16(rom[2:4], 0x2222)
	b := bus.New(nil, rom)

	var hookChannel int
	var hookUnits, hookCycles uint32
	d := New(b, nil, Hooks{Complete: func(channel int, units, cycles uint32) {
		hookChannel = channel
		hookUnits = units
		hookCycles = cycles
	}})

	dest := uint32(bus.IWRAMStart + 0xb00)
	programDMA(b, 1, bus.ROM0Start, dest, 2, controlEnable)

	// WS0 default: first 16-bit ROM read 5 cycles, sequential second read 3;
	// IWRAM writes cost 1 each; DMA processing adds 2 internal cycles.
	units, cycles := d.LastTransfer(1)
	if units != 2 || cycles != 12 {
		t.Fatalf("DMA1 timing units=%d cycles=%d, want 2/12", units, cycles)
	}
	if hookChannel != 1 || hookUnits != 2 || hookCycles != 12 {
		t.Fatalf("completion hook channel=%d units=%d cycles=%d", hookChannel, hookUnits, hookCycles)
	}
}

func TestDMACompletionRequestsChannelIRQ(t *testing.T) {
	b := bus.New(nil, nil)
	irq := gbairq.New(b, nil)
	_ = New(b, irq, Hooks{})

	source := uint32(bus.EWRAMStart + 0xc00)
	dest := uint32(bus.IWRAMStart + 0xc00)
	b.Write16(source, 0xface, bus.Access{})

	programDMA(b, 2, source, dest, 1, controlEnable|controlIRQ)
	if got := irq.IF(); got != uint16(gbairq.DMA2) {
		t.Fatalf("IF after DMA2 completion = %04x, want DMA2", got)
	}
}

func TestDMAResetClearsAllState(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})
	programDMA(b, 0, bus.EWRAMStart, bus.IWRAMStart, 1, controlEnable)
	d.Reset()

	for i := 0; i < 4; i++ {
		if d.Source(i) != 0 || d.Destination(i) != 0 || d.Count(i) != 0 || d.Control(i) != 0 {
			t.Fatalf("DMA%d reset state SAD=%08x DAD=%08x count=%04x control=%04x",
				i, d.Source(i), d.Destination(i), d.Count(i), d.Control(i))
		}
	}
}

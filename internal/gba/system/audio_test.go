package system

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/audio"
	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestDirectSoundTimerOverflowSchedulesFIFORefillDMA(t *testing.T) {
	m := New(nil, nil)

	// Leave FIFO A on timer 0 and start with 17 queued bytes so one overflow
	// reaches the hardware refill threshold.
	for i := 0; i < 4; i++ {
		m.Bus.Write32(bus.IOStart+0x0a0, uint32(i+1)*0x01010101, bus.Access{})
	}
	m.Bus.Write8(bus.IOStart+0x0a0, 0x11, bus.Access{})
	if got := m.Audio.FIFOLevel(audio.FIFOA); got != 17 {
		t.Fatalf("FIFO A setup level = %d, want 17", got)
	}

	source := uint32(bus.IWRAMStart + 0x7000)
	for i := 0; i < 4; i++ {
		m.Bus.Write32(source+uint32(i*4), 0x80818283+uint32(i), bus.Access{})
	}
	// DMA1, repeat, special timing, destination FIFO A.
	programDMA(m, 1, source, bus.IOStart+0x0a0, 1, (1<<9)|(3<<12)|(1<<15))

	// Timer 0 overflows every 64 cycles.
	m.Bus.Write16(bus.IOStart+0x100, 0xffc0, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, 1<<7, bus.Access{})

	m.Advance(64)
	if got := m.Audio.FIFOLevel(audio.FIFOA); got != 16 {
		t.Fatalf("FIFO A after timer overflow = %d, want 16", got)
	}
	if !m.DMA.Pending() {
		t.Fatal("half-full FIFO A did not queue Direct Sound DMA")
	}
	if got := m.Audio.CurrentSample(audio.FIFOA); got != 1 {
		t.Fatalf("FIFO A current sample = %d, want 1", got)
	}

	beforeStalls := m.DMAStallCycles()
	m.Advance(uint32(DMAStartLatency))
	if got := m.Audio.FIFOLevel(audio.FIFOA); got != 32 {
		t.Fatalf("FIFO A after DMA refill = %d, want 32", got)
	}
	if delta := m.DMAStallCycles() - beforeStalls; delta == 0 {
		t.Fatal("FIFO refill did not stall CPU through scheduler")
	}
	if units, _ := m.DMA.LastTransfer(1); units != 4 {
		t.Fatalf("Direct Sound DMA units = %d, want 4", units)
	}
}

func TestDirectSoundTimerSelectionUsesTimer1ForFIFOB(t *testing.T) {
	m := New(nil, nil)

	m.Bus.Write32(bus.IOStart+0x0a0, 0x04030201, bus.Access{})
	m.Bus.Write32(bus.IOStart+0x0a4, 0x08070605, bus.Access{})
	// Select timer 1 only for FIFO B.
	m.Bus.Write16(bus.IOStart+0x082, 1<<14, bus.Access{})

	// Timer 0 overflows after 4 cycles. Timer 1 is count-up and receives that
	// overflow, itself overflowing because its counter starts at ffff.
	m.Bus.Write16(bus.IOStart+0x100, 0xfffc, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x104, 0xffff, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x106, (1<<2)|(1<<7), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, 1<<7, bus.Access{})

	m.Advance(4)
	if got := m.Audio.FIFOLevel(audio.FIFOA); got != 3 {
		t.Fatalf("FIFO A timer-0 level = %d, want 3", got)
	}
	if got := m.Audio.FIFOLevel(audio.FIFOB); got != 3 {
		t.Fatalf("FIFO B timer-1 level = %d, want 3", got)
	}
	if got := m.Audio.CurrentSample(audio.FIFOA); got != 1 {
		t.Fatalf("FIFO A sample = %d, want 1", got)
	}
	if got := m.Audio.CurrentSample(audio.FIFOB); got != 5 {
		t.Fatalf("FIFO B sample = %d, want 5", got)
	}
}

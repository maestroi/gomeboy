package system

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
)

const armNOP uint32 = 0xe1a00000

func writeNOPs(m *Machine, addr uint32, count int) {
	for i := 0; i < count; i++ {
		m.Bus.Write32(addr+uint32(i*4), armNOP, bus.Access{})
	}
	m.CPU.SetPC(addr)
}

func programDMA(m *Machine, index int, source, dest uint32, count uint16, control uint16) {
	base := bus.IOStart + 0x0b0 + uint32(index*12)
	m.Bus.Write32(base, source, bus.Access{})
	m.Bus.Write32(base+4, dest, bus.Access{})
	m.Bus.Write16(base+8, count, bus.Access{})
	m.Bus.Write16(base+10, control, bus.Access{})
}

func TestImmediateDMAStartsAfterTwoCyclesAndStallsCPU(t *testing.T) {
	m := New(nil, nil)
	code := uint32(bus.IWRAMStart + 0x100)
	writeNOPs(m, code, 4)

	// Timer0 gives us an independent witness that peripheral time continues
	// through both CPU execution and the DMA-owned bus interval.
	m.Bus.Write16(bus.IOStart+0x100, 0xfff0, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, 1<<7, bus.Access{})

	source := uint32(bus.IWRAMStart + 0x400)
	dest := uint32(bus.IWRAMStart + 0x500)
	m.Bus.Write16(source, 0x55aa, bus.Access{})
	programDMA(m, 0, source, dest, 1, 1<<15)

	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("immediate DMA ran on Enable: %04x", got)
	}
	if !m.DMA.Pending() {
		t.Fatal("immediate DMA was not queued for scheduler service")
	}

	first, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if first.ElapsedCycles != 1 || first.DMAStallCycles != 0 {
		t.Fatalf("first CPU step elapsed/stall = %d/%d, want 1/0",
			first.ElapsedCycles, first.DMAStallCycles)
	}
	if m.Cycle() != 1 {
		t.Fatalf("cycle after first CPU step = %d, want 1", m.Cycle())
	}
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("DMA started before two-cycle latency elapsed: %04x", got)
	}

	second, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if second.CPU.TotalCycles != 1 {
		t.Fatalf("second CPU instruction cycles = %d, want 1", second.CPU.TotalCycles)
	}
	// IWRAM->IWRAM halfword DMA is 1 read + 1 write + 2 internal = 4 cycles.
	if second.DMAStallCycles != 4 || second.ElapsedCycles != 5 {
		t.Fatalf("second CPU step elapsed/stall = %d/%d, want 5/4",
			second.ElapsedCycles, second.DMAStallCycles)
	}
	if m.Cycle() != 6 {
		t.Fatalf("cycle after DMA-stalled step = %d, want 6", m.Cycle())
	}
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0x55aa {
		t.Fatalf("scheduled DMA result = %04x, want 55aa", got)
	}
	if got := m.Timers.Counter(0); got != 0xfff6 {
		t.Fatalf("timer during CPU+DMA time = %04x, want fff6", got)
	}
	if got := m.PPU.LineCycle(); got != 6 {
		t.Fatalf("PPU cycle during CPU+DMA time = %d, want 6", got)
	}
	if m.CPU.PC() != code+8 {
		t.Fatalf("CPU executed during DMA stall: PC=%08x, want %08x", m.CPU.PC(), code+8)
	}
}

func TestPPUHBlankDMAUsesCentralStartLatency(t *testing.T) {
	m := New(nil, nil)

	source := uint32(bus.IWRAMStart + 0x600)
	dest := uint32(bus.IWRAMStart + 0x700)
	m.Bus.Write16(source, 0xbeef, bus.Access{})
	// DMA1, one-shot HBlank.
	programDMA(m, 1, source, dest, 1, (2<<12)|(1<<15))

	m.Advance(ppu.VisibleCycles - 1)
	if m.Cycle() != uint64(ppu.VisibleCycles-1) {
		t.Fatalf("cycle before HBlank = %d", m.Cycle())
	}

	m.Advance(1) // HBlank request at cycle 960.
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("HBlank DMA ignored start latency: %04x", got)
	}
	if !m.DMA.Pending() {
		t.Fatal("HBlank edge did not queue DMA")
	}

	m.Advance(1)
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("HBlank DMA started after only one latency cycle: %04x", got)
	}

	m.Advance(1) // cycle 962 start, then four DMA stall cycles.
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0xbeef {
		t.Fatalf("HBlank DMA result = %04x, want beef", got)
	}
	if m.Cycle() != uint64(ppu.VisibleCycles)+DMAStartLatency+4 {
		t.Fatalf("cycle after HBlank DMA = %d, want %d",
			m.Cycle(), uint64(ppu.VisibleCycles)+DMAStartLatency+4)
	}
	if m.PPU.LineCycle() != uint32(m.Cycle()) {
		t.Fatalf("PPU did not advance through DMA stall: lineCycle=%d cycle=%d",
			m.PPU.LineCycle(), m.Cycle())
	}
}

func TestCentralClockDeliversTimerIRQAtOverflowEdge(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x100, 0xfffe, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, (1<<6)|(1<<7), bus.Access{})

	m.Advance(1)
	if got := m.Timers.Counter(0); got != 0xffff {
		t.Fatalf("timer after one cycle = %04x, want ffff", got)
	}
	if m.IRQ.IF() != 0 || m.CPU.IRQLine() {
		t.Fatalf("timer IRQ arrived early: IF=%04x line=%v", m.IRQ.IF(), m.CPU.IRQLine())
	}

	m.Advance(1)
	if got := m.Timers.Counter(0); got != 0xfffe {
		t.Fatalf("timer after overflow = %04x, want reload fffe", got)
	}
	if m.IRQ.IF() != uint16(gbairq.Timer0) || !m.CPU.IRQLine() {
		t.Fatalf("timer overflow IRQ IF=%04x line=%v", m.IRQ.IF(), m.CPU.IRQLine())
	}
	if m.Cycle() != 2 {
		t.Fatalf("timer overflow cycle = %d, want 2", m.Cycle())
	}
}

func TestIRQExceptionInternalCycleAdvancesCentralClock(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	m.CPU.SetPC(bus.IWRAMStart)
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.VBlank), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.IRQ.Request(gbairq.VBlank)

	result, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !result.CPU.ExceptionTaken || result.CPU.Exception != cpu.ExceptionIRQ {
		t.Fatalf("IRQ step did not take exception: %+v", result.CPU)
	}
	if result.ElapsedCycles != 1 || m.Cycle() != 1 {
		t.Fatalf("IRQ entry elapsed/cycle = %d/%d, want 1/1",
			result.ElapsedCycles, m.Cycle())
	}
	if m.PPU.LineCycle() != 1 {
		t.Fatalf("PPU did not advance during IRQ entry: %d", m.PPU.LineCycle())
	}
}

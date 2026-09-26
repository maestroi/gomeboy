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
	// This ARM data-processing NOP is one IWRAM fetch plus one internal cycle.
	// The DMA deadline therefore lands inside this instruction: after its two
	// CPU-owned cycles, DMA takes the bus for four cycles before Step returns.
	if first.CPU.TotalCycles != 2 {
		t.Fatalf("first CPU instruction cycles = %d, want 2", first.CPU.TotalCycles)
	}
	// IWRAM->IWRAM halfword DMA is 1 read + 1 write + 2 internal = 4 cycles.
	if first.DMAStallCycles != 4 || first.ElapsedCycles != 6 {
		t.Fatalf("first CPU step elapsed/stall = %d/%d, want 6/4",
			first.ElapsedCycles, first.DMAStallCycles)
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
	if m.CPU.PC() != code+4 {
		t.Fatalf("CPU executed during DMA stall: PC=%08x, want %08x", m.CPU.PC(), code+4)
	}

	second, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if second.DMAStallCycles != 0 {
		t.Fatalf("completed DMA stalled the next instruction by %d cycles", second.DMAStallCycles)
	}
	if m.CPU.PC() != code+8 {
		t.Fatalf("second instruction PC=%08x, want %08x", m.CPU.PC(), code+8)
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

func TestCentralClockDelaysTimerIRQPropagation(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x100, 0xfffe, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, (1<<6)|(1<<7), bus.Access{})

	m.Advance(2)
	if got := m.Timers.Counter(0); got != 0xfffe {
		t.Fatalf("timer after overflow = %04x, want reload fffe", got)
	}
	if m.IRQ.IF() != uint16(gbairq.Timer0) {
		t.Fatalf("timer IF at overflow = %04x, want Timer0", m.IRQ.IF())
	}
	if m.CPU.IRQLine() {
		t.Fatal("timer IRQ reached CPU without propagation delay")
	}

	m.Advance(uint32(IRQPropagationLatency - 1))
	if m.CPU.IRQLine() {
		t.Fatal("timer IRQ reached CPU one cycle before propagation deadline")
	}

	m.Advance(1)
	if !m.CPU.IRQLine() {
		t.Fatal("timer IRQ did not reach CPU at propagation deadline")
	}
	if want := uint64(2) + IRQPropagationLatency; m.Cycle() != want {
		t.Fatalf("IRQ delivery cycle = %d, want %d", m.Cycle(), want)
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

	m.Advance(uint32(IRQPropagationLatency))
	if !m.CPU.IRQLine() {
		t.Fatal("IRQ line was not delivered after propagation latency")
	}

	result, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !result.CPU.ExceptionTaken || result.CPU.Exception != cpu.ExceptionIRQ {
		t.Fatalf("IRQ step did not take exception: %+v", result.CPU)
	}
	if result.ElapsedCycles != 1 || m.Cycle() != IRQPropagationLatency+1 {
		t.Fatalf("IRQ entry elapsed/cycle = %d/%d, want 1/%d",
			result.ElapsedCycles, m.Cycle(), IRQPropagationLatency+1)
	}
	if m.PPU.LineCycle() != uint32(IRQPropagationLatency+1) {
		t.Fatalf("PPU did not advance during IRQ delay+entry: %d", m.PPU.LineCycle())
	}
}

func TestCPURegisterWriteStartsLatencyAfterBusAccess(t *testing.T) {
	m := New(nil, nil)

	source := uint32(bus.IWRAMStart + 0x1b00)
	dest := uint32(bus.IWRAMStart + 0x1c00)
	m.Bus.Write16(source, 0xcafe, bus.Access{})

	base := uint32(bus.IOStart + 0x0b0)
	m.Bus.Write32(base, source, bus.Access{})
	m.Bus.Write32(base+4, dest, bus.Access{})
	m.Bus.Write16(base+8, 1, bus.Access{})

	// Use the CPU-facing timed memory adapter for CNT_H. I/O takes one cycle;
	// the two-cycle DMA start delay begins after that write has completed.
	if cycles := m.memory.Write16(base+10, 1<<15, bus.Access{}); cycles != 1 {
		t.Fatalf("DMA control write cycles = %d, want 1", cycles)
	}
	if m.Cycle() != 1 {
		t.Fatalf("cycle after DMA control write = %d, want 1", m.Cycle())
	}

	m.Advance(1)
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("DMA started one cycle after control write: %04x", got)
	}
	m.Advance(1)
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0xcafe {
		t.Fatalf("DMA did not start two cycles after control write: %04x", got)
	}
	// 1 write + 2 latency + 4 transfer = cycle 7.
	if m.Cycle() != 7 {
		t.Fatalf("cycle after CPU-programmed DMA = %d, want 7", m.Cycle())
	}
}

func TestDisableAndReenableDMAResetsStartLatency(t *testing.T) {
	m := New(nil, nil)

	source := uint32(bus.IWRAMStart + 0x1d00)
	dest := uint32(bus.IWRAMStart + 0x1e00)
	m.Bus.Write16(source, 0x1234, bus.Access{})
	base := uint32(bus.IOStart + 0x0b0)
	m.Bus.Write32(base, source, bus.Access{})
	m.Bus.Write32(base+4, dest, bus.Access{})
	m.Bus.Write16(base+8, 1, bus.Access{})

	m.Bus.Write16(base+10, 1<<15, bus.Access{}) // due at cycle 2
	m.Advance(1)
	m.Bus.Write16(base+10, 0, bus.Access{})     // cancel old request at cycle 1
	m.Bus.Write16(base+10, 1<<15, bus.Access{}) // new deadline is cycle 3

	m.Advance(1)
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("re-enabled DMA inherited old deadline: %04x", got)
	}
	m.Advance(1)
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0x1234 {
		t.Fatalf("re-enabled DMA missed fresh deadline: %04x", got)
	}
}

func enterHALT(t *testing.T, m *Machine) {
	t.Helper()
	m.CPU.SetPC(bus.BIOSStart)
	m.Bus.Write8(bus.IOStart+0x300, 1, bus.Access{})
	m.Bus.Write8(bus.IOStart+0x301, 0, bus.Access{})
	if !m.Halted() {
		t.Fatal("HALTCNT did not enter HALT")
	}
}

func stepUntilWake(t *testing.T, m *Machine, maxSteps int) (StepResult, uint64) {
	t.Helper()
	var elapsed uint64
	for i := 0; i < maxSteps; i++ {
		result, err := m.Step()
		if err != nil {
			t.Fatal(err)
		}
		elapsed += result.ElapsedCycles
		if result.Woke {
			return result, elapsed
		}
		if !result.Halted {
			t.Fatalf("HALT ended without wake event after %d steps", i+1)
		}
	}
	t.Fatalf("HALT did not wake within %d scheduler boundaries", maxSteps)
	return StepResult{}, elapsed
}

func TestHALTCNTRequiresBIOSExecution(t *testing.T) {
	m := New(nil, nil)

	// POSTFLG is established by BIOS boot.
	m.CPU.SetPC(bus.BIOSStart)
	m.Bus.Write8(bus.IOStart+0x300, 1, bus.Access{})

	m.CPU.SetPC(bus.IWRAMStart)
	m.Bus.Write8(bus.IOStart+0x301, 0, bus.Access{})
	if m.Halted() {
		t.Fatal("non-BIOS HALTCNT write entered HALT")
	}

	m.CPU.SetPC(bus.BIOSStart)
	m.Bus.Write8(bus.IOStart+0x301, 0, bus.Access{})
	if !m.Halted() {
		t.Fatal("BIOS HALTCNT write did not enter HALT")
	}
}

func TestHALTWakesOnEnabledTimerRequestWithIMEClear(t *testing.T) {
	m := New(nil, nil)
	code := uint32(bus.IWRAMStart + 0x2000)
	writeNOPs(m, code, 2)

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	// Leave IME clear deliberately.
	m.Bus.Write16(bus.IOStart+0x100, 0xfffe, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, (1<<6)|(1<<7), bus.Access{})

	enterHALT(t, m)
	m.CPU.SetPC(code)
	pc := m.CPU.PC()

	request, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !request.Halted || request.Woke || request.ElapsedCycles != 2 {
		t.Fatalf("timer request boundary = halted:%v woke:%v elapsed:%d, want true/false/2",
			request.Halted, request.Woke, request.ElapsedCycles)
	}
	if m.IRQ.IF() != uint16(gbairq.Timer0) || m.CPU.IRQLine() {
		t.Fatalf("timer request state IF=%04x line=%v", m.IRQ.IF(), m.CPU.IRQLine())
	}

	wake, propagation := stepUntilWake(t, m, 8)
	if !wake.Woke || wake.Halted || m.Halted() {
		t.Fatalf("timer wake result woke/halted = %v/%v machine=%v",
			wake.Woke, wake.Halted, m.Halted())
	}
	if propagation != IRQPropagationLatency {
		t.Fatalf("HALT IRQ propagation = %d cycles across scheduler boundaries, want %d",
			propagation, IRQPropagationLatency)
	}
	if m.CPU.PC() != pc {
		t.Fatalf("CPU executed while fast-forwarding HALT: PC=%08x want %08x", m.CPU.PC(), pc)
	}
	if m.CPU.IRQLine() {
		t.Fatal("IME-clear HALT wake asserted CPU IRQ line")
	}

	next, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if next.CPU.ExceptionTaken {
		t.Fatal("IME-clear wake incorrectly took IRQ exception")
	}
	if m.CPU.PC() != code+4 {
		t.Fatalf("CPU did not resume normal execution after wake: PC=%08x", m.CPU.PC())
	}
}

func TestHALTWakeThenTakesIRQWhenIMEEnabled(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	code := uint32(bus.IWRAMStart + 0x2100)
	writeNOPs(m, code, 1)

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x100, 0xffff, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, (1<<6)|(1<<7), bus.Access{})

	enterHALT(t, m)
	m.CPU.SetPC(code)

	request, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if request.Woke || !request.Halted || request.ElapsedCycles != 1 {
		t.Fatalf("timer request step = woke:%v halted:%v elapsed:%d, want false/true/1",
			request.Woke, request.Halted, request.ElapsedCycles)
	}
	if m.CPU.IRQLine() {
		t.Fatal("IRQ line asserted before propagation deadline")
	}

	wake, propagation := stepUntilWake(t, m, 10)
	if !wake.Woke || propagation != IRQPropagationLatency {
		t.Fatalf("HALT IRQ wake = woke:%v propagation:%d, want true/%d",
			wake.Woke, propagation, IRQPropagationLatency)
	}
	if !m.CPU.IRQLine() {
		t.Fatal("IRQ line was not asserted at HALT wake event")
	}
	if m.CPU.PC() != code {
		t.Fatalf("wake boundary executed CPU: PC=%08x want %08x", m.CPU.PC(), code)
	}

	irqStep, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !irqStep.CPU.ExceptionTaken || irqStep.CPU.Exception != cpu.ExceptionIRQ {
		t.Fatalf("post-wake step did not take IRQ: %+v", irqStep.CPU)
	}
	if m.CPU.PC() != 0x18 {
		t.Fatalf("IRQ vector PC = %08x, want 00000018", m.CPU.PC())
	}
}

func TestHALTRequiresIEAndIFIntersection(t *testing.T) {
	m := New(nil, nil)
	code := uint32(bus.IWRAMStart + 0x2200)
	writeNOPs(m, code, 1)

	m.IRQ.Request(gbairq.Timer1)
	enterHALT(t, m)
	m.CPU.SetPC(code)

	result, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Halted || result.Woke {
		t.Fatalf("IF-only HALT result halted/woke = %v/%v, want true/false",
			result.Halted, result.Woke)
	}
	if result.ElapsedCycles != uint64(ppu.VisibleCycles) {
		t.Fatalf("IF-only HALT did not jump to next PPU edge: %d, want %d",
			result.ElapsedCycles, ppu.VisibleCycles)
	}
	if m.CPU.PC() != code {
		t.Fatalf("CPU executed while IF lacked IE: PC=%08x want %08x", m.CPU.PC(), code)
	}

	// Enabling the already-pending source starts the seven-cycle wake event.
	// IME remains clear, so wake will not assert the CPU IRQ line.
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer1), bus.Access{})
	wake, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !wake.Woke || wake.ElapsedCycles != IRQPropagationLatency || m.Halted() {
		t.Fatalf("enabled pending wake = woke:%v elapsed:%d halted:%v",
			wake.Woke, wake.ElapsedCycles, m.Halted())
	}
	if m.CPU.IRQLine() {
		t.Fatal("IME-clear enabled-pending wake asserted IRQ line")
	}
}

func TestHALTServicesDeferredDMAAndWakesOnDMAIRQ(t *testing.T) {
	m := New(nil, nil)

	source := uint32(bus.IWRAMStart + 0x2300)
	dest := uint32(bus.IWRAMStart + 0x2400)
	m.Bus.Write16(source, 0x5aa5, bus.Access{})
	// DMA1, one-shot HBlank with completion IRQ.
	programDMA(m, 1, source, dest, 1, (2<<12)|(1<<14)|(1<<15))
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.DMA1), bus.Access{})

	m.Advance(ppu.VisibleCycles - 1)
	enterHALT(t, m)
	m.CPU.SetPC(bus.IWRAMStart + 0x2500)
	pc := m.CPU.PC()

	first, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Halted || first.Woke || first.ElapsedCycles != 1 {
		t.Fatalf("HBlank request step = halted:%v woke:%v elapsed:%d, want true/false/1",
			first.Halted, first.Woke, first.ElapsedCycles)
	}
	if !m.DMA.Pending() {
		t.Fatal("HBlank while halted did not queue DMA")
	}

	second, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if second.Woke || !second.Halted {
		t.Fatalf("DMA completion boundary = woke:%v halted:%v, want false/true",
			second.Woke, second.Halted)
	}
	if second.DMAStallCycles != 4 || second.ElapsedCycles != 6 {
		t.Fatalf("halted DMA elapsed/stall = %d/%d, want 6/4",
			second.ElapsedCycles, second.DMAStallCycles)
	}
	if got, _ := m.Bus.Read16(dest, bus.Access{}); got != 0x5aa5 {
		t.Fatalf("halted DMA result = %04x, want 5aa5", got)
	}
	if m.IRQ.IF()&uint16(gbairq.DMA1) == 0 {
		t.Fatalf("DMA1 IF missing after halted transfer: %04x", m.IRQ.IF())
	}

	third, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !third.Woke || third.Halted || third.ElapsedCycles != IRQPropagationLatency {
		t.Fatalf("DMA IRQ wake = woke:%v halted:%v elapsed:%d, want true/false/%d",
			third.Woke, third.Halted, third.ElapsedCycles, IRQPropagationLatency)
	}
	if m.CPU.PC() != pc {
		t.Fatalf("CPU executed during halted DMA/IRQ service: PC=%08x want %08x", m.CPU.PC(), pc)
	}
}

func TestSTOPIsRecognizedButNotMisimplementedAsHALT(t *testing.T) {
	m := New(nil, nil)
	m.CPU.SetPC(bus.BIOSStart)
	m.Bus.Write8(bus.IOStart+0x300, 1, bus.Access{})
	m.Bus.Write8(bus.IOStart+0x301, 0x80, bus.Access{})

	if !m.StopRequested() {
		t.Fatal("HALTCNT STOP request was not recorded")
	}
	if m.Halted() {
		t.Fatal("STOP request was incorrectly treated as ordinary HALT")
	}
}

func TestIRQAcknowledgedDuringPropagationDoesNotReachCPU(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.VBlank), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.IRQ.Request(gbairq.VBlank)

	m.Advance(3)
	if m.CPU.IRQLine() {
		t.Fatal("IRQ line asserted before propagation deadline")
	}
	m.Bus.Write16(bus.IOStart+0x202, uint16(gbairq.VBlank), bus.Access{})
	if m.IRQ.IF() != 0 {
		t.Fatalf("IF acknowledge failed during propagation: %04x", m.IRQ.IF())
	}

	m.Advance(4)
	if m.CPU.IRQLine() {
		t.Fatal("acknowledged IRQ asserted CPU line at stale delivery event")
	}

	code := uint32(bus.IWRAMStart + 0x2600)
	writeNOPs(m, code, 1)
	step, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if step.CPU.ExceptionTaken {
		t.Fatalf("acknowledged IRQ still took exception: %+v", step.CPU)
	}
}

func TestCPUWriteIEUsesSevenCyclesFromRegisterAccessEdge(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}

	// IF is already pending but disabled, so no IRQ event exists yet.
	m.IRQ.Request(gbairq.VBlank)
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})

	// The timed CPU-facing I/O write consumes one cycle. IRQ qualification is
	// observed on that register access edge, leaving six cycles after the write.
	if cycles := m.memory.Write16(bus.IOStart+0x200, uint16(gbairq.VBlank), bus.Access{}); cycles != 1 {
		t.Fatalf("IE write cycles = %d, want 1", cycles)
	}
	if m.Cycle() != 1 {
		t.Fatalf("cycle after IE write = %d, want 1", m.Cycle())
	}

	m.Advance(uint32(IRQPropagationLatency - 2))
	if m.CPU.IRQLine() {
		t.Fatal("CPU IRQ line asserted before seventh cycle from IE write edge")
	}
	m.Advance(1)
	if m.Cycle() != IRQPropagationLatency {
		t.Fatalf("IRQ delivery cycle = %d, want %d", m.Cycle(), IRQPropagationLatency)
	}
	if !m.CPU.IRQLine() {
		t.Fatal("CPU IRQ line missing at seventh cycle from IE write edge")
	}
}

func TestDMAIRQPropagationStartsAtTransferCompletion(t *testing.T) {
	m := New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}

	source := uint32(bus.IWRAMStart + 0x2700)
	dest := uint32(bus.IWRAMStart + 0x2800)
	m.Bus.Write16(source, 0x7788, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.DMA0), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	programDMA(m, 0, source, dest, 1, (1<<14)|(1<<15))

	// DMA starts at cycle 2 and the IWRAM halfword transfer consumes four
	// stall cycles, so completion (and IF assertion) occurs at cycle 6.
	m.Advance(2)
	if m.Cycle() != 6 {
		t.Fatalf("DMA completion cycle = %d, want 6", m.Cycle())
	}
	if m.IRQ.IF()&uint16(gbairq.DMA0) == 0 {
		t.Fatalf("DMA0 IF not set at completion: %04x", m.IRQ.IF())
	}
	if m.CPU.IRQLine() {
		t.Fatal("DMA IRQ propagation started from transfer start instead of completion")
	}

	m.Advance(uint32(IRQPropagationLatency - 1))
	if m.CPU.IRQLine() {
		t.Fatal("DMA IRQ reached CPU one cycle before post-completion deadline")
	}
	m.Advance(1)
	if !m.CPU.IRQLine() {
		t.Fatal("DMA IRQ did not reach CPU after post-completion propagation delay")
	}
	if want := uint64(6) + IRQPropagationLatency; m.Cycle() != want {
		t.Fatalf("DMA IRQ delivery cycle = %d, want %d", m.Cycle(), want)
	}
}

// Package system wires the GBA CPU, bus, DMA, timers, PPU, and interrupt
// controller onto one master-clock timeline.
package system

import (
	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/dma"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
	"github.com/maestroi/gomeboy/internal/gba/power"
	"github.com/maestroi/gomeboy/internal/gba/timer"
)

const (
	// DMAStartLatency is the delay from a DMA request/enable edge to bus ownership.
	// The transfer then stalls the CPU for the bus/internal cycles reported by DMA.
	DMAStartLatency uint64 = 2

	// IRQPropagationLatency is the GBA interrupt-controller delay from an
	// enabled pending request (IE & IF) to the CPU-visible IRQ delivery event.
	IRQPropagationLatency uint64 = 7
)

// StepResult describes one architectural CPU step plus any DMA stalls that
// occurred before or during that instruction.
type StepResult struct {
	CPU            cpu.StepResult
	ElapsedCycles  uint64
	DMAStallCycles uint64
	Halted         bool
	Woke           bool
}

// Machine is the central GBA timing domain. All components advance in GBA
// master-clock cycles; it deliberately does not reuse the GB/GBC scheduler or
// its machine-cycle/double-speed rules.
type Machine struct {
	Bus    *bus.Bus
	CPU    *cpu.CPU
	IRQ    *gbairq.Controller
	DMA    *dma.DMA
	Timers *timer.Timers
	PPU    *ppu.PPU
	Power  *power.Controller

	cycles uint64
	memory timedMemory

	dmaStartAt    uint64
	dmaScheduled  bool
	dmaRunning    bool
	dmaCompleting bool
	dmaStalls     uint64

	irqEventAt   uint64
	irqScheduled bool
	irqAfterDMA  bool
	haltWakeSeq  uint64

	halted        bool
	stopRequested bool
}

// New creates a wired GBA timing domain around owned BIOS/ROM bus storage.
func New(bios, rom []byte) *Machine {
	m := &Machine{}
	m.Bus = bus.New(bios, rom)
	m.CPU = cpu.New()
	m.IRQ = gbairq.NewWithHooks(m.Bus, m.CPU, gbairq.Hooks{
		Evaluate:  m.evaluateIRQ,
		DeferLine: true,
	})
	m.Power = power.New(m.Bus, power.Hooks{
		BIOSAccess: func() bool {
			pc := m.CPU.PC()
			return pc >= bus.BIOSStart && pc < bus.BIOSStart+bus.BIOSSize
		},
		Halt: m.enterHalt,
		Stop: m.requestStop,
	})

	m.DMA = dma.New(m.Bus, m.IRQ, dma.Hooks{
		RequestStart: m.requestDMAStart,
		CancelStart:  m.cancelDMAStart,
	})
	m.Timers = timer.New(m.Bus, m.IRQ, timer.Hooks{})
	m.PPU = ppu.New(m.Bus, ppu.Hooks{
		HBlank: func() {
			m.DMA.Trigger(dma.StartHBlank)
		},
		ScanlineStart: func(vcount uint16) {
			m.DMA.TriggerVideoCapture(vcount)
		},
		VBlank: func() {
			m.DMA.Trigger(dma.StartVBlank)
		},
		IRQ: m.IRQ.Request,
	})
	m.memory.m = m
	return m
}

// Cycle returns the current GBA master-clock cycle.
func (m *Machine) Cycle() uint64 { return m.cycles }

// DMAStallCycles returns the cumulative master-clock cycles for which DMA has
// owned the bus and the CPU has therefore been suspended.
func (m *Machine) DMAStallCycles() uint64 { return m.dmaStalls }

// Halted reports whether HALTCNT has stopped CPU execution.
func (m *Machine) Halted() bool { return m.halted }

// StopRequested reports whether BIOS code requested STOP mode. STOP has
// different clock/wake behavior from HALT and remains a later system slice.
func (m *Machine) StopRequested() bool { return m.stopRequested }

// Step executes one CPU instruction/exception boundary on the shared master
// clock. CPU fetches, data accesses, and internal idle cycles advance PPU and
// timers immediately through timedMemory. DMA may take the bus between those
// CPU phases once its start latency expires.
func (m *Machine) Step() (StepResult, error) {
	startCycle := m.cycles
	startStalls := m.dmaStalls
	startWakeSeq := m.haltWakeSeq

	m.serviceDueEvents()
	if m.halted {
		// HALT stops only the CPU. Advance directly to one meaningful hardware
		// boundary instead of burning instruction-sized cycles. IRQ wake itself
		// is one of those scheduled boundaries after its propagation delay.
		if m.halted {
			m.advanceHaltedEvent()
		}
		return StepResult{
			ElapsedCycles:  m.cycles - startCycle,
			DMAStallCycles: m.dmaStalls - startStalls,
			Halted:         m.halted,
			Woke:           m.haltWakeSeq != startWakeSeq,
		}, nil
	}

	m.memory.cpuCycles = 0

	result, err := m.CPU.Step(&m.memory)
	if missing := result.TotalCycles - min(result.TotalCycles, m.memory.cpuCycles); missing != 0 {
		// Exception entry currently reports one internal cycle without calling
		// Memory.Idle. Keep it on the same central timeline.
		m.advanceCPU(missing)
	}

	return StepResult{
		CPU:            result,
		ElapsedCycles:  m.cycles - startCycle,
		DMAStallCycles: m.dmaStalls - startStalls,
		Halted:         m.halted,
		Woke:           m.haltWakeSeq != startWakeSeq,
	}, err
}

// Advance advances the system while the CPU itself performs no bus work. DMA
// requests that become due still seize the bus and extend elapsed master time.
func (m *Machine) Advance(cycles uint32) {
	m.advanceCPU(cycles)
}

func (m *Machine) enterHalt() {
	m.halted = true
}

func (m *Machine) requestStop() {
	// STOP freezes additional hardware domains and has a restricted wake-source
	// set. Record it explicitly rather than incorrectly treating it as HALT.
	m.stopRequested = true
}

func (m *Machine) evaluateIRQ(enabledPending bool, irqAsserted bool) {
	// Once delivered, the CPU IRQ input remains level-sensitive and must drop
	// immediately when software removes IME/IE/IF qualification.
	if !irqAsserted {
		m.CPU.SetIRQLine(false)
	}
	if !enabledPending {
		return
	}

	// DMA raises its IF bit synchronously when ServicePending returns, while the
	// scheduler advances the transfer's bus time immediately afterward. Timestamp
	// its IRQ propagation from the actual end of that transfer, not its start.
	if m.dmaCompleting {
		m.irqAfterDMA = true
		return
	}
	m.scheduleIRQEvent()
}

func (m *Machine) scheduleIRQEvent() {
	if m.irqScheduled {
		return
	}
	m.irqEventAt = m.cycles + IRQPropagationLatency
	m.irqScheduled = true
}

func (m *Machine) serviceDueIRQ() {
	if !m.irqScheduled || m.irqEventAt > m.cycles {
		return
	}
	m.irqScheduled = false

	// The propagation event releases HALT regardless of IME. At delivery time
	// the current IE/IF/IME state decides whether the CPU-visible IRQ line rises.
	if m.halted {
		m.halted = false
		m.haltWakeSeq++
	}
	m.CPU.SetIRQLine(m.IRQ.IRQAsserted())
}

func (m *Machine) serviceDueEvents() {
	m.serviceDueIRQ()
	m.serviceDueDMA()
	m.serviceDueIRQ()
}

func (m *Machine) advanceHaltedEvent() {
	// PPU always supplies a finite deadline. Timers and a deferred DMA start
	// may provide an earlier one.
	step := uint64(m.PPU.CyclesUntilEvent())
	if untilTimer := uint64(m.Timers.CyclesUntilEvent()); untilTimer < step {
		step = untilTimer
	}
	if m.dmaScheduled {
		untilDMA := uint64(0)
		if m.dmaStartAt > m.cycles {
			untilDMA = m.dmaStartAt - m.cycles
		}
		if untilDMA < step {
			step = untilDMA
		}
	}
	if m.irqScheduled {
		untilIRQ := uint64(0)
		if m.irqEventAt > m.cycles {
			untilIRQ = m.irqEventAt - m.cycles
		}
		if untilIRQ < step {
			step = untilIRQ
		}
	}

	if step == 0 {
		m.serviceDueEvents()
		return
	}
	m.advanceCPU(uint32(step))
}

func (m *Machine) requestDMAStart() {
	if !m.DMA.Pending() {
		return
	}
	// DMA Enable can be written by the CPU in the middle of a timed I/O bus
	// access. In that case the start latency begins when that access completes.
	if m.memory.inBusCall {
		m.memory.requestAfterAccess = true
		return
	}
	m.scheduleDMAStart()
}

func (m *Machine) cancelDMAStart() {
	if !m.DMA.Pending() {
		m.dmaScheduled = false
	}
}

func (m *Machine) scheduleDMAStart() {
	at := m.cycles + DMAStartLatency
	if !m.dmaScheduled || at < m.dmaStartAt {
		m.dmaStartAt = at
		m.dmaScheduled = true
	}
}

// advanceCPU advances a requested number of CPU-owned cycles. DMA stall time
// does not consume remaining CPU work: a transfer pauses that work, advances
// the rest of the machine, then the CPU phase resumes.
func (m *Machine) advanceCPU(cycles uint32) {
	remaining := uint64(cycles)
	for remaining > 0 {
		m.serviceDueEvents()

		step := remaining
		if m.dmaScheduled && m.dmaStartAt > m.cycles {
			if untilDMA := m.dmaStartAt - m.cycles; untilDMA < step {
				step = untilDMA
			}
		}
		if m.irqScheduled && m.irqEventAt > m.cycles {
			if untilIRQ := m.irqEventAt - m.cycles; untilIRQ < step {
				step = untilIRQ
			}
		}
		m.advanceHardware(uint32(step))
		remaining -= step
		m.serviceDueEvents()
	}
	m.serviceDueEvents()
}

func (m *Machine) serviceDueDMA() {
	if m.dmaRunning {
		return
	}
	for m.dmaScheduled && m.dmaStartAt <= m.cycles {
		m.dmaScheduled = false
		if !m.DMA.Pending() {
			continue
		}

		m.dmaRunning = true
		m.dmaCompleting = true
		stall := m.DMA.ServicePending()
		m.dmaCompleting = false
		if stall == 0 {
			m.dmaRunning = false
			continue
		}

		m.dmaStalls += uint64(stall)
		m.advanceHardware(stall)
		m.dmaRunning = false

		if m.irqAfterDMA {
			m.irqAfterDMA = false
			if m.IRQ.EnabledPending() {
				m.scheduleIRQEvent()
			}
		}
		// PPU/timer edges crossed during the transfer may have queued another
		// request. Fine-grained mid-transfer DMA preemption is a later slice;
		// queued work begins as soon as the current batch releases the bus.
	}
}

// advanceHardware moves peripherals without servicing DMA. Callers use this
// while DMA owns the bus and while advancing a CPU phase between DMA deadlines.
func (m *Machine) advanceHardware(cycles uint32) {
	remaining := cycles
	for remaining > 0 {
		m.serviceDueIRQ()

		step := remaining
		if untilPPU := m.PPU.CyclesUntilEvent(); untilPPU < step {
			step = untilPPU
		}
		if untilTimer := m.Timers.CyclesUntilEvent(); untilTimer < step {
			step = untilTimer
		}
		if m.irqScheduled && m.irqEventAt > m.cycles {
			if untilIRQ := m.irqEventAt - m.cycles; untilIRQ < uint64(step) {
				step = uint32(untilIRQ)
			}
		}

		m.cycles += uint64(step)
		m.Timers.Advance(step)
		m.PPU.Advance(step)
		remaining -= step
		m.serviceDueIRQ()
	}
}

// timedMemory is the CPU-facing bus adapter. It advances master time at each
// actual ARM7 bus/idle phase instead of waiting until the instruction returns.
type timedMemory struct {
	m                  *Machine
	cpuCycles          uint32
	inBusCall          bool
	requestAfterAccess bool
}

func (t *timedMemory) Read8(addr uint32, access gbamemory.Access) (byte, uint32) {
	value, cycles := t.m.Bus.Read8(addr, access)
	t.consume(cycles)
	return value, cycles
}

func (t *timedMemory) Read16(addr uint32, access gbamemory.Access) (uint16, uint32) {
	value, cycles := t.m.Bus.Read16(addr, access)
	t.consume(cycles)
	return value, cycles
}

func (t *timedMemory) Read32(addr uint32, access gbamemory.Access) (uint32, uint32) {
	value, cycles := t.m.Bus.Read32(addr, access)
	t.consume(cycles)
	return value, cycles
}

func (t *timedMemory) Write8(addr uint32, value byte, access gbamemory.Access) uint32 {
	t.inBusCall = true
	cycles := t.m.Bus.Write8(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) Write16(addr uint32, value uint16, access gbamemory.Access) uint32 {
	t.inBusCall = true
	cycles := t.m.Bus.Write16(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) Write32(addr uint32, value uint32, access gbamemory.Access) uint32 {
	t.inBusCall = true
	cycles := t.m.Bus.Write32(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) Idle(cycles uint32) {
	t.m.Bus.Idle(cycles)
	t.consume(cycles)
}

func (t *timedMemory) consume(cycles uint32) {
	t.cpuCycles += cycles
	t.m.advanceCPU(cycles)
}

func (t *timedMemory) flushDeferredDMARequest() {
	if !t.requestAfterAccess {
		return
	}
	t.requestAfterAccess = false
	if t.m.DMA.Pending() {
		t.m.scheduleDMAStart()
	}
}

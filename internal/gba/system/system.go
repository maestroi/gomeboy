// Package system wires the GBA CPU, bus, DMA, timers, audio, keypad, PPU,
// and interrupt controller onto one master-clock timeline.
package system

import (
	"github.com/maestroi/gomeboy/internal/gba/audio"
	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/dma"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/keypad"
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
	IRQPropagationLatency uint64 = 5
)

// StepResult describes one architectural CPU step plus any DMA stalls that
// occurred before or during that instruction.
type StepResult struct {
	CPU            cpu.StepResult
	ElapsedCycles  uint64
	DMAStallCycles uint64
	Halted         bool
	Stopped        bool
	Woke           bool
}

// Machine is the central GBA timing domain. All components advance in GBA
// master-clock cycles; it deliberately does not reuse the GB/GBC scheduler or
// its machine-cycle/double-speed rules.
type Machine struct {
	Bus    *bus.Bus
	CPU    *cpu.CPU
	IRQ    *gbairq.Controller
	Audio  *audio.Audio
	Keypad *keypad.Keypad
	DMA    *dma.DMA
	Timers *timer.Timers
	PPU    *ppu.PPU
	Power  *power.Controller

	Cartridge cartridge.Setup

	cycles uint64
	memory timedMemory

	dmaStartAt   [4]uint64
	dmaScheduled uint8
	dmaRunning   bool
	dmaStalls    uint64

	irqEventAt   uint64
	irqScheduled bool
	haltWakeSeq  uint64

	halted         bool
	stopped        bool
	stopPending    bool
	stopWakePending bool
	stopRequested  bool
}

// New creates a wired GBA timing domain around owned BIOS/ROM bus storage and
// automatically configures cartridge save hardware from ROM markers.
func New(bios, rom []byte) *Machine {
	return NewWithCartridgeConfig(bios, rom, cartridge.Config{})
}

// NewWithCartridgeConfig creates a wired GBA timing domain with an explicit
// cartridge save configuration. Non-auto save types override ROM detection.
func NewWithCartridgeConfig(bios, rom []byte, config cartridge.Config) *Machine {
	m := &Machine{}
	m.Bus = bus.New(bios, rom)
	m.Cartridge = cartridge.Configure(m.Bus, rom, config)
	m.CPU = cpu.New()
	m.IRQ = gbairq.NewWithHooks(m.Bus, m.CPU, gbairq.Hooks{
		Evaluate:        m.evaluateIRQ,
		ExternalRequest: m.handleExternalIRQ,
		DeferLine:       true,
	})
	m.Keypad = keypad.New(m.Bus, m.IRQ)
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
	m.Audio = audio.New(m.Bus, audio.Hooks{
		RequestFIFO: func(fifo audio.FIFO) {
			switch fifo {
			case audio.FIFOA:
				m.DMA.TriggerFIFO(dma.FIFOA)
			case audio.FIFOB:
				m.DMA.TriggerFIFO(dma.FIFOB)
			}
		},
	})
	m.Timers = timer.New(m.Bus, m.IRQ, timer.Hooks{
		Overflow: m.Audio.TimerOverflow,
	})
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

// Halted reports whether HALTCNT has stopped CPU execution while the rest of
// the GBA clock domain continues running.
func (m *Machine) Halted() bool { return m.halted }

// Stopped reports whether HALTCNT STOP has halted the GBA system clock.
func (m *Machine) Stopped() bool { return m.stopped }

// StopRequested reports whether BIOS code has requested STOP at least once.
func (m *Machine) StopRequested() bool { return m.stopRequested }

// Step executes one CPU instruction/exception boundary on the shared master
// clock. CPU fetches, data accesses, and internal idle cycles advance PPU and
// timers immediately through timedMemory. DMA may take the bus between those
// CPU phases once its start latency expires.
func (m *Machine) Step() (StepResult, error) {
	startCycle := m.cycles
	startStalls := m.dmaStalls
	startWakeSeq := m.haltWakeSeq
	wasStopped := m.stopped
	wasHalted := m.halted

	if wasStopped {
		// STOP-capable external signals are sampled outside the stopped system
		// clock. If their IE bit is enabled they restart the oscillator, but the
		// wake signal itself does not set IF on GBA hardware.
		if m.stopWakePending {
			m.stopWakePending = false
			m.stopped = false
			m.haltWakeSeq++
			// Any IF bits that predated STOP begin normal IRQ propagation only now,
			// after the system clock has restarted.
			if m.IRQ.EnabledPending() {
				m.scheduleIRQEvent()
			}
		}
		return StepResult{
			ElapsedCycles:  m.cycles - startCycle,
			DMAStallCycles: m.dmaStalls - startStalls,
			Halted:         m.halted,
			Stopped:        m.stopped,
			Woke:           m.haltWakeSeq != startWakeSeq,
		}, nil
	}

	m.serviceDueEvents()
	if wasHalted {
		// HALT stops only the CPU. Advance directly to one meaningful hardware
		// boundary instead of burning instruction-sized cycles. IRQ wake itself
		// is one of those scheduled boundaries after its propagation delay. Even
		// when that IRQ event was already due at entry, return at the wake boundary
		// rather than executing an ARM instruction in this same Step call.
		if m.halted {
			m.advanceHaltedEvent()
		}
		return StepResult{
			ElapsedCycles:  m.cycles - startCycle,
			DMAStallCycles: m.dmaStalls - startStalls,
			Halted:         m.halted,
			Stopped:        m.stopped,
			Woke:           m.haltWakeSeq != startWakeSeq,
		}, nil
	}

	m.memory.cpuCycles = 0
	m.memory.inInstruction = true
	result, err := m.CPU.Step(&m.memory)
	m.memory.inInstruction = false
	m.Timers.EndWriteAccess()
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
		Stopped:        m.stopped,
		Woke:           m.haltWakeSeq != startWakeSeq,
	}, err
}

// Advance advances the system while the CPU itself performs no bus work. DMA
// requests that become due still seize the bus and extend elapsed master time.
func (m *Machine) Advance(cycles uint32) {
	if m.stopped {
		return
	}
	m.advanceCPU(cycles)
}

func (m *Machine) enterHalt() {
	m.stopped = false
	m.halted = true
}

func (m *Machine) requestStop() {
	m.stopRequested = true
	// HALTCNT can be written during a CPU bus access. Let that access consume its
	// clock cycle before stopping the oscillator.
	if m.memory.inBusCall {
		m.stopPending = true
		return
	}
	m.enterStop()
}

func (m *Machine) enterStop() {
	m.stopPending = false
	m.stopWakePending = false
	m.halted = false
	m.stopped = true

	// STOP freezes the interrupt controller clock as well. Existing IF state is
	// preserved but cannot propagate until an external STOP wake signal restarts
	// the clock.
	m.irqScheduled = false
	m.CPU.SetIRQLine(false)
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

	if m.stopped {
		// IF/IE state cannot restart STOP. Wake comes from a separate external
		// Keypad/Game Pak/general-purpose-SIO signal path.
		return
	}

	m.scheduleIRQEvent()
}

func (m *Machine) handleExternalIRQ(source gbairq.Source) bool {
	if !m.stopped {
		return false
	}

	// RequestExternal already masks to StopWakeSources. While STOP is active,
	// the signal is consumed without latching IF. It only arms wake when the
	// corresponding source is enabled in IE.
	if m.IRQ != nil && m.IRQ.IE()&uint16(source) != 0 {
		m.stopWakePending = true
	}
	return true
}

func (m *Machine) scheduleIRQEvent() {
	if m.irqScheduled {
		return
	}
	m.irqEventAt = m.cycles + IRQPropagationLatency
	m.irqScheduled = true
}

func (m *Machine) serviceDueIRQ() {
	if m.stopped || !m.irqScheduled || m.irqEventAt > m.cycles {
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
	if m.stopped {
		return
	}
	// Ordinary CPU phases overwhelmingly have no deferred IRQ or DMA work.
	// Avoid walking the DMA channels on every fetch/data/idle phase when there
	// is nothing the scheduler can possibly service.
	if !m.irqScheduled && m.dmaScheduled == 0 && !m.DMA.Active() {
		return
	}
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
	if untilDMA, ok := m.cyclesUntilDMAStart(); ok && untilDMA < step {
		step = untilDMA
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

func (m *Machine) requestDMAStart(channels uint8) {
	channels &= m.DMA.PendingMask()
	if channels == 0 {
		return
	}
	// DMA Enable can be written by the CPU in the middle of a timed I/O bus
	// access. In that case the start latency begins when that access completes.
	if m.memory.inBusCall {
		m.memory.requestAfterAccess |= channels
		return
	}
	m.scheduleDMAStart(channels)
}

func (m *Machine) cancelDMAStart(channel int) {
	if channel < 0 || channel >= 4 {
		return
	}
	bit := uint8(1 << channel)
	m.dmaScheduled &^= bit
	m.memory.requestAfterAccess &^= bit
}

func (m *Machine) scheduleDMAStart(channels uint8) {
	channels &= m.DMA.PendingMask()
	for channel := 0; channel < 4; channel++ {
		bit := uint8(1 << channel)
		if channels&bit == 0 || m.dmaScheduled&bit != 0 {
			continue
		}
		m.dmaStartAt[channel] = m.cycles + DMAStartLatency
		m.dmaScheduled |= bit
	}
}

func (m *Machine) cyclesUntilDMAStart() (uint64, bool) {
	if m.dmaScheduled == 0 {
		return 0, false
	}
	var best uint64
	found := false
	for channel := 0; channel < 4; channel++ {
		bit := uint8(1 << channel)
		if m.dmaScheduled&bit == 0 {
			continue
		}
		remaining := uint64(0)
		if m.dmaStartAt[channel] > m.cycles {
			remaining = m.dmaStartAt[channel] - m.cycles
		}
		if !found || remaining < best {
			best = remaining
			found = true
		}
	}
	return best, found
}

func (m *Machine) activateDueDMA() {
	if m.dmaScheduled == 0 {
		return
	}
	var ready uint8
	for channel := 0; channel < 4; channel++ {
		bit := uint8(1 << channel)
		if m.dmaScheduled&bit == 0 || m.dmaStartAt[channel] > m.cycles {
			continue
		}
		m.dmaScheduled &^= bit
		ready |= bit
	}
	if ready != 0 {
		m.DMA.ActivatePending(ready)
	}
}

// advanceCPU advances a requested number of CPU-owned cycles. DMA stall time
// does not consume remaining CPU work: a transfer pauses that work, advances
// the rest of the machine, then the CPU phase resumes.
func (m *Machine) advanceCPU(cycles uint32) {
	if m.stopped {
		return
	}
	remaining := uint64(cycles)
	for remaining > 0 {
		m.serviceDueEvents()

		step := remaining
		if untilDMA, ok := m.cyclesUntilDMAStart(); ok && untilDMA < step {
			step = untilDMA
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
	if m.dmaRunning || (m.dmaScheduled == 0 && !m.DMA.Active()) {
		return
	}

	m.activateDueDMA()
	if !m.DMA.Active() {
		return
	}

	m.dmaRunning = true
	defer func() { m.dmaRunning = false }()

	for m.DMA.Active() {
		// A request whose start latency expired during the previous unit becomes
		// active here. ServiceUnit then re-arbitrates DMA0..DMA3, allowing a
		// higher-priority channel to preempt before the next lower-priority unit.
		m.activateDueDMA()

		unit := m.DMA.ServiceUnit()
		if unit.Cycles == 0 {
			return
		}

		m.dmaStalls += uint64(unit.Cycles)
		m.advanceHardware(unit.Cycles)

		if unit.Completed {
			// Completion/IRQ state becomes visible only after the final unit's
			// bus/internal cycles have elapsed.
			m.DMA.FinishUnit(unit.Channel)
		}

		m.activateDueDMA()
	}
}

// advanceHardware moves peripherals without servicing DMA. Callers use this
// while DMA owns the bus and while advancing a CPU phase between DMA deadlines.
func (m *Machine) advanceHardware(cycles uint32) {
	if m.stopped || cycles == 0 {
		return
	}

	// Keep the timer scheduler on the same continuously advancing master clock
	// as the rest of the machine, even while all timers are disabled. Timer
	// prescalers are phase-aligned to this global clock, so pausing the timer
	// timestamp while inactive corrupts the first tick after a later enable.
	//
	// Most ARM bus/idle phases are only a handful of cycles long. If no IRQ,
	// timer, or PPU deadline occurs before the end of the phase, advance all
	// hardware directly without entering the generic event-splitting loop.
	if !m.irqScheduled &&
		cycles <= m.Timers.CyclesUntilEvent() &&
		cycles <= m.PPU.CyclesUntilEvent() {
		m.cycles += uint64(cycles)
		m.Cartridge.Advance(cycles)
		m.Audio.Advance(cycles)
		m.Timers.Advance(cycles)
		m.PPU.Advance(cycles)
		return
	}

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
		m.Cartridge.Advance(step)
		// Host-rate audio samples cover the interval before timer edges at its
		// end; timer overflow then updates the Direct Sound latch for the next
		// interval.
		m.Audio.Advance(step)
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
	inInstruction      bool
	requestAfterAccess uint8
}

func (t *timedMemory) Read8(addr uint32, access gbamemory.Access) (byte, uint32) {
	value, cycles := t.m.Bus.Read8(addr, access)
	t.consume(cycles)
	return value, cycles
}

func (t *timedMemory) Read16(addr uint32, access gbamemory.Access) (uint16, uint32) {
	value, cycles := t.m.Bus.Read16(addr, access)
	if !access.Instruction {
		if timer, ok := timerCounterIndex(addr); ok {
			value = t.m.Timers.CounterForCPURead(timer)
		}
	}
	if access.Instruction {
		// Thumb open bus is driven by the instruction pipeline rather than the
		// halfword currently being executed. prefetch0 is PC+2 and prefetch1
		// is PC+4 relative to this fetch; most regions expose prefetch1 in
		// both halfwords, while BIOS/OAM/IWRAM preserve pipeline ordering.
		prefetch0 := t.m.Bus.Peek16(addr + 2)
		prefetch1 := t.m.Bus.Peek16(addr + 4)
		visiblePC := addr + 4
		var open uint32
		switch visiblePC >> 24 {
		case 0x00, 0x07: // BIOS / OAM
			open = uint32(prefetch0) | uint32(prefetch1)<<16
		case 0x03: // IWRAM
			if visiblePC&2 != 0 {
				open = uint32(prefetch0) | uint32(prefetch1)<<16
			} else {
				open = uint32(prefetch1) | uint32(prefetch0)<<16
			}
		default:
			open = uint32(prefetch1) | uint32(prefetch1)<<16
		}
		// Do this before hardware advances so DMA can still replace the
		// physical-bus value if it starts during the fetch.
		t.m.Bus.SetCPUOpenBus(open)
	}
	t.consume(cycles)
	return value, cycles
}

func (t *timedMemory) Read32(addr uint32, access gbamemory.Access) (uint32, uint32) {
	value, cycles := t.m.Bus.Read32(addr, access)
	if !access.Instruction {
		if timer, ok := timerCounterIndex(addr); ok {
			value = value&0xffff0000 | uint32(t.m.Timers.CounterForCPURead(timer))
		}
	}
	if access.Instruction {
		// ARM7 open bus exposes the second prefetched ARM instruction while
		// the current opcode executes. At an ARM fetch at PC, that is PC+8.
		// Set it before advancing hardware so a DMA transfer that occurs
		// during the fetch can still take ownership of the physical bus.
		t.m.Bus.SetCPUOpenBus(t.m.Bus.Peek32(addr + 8))
	}
	t.consume(cycles)
	return value, cycles
}

func timerCounterIndex(addr uint32) (int, bool) {
	if addr < bus.IOStart+0x100 || addr > bus.IOStart+0x10c || (addr-(bus.IOStart+0x100))%4 != 0 {
		return 0, false
	}
	return int((addr - (bus.IOStart + 0x100)) / 4), true
}

func (t *timedMemory) Write8(addr uint32, value byte, access gbamemory.Access) uint32 {
	t.inBusCall = true
	if t.inInstruction {
		t.m.Timers.BeginWriteAccess()
	}
	cycles := t.m.Bus.Write8(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	if t.inInstruction {
		t.m.Timers.EndWriteBusAccess()
	}
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) Write16(addr uint32, value uint16, access gbamemory.Access) uint32 {
	t.inBusCall = true
	if t.inInstruction {
		t.m.Timers.BeginWriteAccess()
	}
	cycles := t.m.Bus.Write16(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	if t.inInstruction {
		t.m.Timers.EndWriteBusAccess()
	}
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) Write32(addr uint32, value uint32, access gbamemory.Access) uint32 {
	t.inBusCall = true
	if t.inInstruction {
		t.m.Timers.BeginWriteAccess()
	}
	cycles := t.m.Bus.Write32(addr, value, access)
	t.inBusCall = false
	t.consume(cycles)
	if t.inInstruction {
		t.m.Timers.EndWriteBusAccess()
	}
	t.flushDeferredDMARequest()
	return cycles
}

func (t *timedMemory) AccessCycles(addr uint32, width uint32, access gbamemory.Access) uint32 {
	return t.m.Bus.AccessCycles(addr, width, access)
}

func (t *timedMemory) SpendCycles(cycles uint32) {
	t.consume(cycles)
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
	if t.requestAfterAccess != 0 {
		channels := t.requestAfterAccess
		t.requestAfterAccess = 0
		if t.m.DMA.Pending() {
			t.m.scheduleDMAStart(channels)
		}
	}
	if t.m.stopPending {
		t.m.enterStop()
	}
}

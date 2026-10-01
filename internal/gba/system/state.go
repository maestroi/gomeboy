package system

import (
	"github.com/maestroi/gomeboy/internal/gba/audio"
	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/dma"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/keypad"
	"github.com/maestroi/gomeboy/internal/gba/power"
	"github.com/maestroi/gomeboy/internal/gba/ppu"
	"github.com/maestroi/gomeboy/internal/gba/timer"
)

// State is a complete native GBA machine snapshot. Configuration such as ROM,
// BIOS, hooks, and host audio policy remains owned by the receiving Machine.
type State struct {
	CPU       cpu.State
	Bus       bus.State
	Cartridge cartridge.State
	IRQ       gbairq.State
	Audio     audio.State
	Keypad    keypad.State
	DMA       dma.State
	Timers    timer.State
	PPU       ppu.State
	Power     power.State

	Cycles uint64

	DMAStartAt   [4]uint64
	DMAScheduled uint8
	DMARunning   bool
	DMAStalls    uint64

	IRQEventAt   uint64
	IRQScheduled bool
	HaltWakeSeq  uint64

	Halted          bool
	Stopped         bool
	StopPending     bool
	StopWakePending bool
	StopRequested   bool

	MemoryCPUCycles          uint32
	MemoryInBusCall          bool
	MemoryInInstruction      bool
	MemoryRequestAfterAccess uint8
}

func (m *Machine) Snapshot() State {
	var s State
	m.SnapshotInto(&s)
	return s
}

// SnapshotInto captures the machine while reusing variable-size state buffers.
func (m *Machine) SnapshotInto(s *State) {
	if s == nil {
		return
	}
	s.CPU = m.CPU.Snapshot()
	m.Bus.SnapshotInto(&s.Bus)
	m.Cartridge.SnapshotInto(&s.Cartridge)
	s.IRQ = m.IRQ.Snapshot()
	s.Audio = m.Audio.Snapshot()
	s.Keypad = m.Keypad.Snapshot()
	s.DMA = m.DMA.Snapshot()
	m.Timers.SnapshotInto(&s.Timers)
	s.PPU = m.PPU.Snapshot()
	s.Power = m.Power.Snapshot()

	s.Cycles = m.cycles
	s.DMAStartAt = m.dmaStartAt
	s.DMAScheduled = m.dmaScheduled
	s.DMARunning = m.dmaRunning
	s.DMAStalls = m.dmaStalls
	s.IRQEventAt = m.irqEventAt
	s.IRQScheduled = m.irqScheduled
	s.HaltWakeSeq = m.haltWakeSeq
	s.Halted = m.halted
	s.Stopped = m.stopped
	s.StopPending = m.stopPending
	s.StopWakePending = m.stopWakePending
	s.StopRequested = m.stopRequested
	s.MemoryCPUCycles = m.memory.cpuCycles
	s.MemoryInBusCall = m.memory.inBusCall
	s.MemoryInInstruction = m.memory.inInstruction
	s.MemoryRequestAfterAccess = m.memory.requestAfterAccess
}

// Restore restores a state into the same configured machine.
func (m *Machine) Restore(s State) error {
	m.Bus.Restore(s.Bus)
	if err := m.Cartridge.Restore(s.Cartridge); err != nil {
		return err
	}
	m.CPU.Restore(s.CPU)
	m.IRQ.Restore(s.IRQ)
	m.Audio.Restore(s.Audio)
	m.Keypad.Restore(s.Keypad)
	m.DMA.Restore(s.DMA)
	m.Timers.Restore(s.Timers)
	m.PPU.Restore(s.PPU)
	m.Power.Restore(s.Power)

	m.cycles = s.Cycles
	m.dmaStartAt = s.DMAStartAt
	m.dmaScheduled = s.DMAScheduled & 0x0f
	m.dmaRunning = s.DMARunning
	m.dmaStalls = s.DMAStalls
	m.irqEventAt = s.IRQEventAt
	m.irqScheduled = s.IRQScheduled
	m.haltWakeSeq = s.HaltWakeSeq
	m.halted = s.Halted
	m.stopped = s.Stopped
	m.stopPending = s.StopPending
	m.stopWakePending = s.StopWakePending
	m.stopRequested = s.StopRequested
	m.memory.m = m
	m.memory.cpuCycles = s.MemoryCPUCycles
	m.memory.inBusCall = s.MemoryInBusCall
	m.memory.inInstruction = s.MemoryInInstruction
	m.memory.requestAfterAccess = s.MemoryRequestAfterAccess & 0x0f
	return nil
}

// Package dma implements the Game Boy Advance DMA channels.
package dma

import (
	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

const (
	firstDMAOffset uint32 = 0x0b0
	dmaStride             = 12

	controlDestMask   uint16 = 0x0060
	controlSourceMask uint16 = 0x0180
	controlRepeat     uint16 = 1 << 9
	controlWord       uint16 = 1 << 10
	controlGamePakDRQ uint16 = 1 << 11
	controlTimingMask uint16 = 0x3000
	controlIRQ        uint16 = 1 << 14
	controlEnable     uint16 = 1 << 15

	timingImmediate uint16 = 0
	timingVBlank    uint16 = 1 << 12
	timingHBlank    uint16 = 2 << 12
	timingSpecial   uint16 = 3 << 12
)

var irqSources = [4]gbairq.Source{
	gbairq.DMA0,
	gbairq.DMA1,
	gbairq.DMA2,
	gbairq.DMA3,
}

// IRQSink receives DMA-completion interrupt requests.
type IRQSink interface {
	Request(gbairq.Source)
}

// StartEvent identifies an external DMA start condition.
type StartEvent uint8

const (
	StartVBlank StartEvent = iota + 1
	StartHBlank
	StartSpecial
)

// Hooks exposes DMA timing to scheduler/debug integration.
// Complete fires once per completed channel. Stall fires once for a serviced
// request batch with the total number of CPU-stall cycles consumed by DMA.
type Hooks struct {
	Complete func(channel int, units uint32, cycles uint32)
	Stall    func(cycles uint32)
}

type channel struct {
	sourceInitial uint32
	destInitial   uint32
	countInitial  uint16
	control       uint16

	sourceCurrent uint32
	destCurrent   uint32
	countCurrent  uint32

	lastCycles uint32
	lastUnits  uint32
}

// DMA owns DMA0-DMA3 register and internal transfer state.
type DMA struct {
	bus   *bus.Bus
	irq   IRQSink
	hooks Hooks

	ch [4]channel

	pending   uint8
	servicing bool
}

// New maps DMA0-DMA3 onto b.
func New(b *bus.Bus, irq IRQSink, hooks Hooks) *DMA {
	if b == nil {
		panic("gba dma: nil bus")
	}
	d := &DMA{bus: b, irq: irq, hooks: hooks}
	d.install(b.IO())
	return d
}

func (d *DMA) install(io *bus.IO) {
	for index := 0; index < 4; index++ {
		index := index
		base := firstDMAOffset + uint32(index*dmaStride)

		d.registerWriteOnly32(io, base,
			func() uint32 { return d.ch[index].sourceInitial },
			func(value uint32) { d.ch[index].sourceInitial = value & sourceMask(index) },
		)
		d.registerWriteOnly32(io, base+4,
			func() uint32 { return d.ch[index].destInitial },
			func(value uint32) { d.ch[index].destInitial = value & destMask(index) },
		)

		io.Register16WithByteWrite(base+8,
			func() uint16 { return d.openBusHalfword(base + 8) },
			func(value uint16) { d.ch[index].countInitial = value & countMask(index) },
			func(byteOffset uint32, value byte) {
				current := d.ch[index].countInitial
				if byteOffset == 0 {
					current = current&0xff00 | uint16(value)
				} else {
					current = current&0x00ff | uint16(value)<<8
				}
				d.ch[index].countInitial = current & countMask(index)
			},
		)

		io.Register16(base+10,
			func() uint16 { return d.ch[index].control },
			func(value uint16) { d.writeControl(index, value) },
		)
	}
}

func (d *DMA) registerWriteOnly32(io *bus.IO, offset uint32, get func() uint32, set func(uint32)) {
	for half := 0; half < 2; half++ {
		half := half
		halfOffset := offset + uint32(half*2)
		io.Register16WithByteWrite(halfOffset,
			func() uint16 { return d.openBusHalfword(halfOffset) },
			func(value uint16) {
				current := get()
				if half == 0 {
					current = current&0xffff0000 | uint32(value)
				} else {
					current = current&0x0000ffff | uint32(value)<<16
				}
				set(current)
			},
			func(byteOffset uint32, value byte) {
				current := get()
				shift := uint((half*2)+int(byteOffset)) * 8
				current = current&^(uint32(0xff)<<shift) | uint32(value)<<shift
				set(current)
			},
		)
	}
}

func (d *DMA) openBusHalfword(ioOffset uint32) uint16 {
	value := d.bus.OpenBus()
	if ioOffset&2 != 0 {
		return uint16(value >> 16)
	}
	return uint16(value)
}

func sourceMask(index int) uint32 {
	if index == 0 {
		return 0x07ffffff
	}
	return 0x0fffffff
}

func destMask(index int) uint32 {
	if index < 3 {
		return 0x07ffffff
	}
	return 0x0fffffff
}

func countMask(index int) uint16 {
	if index == 3 {
		return 0xffff
	}
	return 0x3fff
}

func controlMask(index int) uint16 {
	mask := controlDestMask | controlSourceMask | controlRepeat | controlWord |
		controlTimingMask | controlIRQ | controlEnable
	if index == 3 {
		mask |= controlGamePakDRQ
	}
	return mask
}

func (d *DMA) writeControl(index int, value uint16) {
	c := &d.ch[index]
	oldEnabled := c.control&controlEnable != 0
	c.control = value & controlMask(index)
	newEnabled := c.control&controlEnable != 0

	if oldEnabled || !newEnabled {
		return
	}

	d.latch(index)
	if c.control&controlTimingMask == timingImmediate {
		d.pending |= 1 << index
		d.servicePending()
	}
}

func (d *DMA) latch(index int) {
	c := &d.ch[index]
	width := d.width(index)
	alignMask := ^uint32(width - 1)
	c.sourceCurrent = c.sourceInitial & alignMask
	c.destCurrent = c.destInitial & alignMask
	c.countCurrent = effectiveCount(index, c.countInitial)
}

func effectiveCount(index int, value uint16) uint32 {
	if value != 0 {
		return uint32(value)
	}
	if index == 3 {
		return 0x10000
	}
	return 0x4000
}

func (d *DMA) width(index int) uint32 {
	if d.ch[index].control&controlWord != 0 {
		return 4
	}
	return 2
}

// Trigger queues all enabled channels matching event, then services them in
// hardware priority order (DMA0 highest through DMA3 lowest). The returned
// value is the total CPU-stall time consumed by the batch.
func (d *DMA) Trigger(event StartEvent) uint32 {
	timing, ok := timingForEvent(event)
	if !ok {
		return 0
	}

	for index := 0; index < 4; index++ {
		c := &d.ch[index]
		if c.control&controlEnable == 0 || c.control&controlTimingMask != timing {
			continue
		}
		d.pending |= 1 << index
	}
	return d.servicePending()
}

func timingForEvent(event StartEvent) (uint16, bool) {
	switch event {
	case StartVBlank:
		return timingVBlank, true
	case StartHBlank:
		return timingHBlank, true
	case StartSpecial:
		return timingSpecial, true
	default:
		return 0, false
	}
}

func (d *DMA) servicePending() uint32 {
	if d.servicing {
		return 0
	}
	d.servicing = true
	defer func() { d.servicing = false }()

	var total uint32
	for d.pending != 0 {
		for index := 0; index < 4; index++ {
			mask := uint8(1 << index)
			if d.pending&mask == 0 {
				continue
			}
			d.pending &^= mask

			// Software may have disabled the channel after it was queued.
			if d.ch[index].control&controlEnable != 0 {
				total += d.run(index)
			}
			break
		}
	}

	if total != 0 && d.hooks.Stall != nil {
		d.hooks.Stall(total)
	}
	return total
}

func (d *DMA) run(index int) uint32 {
	c := &d.ch[index]
	units := c.countCurrent
	if units == 0 {
		return 0
	}

	width := d.width(index)
	sourceMode := (c.control & controlSourceMask) >> 7
	destMode := (c.control & controlDestMask) >> 5

	source := c.sourceCurrent
	dest := c.destCurrent
	startSource := source
	startDest := dest
	var cycles uint32

	for unit := uint32(0); unit < units; unit++ {
		access := bus.Access{Sequential: unit != 0, DMA: true}
		if width == 4 {
			value, readCycles := d.bus.Read32(source, access)
			writeCycles := d.bus.Write32(dest, value, access)
			cycles += readCycles + writeCycles
		} else {
			value, readCycles := d.bus.Read16(source, access)
			writeCycles := d.bus.Write16(dest, value, access)
			cycles += readCycles + writeCycles
		}

		source = adjustAddress(source, sourceMode, width, false)
		dest = adjustAddress(dest, destMode, width, true)
	}

	c.sourceCurrent = source
	c.destCurrent = dest
	c.countCurrent = 0

	// DMA processing adds two internal cycles normally. If both ends are on
	// the Game Pak bus, ARM7/GBA documentation specifies four internal cycles.
	internal := uint32(2)
	if isGamePak(startSource) && isGamePak(startDest) {
		internal = 4
	}
	cycles += internal

	c.lastCycles = cycles
	c.lastUnits = units

	if c.control&controlIRQ != 0 && d.irq != nil {
		d.irq.Request(irqSources[index])
	}

	timing := c.control & controlTimingMask
	repeat := timing != timingImmediate && c.control&controlRepeat != 0
	if repeat {
		c.countCurrent = effectiveCount(index, c.countInitial)
		if destMode == 3 {
			alignMask := ^uint32(width - 1)
			c.destCurrent = c.destInitial & alignMask
		}
	} else {
		c.control &^= controlEnable
		c.countCurrent = 0
	}

	if d.hooks.Complete != nil {
		d.hooks.Complete(index, units, cycles)
	}
	return cycles
}

func adjustAddress(address uint32, mode uint16, width uint32, destination bool) uint32 {
	switch mode {
	case 0:
		return address + width
	case 1:
		return address - width
	case 2:
		return address
	case 3:
		if destination {
			// Increment/reload increments during the run. The reload part is
			// applied when a repeated event-driven transfer is re-armed.
			return address + width
		}
		// Source mode 3 is prohibited on GBA. Keep it stable/deterministic
		// rather than inventing an undocumented address progression.
		return address
	default:
		return address
	}
}

func isGamePak(address uint32) bool {
	return address >= bus.ROM0Start && address < bus.SaveStart
}

// Reset clears initial/internal state and disables all channels.
func (d *DMA) Reset() {
	d.ch = [4]channel{}
	d.pending = 0
	d.servicing = false
}

// Source returns the programmed initial source address.
func (d *DMA) Source(index int) uint32 { return d.ch[index].sourceInitial }

// Destination returns the programmed initial destination address.
func (d *DMA) Destination(index int) uint32 { return d.ch[index].destInitial }

// Count returns the programmed initial word count.
func (d *DMA) Count(index int) uint16 { return d.ch[index].countInitial }

// Control returns the masked DMAxCNT_H value.
func (d *DMA) Control(index int) uint16 { return d.ch[index].control }

// LastTransfer reports the most recent completed transfer's unit/cycle count.
func (d *DMA) LastTransfer(index int) (units uint32, cycles uint32) {
	c := &d.ch[index]
	return c.lastUnits, c.lastCycles
}

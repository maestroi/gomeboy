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

	fifoAAddress uint32 = bus.IOStart + 0x0a0
	fifoBAddress uint32 = bus.IOStart + 0x0a4
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
)

// SoundFIFO identifies one of the two GBA Direct Sound FIFOs.
type SoundFIFO uint8

const (
	FIFOA SoundFIFO = iota + 1
	FIFOB
)

// Hooks exposes DMA timing to scheduler/debug integration.
// Complete fires once per completed channel. Stall fires once for a serviced
// request batch with the total number of CPU-stall cycles consumed by DMA.
//
// RequestStart switches DMA into scheduler-owned mode. When non-nil, pending
// channel bits are passed to the owner, which applies start latency before
// calling ActivatePending. CancelStart fires when software disables a channel
// so its scheduled deadline can be discarded.
type Hooks struct {
	Complete     func(channel int, units uint32, cycles uint32)
	Stall        func(cycles uint32)
	RequestStart func(channels uint8)
	CancelStart  func(channel int)
}

type channel struct {
	sourceInitial uint32
	destInitial   uint32
	countInitial  uint16
	control       uint16

	sourceCurrent uint32
	destCurrent   uint32
	countCurrent  uint32
	destReload    uint32
	countReload   uint32

	lastCycles uint32
	lastUnits  uint32

	// disableAfterRun marks a final event-driven transfer (currently the last
	// DMA3 video-capture scanline) that must clear Enable even with Repeat set.
	disableAfterRun bool

	// Scheduler-owned transfers are serviced one unit at a time so channel
	// priority can be reconsidered between units.
	transferActive      bool
	completionPending   bool
	transferRemaining   uint32
	transferUnits       uint32
	transferWidth       uint32
	transferSourceMode  uint16
	transferDestMode    uint16
	transferStartSource uint32
	transferStartDest   uint32
	transferCycles      uint32
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
		if !newEnabled {
			c.disableAfterRun = false
			d.pending &^= 1 << index
			if d.hooks.CancelStart != nil {
				d.hooks.CancelStart(index)
			}
		}
		return
	}

	d.latch(index)
	// DMA3 Game Pak DRQ replaces the ordinary start condition: even timing=Now
	// waits for the cartridge request instead of running on the Enable edge.
	gamePakDRQ := index == 3 && c.control&controlGamePakDRQ != 0
	if c.control&controlTimingMask == timingImmediate && !gamePakDRQ {
		d.queue(index)
		d.requestPending()
	}
}

func (d *DMA) latch(index int) {
	c := &d.ch[index]
	width := d.width(index)
	alignMask := ^uint32(width - 1)
	c.sourceCurrent = c.sourceInitial & alignMask
	c.destReload = c.destInitial & alignMask
	c.destCurrent = c.destReload
	c.countReload = effectiveCount(index, c.countInitial)
	c.countCurrent = c.countReload
	c.disableAfterRun = false
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
		// Game Pak DRQ is a separate DMA3 request source and overrides the
		// programmed ordinary start timing.
		if index == 3 && c.control&controlGamePakDRQ != 0 {
			continue
		}
		d.queue(index)
	}
	return d.requestPending()
}

// TriggerVideoCapture requests DMA3's display-synchronized special transfer
// at the start of one scanline. GBA capture is active for VCOUNT 2..161; the
// line-161 transfer is the final burst and clears Enable even with Repeat set.
func (d *DMA) TriggerVideoCapture(vcount uint16) uint32 {
	if vcount < 2 || vcount >= 162 {
		return 0
	}

	c := &d.ch[3]
	if c.control&controlEnable == 0 ||
		c.control&controlTimingMask != timingSpecial ||
		c.control&controlGamePakDRQ != 0 {
		return 0
	}

	if vcount == 161 {
		c.disableAfterRun = true
	}
	d.queue(3)
	return d.requestPending()
}

// TriggerGamePakDRQ services one external Game Pak data request. The request
// source is DMA3 bit 11 and overrides DMA3's programmed start-timing field.
// One request runs the complete latched word count and completion is one-shot;
// hardware requires Repeat=0 for this mode.
func (d *DMA) TriggerGamePakDRQ() uint32 {
	c := &d.ch[3]
	if c.control&controlEnable == 0 || c.control&controlGamePakDRQ == 0 {
		return 0
	}
	d.queue(3)
	return d.requestPending()
}

// TriggerFIFO requests a Direct Sound FIFO refill. Only DMA1/DMA2 channels
// armed for Special timing and latched to the requested FIFO participate.
// The APU owns the FIFO fill-level threshold and should call this when the
// hardware requests another burst.
func (d *DMA) TriggerFIFO(fifo SoundFIFO) uint32 {
	dest, ok := soundFIFOAddress(fifo)
	if !ok {
		return 0
	}

	for index := 1; index <= 2; index++ {
		c := &d.ch[index]
		if c.control&controlEnable == 0 || c.control&controlTimingMask != timingSpecial {
			continue
		}
		if c.destReload != dest {
			continue
		}
		d.queue(index)
	}
	return d.requestPending()
}

func soundFIFOAddress(fifo SoundFIFO) (uint32, bool) {
	switch fifo {
	case FIFOA:
		return fifoAAddress, true
	case FIFOB:
		return fifoBAddress, true
	default:
		return 0, false
	}
}

func (d *DMA) soundFIFOTransfer(index int) (uint32, bool) {
	if index != 1 && index != 2 {
		return 0, false
	}
	c := &d.ch[index]
	if c.control&controlTimingMask != timingSpecial {
		return 0, false
	}
	switch c.destReload {
	case fifoAAddress, fifoBAddress:
		return c.destReload, true
	default:
		return 0, false
	}
}

func timingForEvent(event StartEvent) (uint16, bool) {
	switch event {
	case StartVBlank:
		return timingVBlank, true
	case StartHBlank:
		return timingHBlank, true
	default:
		return 0, false
	}
}

func (d *DMA) queue(index int) {
	c := &d.ch[index]
	if c.transferActive || c.completionPending {
		// A channel cannot queue a second hardware request while its current
		// request is still being serviced.
		return
	}
	d.pending |= 1 << index
}

func (d *DMA) requestPending() uint32 {
	if d.pending == 0 {
		return 0
	}
	if d.hooks.RequestStart != nil {
		d.hooks.RequestStart(d.pending)
		return 0
	}
	return d.servicePending()
}

// Pending reports whether one or more channels are waiting for start latency.
func (d *DMA) Pending() bool { return d.pending != 0 }

// PendingMask returns channels waiting for scheduler activation.
func (d *DMA) PendingMask() uint8 { return d.pending }

// Active reports whether at least one DMA request is mid-transfer.
func (d *DMA) Active() bool { return d.ActiveMask() != 0 }

// ActiveMask returns channels currently eligible for unit service.
func (d *DMA) ActiveMask() uint8 {
	var mask uint8
	for index := 0; index < 4; index++ {
		if d.ch[index].transferActive {
			mask |= 1 << index
		}
	}
	return mask
}

// UnitResult describes one scheduler-visible DMA transfer unit.
type UnitResult struct {
	Channel   int
	Cycles    uint32
	Completed bool
}

// ActivatePending moves selected pending channels past their start latency.
// The next ServiceUnit call arbitrates all active channels by DMA priority.
func (d *DMA) ActivatePending(mask uint8) {
	mask &= d.pending
	for index := 0; index < 4; index++ {
		bit := uint8(1 << index)
		if mask&bit == 0 {
			continue
		}
		d.pending &^= bit
		c := &d.ch[index]
		if c.control&controlEnable == 0 || c.transferActive || c.completionPending {
			continue
		}
		d.startTransfer(index)
	}
}

func (d *DMA) startTransfer(index int) {
	c := &d.ch[index]
	units := c.countCurrent
	width := d.width(index)
	sourceMode := (c.control & controlSourceMask) >> 7
	destMode := (c.control & controlDestMask) >> 5

	if fifoDest, ok := d.soundFIFOTransfer(index); ok {
		units = 4
		width = 4
		destMode = 2
		c.sourceCurrent &^= 3
		c.destCurrent = fifoDest
	}
	if units == 0 {
		return
	}

	c.transferActive = true
	c.transferRemaining = units
	c.transferUnits = units
	c.transferWidth = width
	c.transferSourceMode = sourceMode
	c.transferDestMode = destMode
	c.transferStartSource = c.sourceCurrent
	c.transferStartDest = c.destCurrent
	c.transferCycles = 0
}

// ServiceUnit transfers one unit from the highest-priority active channel.
// Completion side effects are deferred to FinishUnit so scheduler integrations
// can advance the unit's elapsed bus/internal cycles before raising DMA IRQs.
func (d *DMA) ServiceUnit() UnitResult {
	index := -1
	for candidate := 0; candidate < 4; candidate++ {
		if d.ch[candidate].transferActive {
			index = candidate
			break
		}
	}
	if index < 0 {
		return UnitResult{Channel: -1}
	}

	c := &d.ch[index]
	unit := c.transferUnits - c.transferRemaining
	access := bus.Access{Sequential: unit != 0, DMA: true}
	source := c.sourceCurrent
	dest := c.destCurrent

	var cycles uint32
	if c.transferWidth == 4 {
		value, readCycles := d.bus.Read32(source, access)
		writeCycles := d.bus.Write32(dest, value, access)
		cycles = readCycles + writeCycles
	} else {
		value, readCycles := d.bus.Read16(source, access)
		writeCycles := d.bus.Write16(dest, value, access)
		cycles = readCycles + writeCycles
	}

	c.sourceCurrent = adjustAddress(source, c.transferSourceMode, c.transferWidth, false)
	c.destCurrent = adjustAddress(dest, c.transferDestMode, c.transferWidth, true)
	c.transferRemaining--
	c.transferCycles += cycles

	completed := c.transferRemaining == 0
	if completed {
		// Preserve the existing aggregate timing model: two internal cycles once
		// per transfer, or four when both endpoints are on the Game Pak bus.
		internal := uint32(2)
		if isGamePak(c.transferStartSource) && isGamePak(c.transferStartDest) {
			internal = 4
		}
		cycles += internal
		c.transferCycles += internal
		c.transferActive = false
		c.completionPending = true
		c.countCurrent = 0
	}

	return UnitResult{Channel: index, Cycles: cycles, Completed: completed}
}

// FinishUnit applies completion/repeat/IRQ state for a unit that returned
// Completed. Call it after the unit's cycles have elapsed on the scheduler.
func (d *DMA) FinishUnit(index int) {
	if index < 0 || index >= 4 {
		return
	}
	c := &d.ch[index]
	if !c.completionPending {
		return
	}
	c.completionPending = false

	cycles := c.transferCycles
	units := c.transferUnits
	c.lastCycles = cycles
	c.lastUnits = units

	if c.control&controlIRQ != 0 && d.irq != nil {
		d.irq.Request(irqSources[index])
	}

	timing := c.control & controlTimingMask
	gamePakDRQ := index == 3 && c.control&controlGamePakDRQ != 0
	repeat := timing != timingImmediate &&
		c.control&controlRepeat != 0 &&
		!gamePakDRQ &&
		!c.disableAfterRun
	c.disableAfterRun = false
	if repeat {
		c.countCurrent = c.countReload
		if c.transferDestMode == 3 {
			c.destCurrent = c.destReload
		}
	} else {
		c.control &^= controlEnable
		c.countCurrent = 0
	}

	if d.hooks.Complete != nil {
		d.hooks.Complete(index, units, cycles)
	}
}

// ServicePending synchronously drains pending/active requests in hardware
// priority order. Scheduler integrations normally use ActivatePending and
// ServiceUnit instead so external events can preempt between units.
func (d *DMA) ServicePending() uint32 { return d.servicePending() }

func (d *DMA) servicePending() uint32 {
	if d.servicing {
		return 0
	}
	d.servicing = true
	defer func() { d.servicing = false }()

	var total uint32
	for d.pending != 0 || d.Active() {
		if d.pending != 0 {
			d.ActivatePending(d.pending)
		}
		unit := d.ServiceUnit()
		if unit.Cycles == 0 {
			break
		}
		total += unit.Cycles
		if unit.Completed {
			d.FinishUnit(unit.Channel)
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
	width := d.width(index)
	sourceMode := (c.control & controlSourceMask) >> 7
	destMode := (c.control & controlDestMask) >> 5

	source := c.sourceCurrent
	dest := c.destCurrent
	if fifoDest, ok := d.soundFIFOTransfer(index); ok {
		// Direct Sound FIFO requests always transfer four 32-bit words and
		// keep the destination fixed, regardless of CNT_L/transfer-width/
		// destination-control programming. Source direction is preserved.
		units = 4
		width = 4
		destMode = 2
		source &^= 3
		dest = fifoDest
	}
	if units == 0 {
		return 0
	}

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
	gamePakDRQ := index == 3 && c.control&controlGamePakDRQ != 0
	repeat := timing != timingImmediate &&
		c.control&controlRepeat != 0 &&
		!gamePakDRQ &&
		!c.disableAfterRun
	c.disableAfterRun = false
	if repeat {
		c.countCurrent = c.countReload
		if destMode == 3 {
			c.destCurrent = c.destReload
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

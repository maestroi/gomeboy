// Package audio implements Game Boy Advance audio hardware.
package audio

import (
	"github.com/maestroi/gomeboy/internal/gba/bus"
)

const (
	soundControlHighOffset uint32 = 0x082
	fifoAOffset            uint32 = 0x0a0
	fifoBOffset            uint32 = 0x0a4

	fifoCapacity     = 32
	fifoDMAThreshold = 16

	controlPSGVolumeMask uint16 = 0x0003
	controlVolumeA       uint16 = 1 << 2
	controlVolumeB       uint16 = 1 << 3
	controlARight        uint16 = 1 << 8
	controlALeft         uint16 = 1 << 9
	controlATimer        uint16 = 1 << 10
	controlAReset        uint16 = 1 << 11
	controlBRight        uint16 = 1 << 12
	controlBLeft         uint16 = 1 << 13
	controlBTimer        uint16 = 1 << 14
	controlBReset        uint16 = 1 << 15

	controlStoredMask = controlPSGVolumeMask | controlVolumeA | controlVolumeB |
		controlARight | controlALeft | controlATimer |
		controlBRight | controlBLeft | controlBTimer
)

// FIFO identifies one of the two GBA Direct Sound FIFOs.
type FIFO uint8

const (
	FIFOA FIFO = iota + 1
	FIFOB
)

// Hooks exposes Direct Sound refill requests to the DMA controller.
type Hooks struct {
	RequestFIFO func(FIFO)
}

type byteFIFO struct {
	data [fifoCapacity]byte
	head int
	size int
}

func (f *byteFIFO) clear() {
	f.head = 0
	f.size = 0
}

func (f *byteFIFO) push(value byte) {
	if f.size >= fifoCapacity {
		return
	}
	index := (f.head + f.size) % fifoCapacity
	f.data[index] = value
	f.size++
}

func (f *byteFIFO) pop() (byte, bool) {
	if f.size == 0 {
		return 0, false
	}
	value := f.data[f.head]
	f.head = (f.head + 1) % fifoCapacity
	f.size--
	return value, true
}

type directSoundChannel struct {
	fifo    byteFIFO
	current int8
}

// Audio owns the GBA Direct Sound control register and FIFO state. Mixing and
// legacy PSG channels are intentionally layered on later.
type Audio struct {
	bus   *bus.Bus
	hooks Hooks

	controlHigh uint16
	channel     [2]directSoundChannel
}

// New maps SOUNDCNT_H and FIFO_A/FIFO_B onto b.
func New(b *bus.Bus, hooks Hooks) *Audio {
	if b == nil {
		panic("gba audio: nil bus")
	}
	a := &Audio{bus: b, hooks: hooks}
	a.install(b.IO())
	return a
}

func (a *Audio) install(io *bus.IO) {
	io.Register16(soundControlHighOffset,
		func() uint16 { return a.controlHigh },
		a.writeControlHigh,
	)

	for index, base := range [...]uint32{fifoAOffset, fifoBOffset} {
		index := index
		base := base
		for half := 0; half < 2; half++ {
			offset := base + uint32(half*2)
			io.Register16WithByteWrite(offset,
				func() uint16 { return a.openBusHalfword(offset) },
				func(value uint16) {
					a.pushByte(index, byte(value))
					a.pushByte(index, byte(value>>8))
				},
				func(_ uint32, value byte) {
					a.pushByte(index, value)
				},
			)
		}
	}
}

func (a *Audio) openBusHalfword(ioOffset uint32) uint16 {
	value := a.bus.OpenBus()
	if ioOffset&2 != 0 {
		return uint16(value >> 16)
	}
	return uint16(value)
}

func (a *Audio) writeControlHigh(value uint16) {
	if value&controlAReset != 0 {
		a.channel[0].fifo.clear()
	}
	if value&controlBReset != 0 {
		a.channel[1].fifo.clear()
	}
	// FIFO reset bits are strobes and therefore read back as zero.
	a.controlHigh = value & controlStoredMask
}

func (a *Audio) pushByte(index int, value byte) {
	a.channel[index].fifo.push(value)
}

// TimerOverflow consumes one signed 8-bit Direct Sound sample for every
// overflow of the selected timer. Only timers 0 and 1 can drive Direct Sound.
//
// After each consumed sample, a FIFO at or below half-full requests a 4-word
// DMA refill. The DMA controller coalesces repeated requests while a burst is
// already pending or active.
func (a *Audio) TimerOverflow(timer int, count uint32) {
	if timer < 0 || timer > 1 || count == 0 {
		return
	}
	for ; count > 0; count-- {
		for index := 0; index < len(a.channel); index++ {
			if a.timer(index) != timer {
				continue
			}
			value, ok := a.channel[index].fifo.pop()
			if ok {
				a.channel[index].current = int8(value)
			} else {
				a.channel[index].current = 0
			}
			if a.channel[index].fifo.size <= fifoDMAThreshold && a.hooks.RequestFIFO != nil {
				a.hooks.RequestFIFO(FIFO(index + 1))
			}
		}
	}
}

func (a *Audio) timer(index int) int {
	if index == 0 {
		if a.controlHigh&controlATimer != 0 {
			return 1
		}
		return 0
	}
	if a.controlHigh&controlBTimer != 0 {
		return 1
	}
	return 0
}

// ControlHigh returns the readable SOUNDCNT_H state. FIFO reset bits are
// self-clearing and are never present in the returned value.
func (a *Audio) ControlHigh() uint16 { return a.controlHigh }

// FIFOLevel returns the number of queued bytes in fifo.
func (a *Audio) FIFOLevel(fifo FIFO) int {
	index, ok := fifoIndex(fifo)
	if !ok {
		return 0
	}
	return a.channel[index].fifo.size
}

// CurrentSample returns the most recently consumed signed 8-bit PCM sample.
func (a *Audio) CurrentSample(fifo FIFO) int8 {
	index, ok := fifoIndex(fifo)
	if !ok {
		return 0
	}
	return a.channel[index].current
}

// Timer returns the selected timer (0 or 1) for fifo.
func (a *Audio) Timer(fifo FIFO) int {
	index, ok := fifoIndex(fifo)
	if !ok {
		return 0
	}
	return a.timer(index)
}

// Reset clears Direct Sound control, FIFO contents, and sample latches.
func (a *Audio) Reset() {
	a.controlHigh = 0
	a.channel = [2]directSoundChannel{}
}

func fifoIndex(fifo FIFO) (int, bool) {
	switch fifo {
	case FIFOA:
		return 0, true
	case FIFOB:
		return 1, true
	default:
		return 0, false
	}
}

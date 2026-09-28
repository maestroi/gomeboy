// Package audio implements Game Boy Advance audio hardware.
package audio

import (
	"sync/atomic"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

const (
	soundControlHighOffset uint32 = 0x082
	soundBiasOffset        uint32 = 0x088
	fifoAOffset            uint32 = 0x0a0
	fifoBOffset            uint32 = 0x0a4

	fifoCapacity     = 32
	fifoDMAThreshold = 16

	masterClockHz uint64 = 1 << 24
	// SampleRate matches the existing desktop audio pipeline and GB/GBC APU.
	SampleRate uint64 = 96000
	outputBufferSize = 1634 * 4

	soundBiasMask uint16 = 0xc3fe
	defaultSoundBias uint16 = 0x0200

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

// Audio owns the GBA Direct Sound control/FIFO state and produces host-rate
// stereo samples. Legacy PSG channels are intentionally layered on later.
type Audio struct {
	bus   *bus.Bus
	hooks Hooks

	controlHigh uint16
	soundBias   uint16
	channel     [2]directSoundChannel
	psg         psgState

	buffer      []float32
	bufferPos   uint32
	samplePhase uint64
	headless    bool
	mute        atomic.Bool
}

// New maps SOUNDCNT_H and FIFO_A/FIFO_B onto b.
func New(b *bus.Bus, hooks Hooks) *Audio {
	if b == nil {
		panic("gba audio: nil bus")
	}
	a := &Audio{bus: b, hooks: hooks, soundBias: defaultSoundBias, buffer: make([]float32, outputBufferSize)}
	a.install(b.IO())
	a.installPSG(b.IO())
	return a
}

func (a *Audio) install(io *bus.IO) {
	// These unused PSG/control halfwords are readable as zero rather than
	// behaving like ordinary unmapped I/O/open bus.
	for _, offset := range [...]uint32{0x066, 0x06e, 0x076, 0x07a, 0x07e, 0x086, 0x08a} {
		io.Register16(offset, func() uint16 { return 0 }, nil)
	}

	io.Register16(soundControlHighOffset,
		func() uint16 { return a.controlHigh },
		a.writeControlHigh,
	)
	io.Register16(soundBiasOffset,
		func() uint16 { return a.soundBias },
		func(value uint16) { a.soundBias = value & soundBiasMask },
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

// Advance advances host audio sampling by GBA master-clock cycles. Direct
// Sound latches themselves are updated by TimerOverflow; this method only
// samples those latches into the host-rate stereo buffer.
func (a *Audio) Advance(cycles uint32) {
	if cycles == 0 {
		return
	}

	// Headless consumers need hardware-visible PSG/timer/FIFO state but do not
	// need to visit every 96 kHz host sample boundary. Preserve the host sample
	// phase so toggling output back on remains deterministic.
	if a.headless {
		step := uint64(cycles)
		a.advancePSG(step)
		a.samplePhase = (a.samplePhase + step*SampleRate) % masterClockHz
		return
	}

	remaining := uint64(cycles)
	for remaining > 0 {
		untilSample := (masterClockHz - a.samplePhase + SampleRate - 1) / SampleRate
		step := remaining
		if untilSample < step {
			step = untilSample
		}
		if a.psg.enabled {
			untilFrame := psgFramePeriod - a.psg.frameCycles
			if untilFrame < step {
				step = untilFrame
			}
		}

		a.advancePSG(step)
		a.samplePhase += step * SampleRate
		remaining -= step

		if a.samplePhase >= masterClockHz {
			a.samplePhase -= masterClockHz
			if !a.headless {
				left, right := a.mixOutput()
				a.appendSample(left, right)
			}
		}
	}
}

func (a *Audio) mixDirectSound() (float32, float32) {
	var left, right int32
	for index := range a.channel {
		sample := int32(a.channel[index].current) << 2
		if (index == 0 && a.controlHigh&controlVolumeA == 0) ||
			(index == 1 && a.controlHigh&controlVolumeB == 0) {
			sample >>= 1
		}
		if (index == 0 && a.controlHigh&controlALeft != 0) ||
			(index == 1 && a.controlHigh&controlBLeft != 0) {
			left += sample
		}
		if (index == 0 && a.controlHigh&controlARight != 0) ||
			(index == 1 && a.controlHigh&controlBRight != 0) {
			right += sample
		}
	}
	return a.applyBias(left), a.applyBias(right)
}

func (a *Audio) mixOutput() (float32, float32) {
	var left, right int32
	for index := range a.channel {
		sample := int32(a.channel[index].current) << 2
		if (index == 0 && a.controlHigh&controlVolumeA == 0) ||
			(index == 1 && a.controlHigh&controlVolumeB == 0) {
			sample >>= 1
		}
		if (index == 0 && a.controlHigh&controlALeft != 0) ||
			(index == 1 && a.controlHigh&controlBLeft != 0) {
			left += sample
		}
		if (index == 0 && a.controlHigh&controlARight != 0) ||
			(index == 1 && a.controlHigh&controlBRight != 0) {
			right += sample
		}
	}
	psgLeft, psgRight := a.mixPSG()
	left += psgLeft
	right += psgRight
	return a.applyBias(left), a.applyBias(right)
}

func (a *Audio) applyBias(sample int32) float32 {
	bias := int32(a.soundBias & 0x03fe)
	value := sample + bias
	if value < 0 {
		value = 0
	} else if value > 0x03ff {
		value = 0x03ff
	}
	return float32(value-bias) / 512.0
}

func (a *Audio) appendSample(left, right float32) {
	if a.mute.Load() {
		left, right = 0, 0
	}
	needed := int(a.bufferPos) + 2
	if needed > len(a.buffer) {
		a.buffer = append(a.buffer, make([]float32, needed-len(a.buffer))...)
	}
	a.buffer[a.bufferPos] = left
	a.buffer[a.bufferPos+1] = right
	a.bufferPos += 2
}

// Samples returns interleaved stereo float32 output and clears the readable
// portion of the host buffer. The count is the number of float32 values.
func (a *Audio) Samples() ([]float32, uint32) {
	samples := a.buffer[:a.bufferPos]
	count := a.bufferPos
	a.bufferPos = 0
	if len(a.buffer) > outputBufferSize {
		a.buffer = a.buffer[:outputBufferSize]
	}
	return samples, count
}

// SetHeadless suppresses host sample buffering while preserving all
// hardware-visible Direct Sound FIFO/timer state.
func (a *Audio) SetHeadless(headless bool) {
	a.headless = headless
	if headless {
		a.bufferPos = 0
	}
}

// SetMute controls host output only; hardware-visible state keeps advancing.
func (a *Audio) SetMute(mute bool) {
	a.mute.Store(mute)
}

// CurrentOutput returns the current routed, volume-scaled Direct Sound mix.
func (a *Audio) CurrentOutput() (left, right float32) {
	return a.mixOutput()
}

// SoundBias returns the masked SOUNDBIAS register state.
func (a *Audio) SoundBias() uint16 { return a.soundBias }

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

// Reset clears Direct Sound control, FIFO contents, sample latches, and host
// output timing. SOUNDBIAS returns to the post-BIOS neutral midpoint used by
// the standalone core.
func (a *Audio) Reset() {
	a.controlHigh = 0
	a.soundBias = defaultSoundBias
	a.channel = [2]directSoundChannel{}
	waveRAM := a.psg.wave.ram
	a.psg = psgState{}
	a.psg.wave.ram = waveRAM
	a.samplePhase = 0
	a.bufferPos = 0
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

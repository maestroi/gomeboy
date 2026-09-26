// Package timer implements the four Game Boy Advance hardware timers.
package timer

import (
	"math"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

const (
	firstTimerOffset uint32 = 0x100
	timerStride             = 4

	controlPrescalerMask uint16 = 0x0003
	controlCountUp       uint16 = 1 << 2
	controlIRQ           uint16 = 1 << 6
	controlEnable        uint16 = 1 << 7
)

var prescalers = [4]uint32{1, 64, 256, 1024}

var irqSources = [4]gbairq.Source{
	gbairq.Timer0,
	gbairq.Timer1,
	gbairq.Timer2,
	gbairq.Timer3,
}

// IRQSink receives timer-overflow interrupt requests.
type IRQSink interface {
	Request(gbairq.Source)
}

// Hooks expose timer overflow counts to later audio/system integration.
// One callback may represent multiple overflows when Advance is called with a
// large cycle interval.
type Hooks struct {
	Overflow func(timer int, count uint32)
}

type state struct {
	reload  uint16
	counter uint16
	control uint16
	phase   uint32
}

// Timers owns TM0-TM3 register state and deterministic cycle advancement.
type Timers struct {
	irq   IRQSink
	hooks Hooks

	timer [4]state
}

// New maps TM0-TM3 onto b.
func New(b *bus.Bus, irq IRQSink, hooks Hooks) *Timers {
	if b == nil {
		panic("gba timer: nil bus")
	}
	t := &Timers{irq: irq, hooks: hooks}
	t.install(b.IO())
	return t
}

func (t *Timers) install(io *bus.IO) {
	for index := 0; index < 4; index++ {
		index := index
		low := firstTimerOffset + uint32(index*timerStride)
		high := low + 2

		io.Register16WithByteWrite(low,
			func() uint16 { return t.timer[index].counter },
			func(value uint16) { t.timer[index].reload = value },
			func(byteOffset uint32, value byte) {
				current := t.timer[index].reload
				if byteOffset == 0 {
					current = current&0xff00 | uint16(value)
				} else {
					current = current&0x00ff | uint16(value)<<8
				}
				t.timer[index].reload = current
			},
		)

		io.Register16(high,
			func() uint16 { return t.timer[index].control },
			func(value uint16) { t.writeControl(index, value) },
		)
	}
}

func controlMask(index int) uint16 {
	if index == 0 {
		return controlPrescalerMask | controlIRQ | controlEnable
	}
	return controlPrescalerMask | controlCountUp | controlIRQ | controlEnable
}

func (t *Timers) writeControl(index int, value uint16) {
	s := &t.timer[index]
	oldControl := s.control
	s.control = value & controlMask(index)

	oldEnabled := oldControl&controlEnable != 0
	newEnabled := s.control&controlEnable != 0
	if !oldEnabled && newEnabled {
		s.counter = s.reload
		s.phase = 0
		return
	}
	if oldEnabled && !newEnabled {
		s.phase = 0
		return
	}

	// Keep the accumulated prescaler position valid when software changes the
	// divisor while the timer remains enabled.
	if newEnabled && s.control&controlCountUp == 0 {
		divisor := prescalers[s.control&controlPrescalerMask]
		if divisor != 0 {
			s.phase %= divisor
		}
	}
}

// CyclesUntilEvent returns the master-clock distance to the next overflow of
// any independently clocked timer. Count-up timers are driven by their parent
// overflow at that same edge and therefore do not need a separate deadline.
func (t *Timers) CyclesUntilEvent() uint32 {
	best := uint64(math.MaxUint32)
	for index := 0; index < 4; index++ {
		s := &t.timer[index]
		if s.control&controlEnable == 0 {
			continue
		}
		if index > 0 && s.control&controlCountUp != 0 {
			continue
		}

		divisor := uint64(prescalers[s.control&controlPrescalerMask])
		ticks := uint64(0x10000 - uint32(s.counter))
		cycles := ticks*divisor - uint64(s.phase)
		if cycles < best {
			best = cycles
		}
	}
	return uint32(best)
}

// Advance advances all enabled timers by GBA master-clock cycles.
//
// Normal timers derive ticks from their prescaler. Count-up timers 1-3 ignore
// the prescaler and receive one tick per overflow of the previous timer.
func (t *Timers) Advance(cycles uint32) {
	var overflows [4]uint32

	for index := 0; index < 4; index++ {
		s := &t.timer[index]
		if s.control&controlEnable == 0 {
			continue
		}

		var ticks uint64
		if index > 0 && s.control&controlCountUp != 0 {
			ticks = uint64(overflows[index-1])
		} else {
			divisor := prescalers[s.control&controlPrescalerMask]
			total := uint64(s.phase) + uint64(cycles)
			ticks = total / uint64(divisor)
			s.phase = uint32(total % uint64(divisor))
		}

		if ticks == 0 {
			continue
		}

		overflows[index] = t.advanceCounter(index, ticks)
		if overflows[index] == 0 {
			continue
		}

		if t.hooks.Overflow != nil {
			t.hooks.Overflow(index, overflows[index])
		}
		if s.control&controlIRQ != 0 && t.irq != nil {
			t.irq.Request(irqSources[index])
		}
	}
}

func (t *Timers) advanceCounter(index int, ticks uint64) uint32 {
	s := &t.timer[index]
	untilOverflow := uint64(0x10000 - uint32(s.counter))
	if ticks < untilOverflow {
		s.counter += uint16(ticks)
		return 0
	}

	ticks -= untilOverflow
	overflows := uint64(1)
	s.counter = s.reload

	period := uint64(0x10000 - uint32(s.reload))
	overflows += ticks / period
	ticks %= period
	s.counter = uint16(uint32(s.reload) + uint32(ticks))
	return uint32(overflows)
}

// Reset clears timer reload/counter/control/prescaler state.
func (t *Timers) Reset() {
	t.timer = [4]state{}
}

// Counter returns the current live/frozen counter for tests/debugging.
func (t *Timers) Counter(index int) uint16 { return t.timer[index].counter }

// Reload returns the programmed reload latch.
func (t *Timers) Reload(index int) uint16 { return t.timer[index].reload }

// Control returns the masked TMxCNT_H value.
func (t *Timers) Control(index int) uint16 { return t.timer[index].control }

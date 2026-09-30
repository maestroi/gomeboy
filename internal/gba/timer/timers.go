// Package timer implements the four Game Boy Advance hardware timers.
package timer

import (
	"math"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	gbascheduler "github.com/maestroi/gomeboy/internal/gba/scheduler"
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
type Hooks struct {
	Overflow func(timer int, count uint32)
}

type state struct {
	reload           uint16
	pendingReload    uint16
	counter          uint16
	control          uint16
	pendingControl   uint16
	timestampStarted uint64
	lastTickAt       uint64
	lastOverflowAt   uint64
	overflowEvent    gbascheduler.Handle
}

// Timers owns TM0-TM3 register state and deterministic cycle advancement.
type Timers struct {
	irq       IRQSink
	hooks     Hooks
	scheduler *gbascheduler.Scheduler

	timer  [4]state
	active uint8

	deferWrites bool
}

// New maps TM0-TM3 onto b.
func New(b *bus.Bus, irq IRQSink, hooks Hooks) *Timers {
	return NewWithScheduler(b, irq, hooks, gbascheduler.New())
}

// NewWithScheduler creates timers on a shared GBA master-clock scheduler.
func NewWithScheduler(b *bus.Bus, irq IRQSink, hooks Hooks, scheduler *gbascheduler.Scheduler) *Timers {
	if b == nil {
		panic("gba timer: nil bus")
	}
	if scheduler == nil {
		panic("gba timer: nil scheduler")
	}
	t := &Timers{irq: irq, hooks: hooks, scheduler: scheduler}
	t.install(b.IO())
	return t
}

func (t *Timers) install(io *bus.IO) {
	for index := 0; index < 4; index++ {
		index := index
		low := firstTimerOffset + uint32(index*timerStride)
		high := low + 2

		io.Register16WithByteWrite(low,
			func() uint16 { return t.Counter(index) },
			func(value uint16) { t.writeReload(index, value) },
			func(byteOffset uint32, value byte) {
				current := t.reloadForWrite(index)
				if byteOffset == 0 {
					current = current&0xff00 | uint16(value)
				} else {
					current = current&0x00ff | uint16(value)<<8
				}
				t.writeReload(index, current)
			},
		)

		io.Register16(high,
			func() uint16 { return t.timer[index].control },
			func(value uint16) { t.writeControl(index, value) },
		)
	}
}

// BeginWriteAccess marks timer writes performed by a CPU instruction. Hardware
// register effects are timestamped one master cycle after the I/O write.
func (t *Timers) BeginWriteAccess() {
	if t.deferWrites {
		return
	}
	t.deferWrites = true
}

// EndWriteBusAccess is retained for the CPU bus adapter. Timestamped timer
// events make the bus-completion boundary explicit in scheduler time.
func (t *Timers) EndWriteBusAccess() {}

// EndWriteAccess ends CPU-write deferral. Scheduled register events remain in
// the queue and commit at their timestamp.
func (t *Timers) EndWriteAccess() {
	t.deferWrites = false
}

func (t *Timers) writeReload(index int, value uint16) {
	s := &t.timer[index]
	s.pendingReload = value
	// TMxCNT_L is a reload latch, not a start/stop control. In GomeBoy's CPU
	// memory callback ordering the I/O write is sampled before the bus phase is
	// consumed, so make the latch visible here. If an overflow lands during the
	// same transfer it must see the newly written reload value.
	s.reload = value
}

func (t *Timers) reloadForWrite(index int) uint16 {
	s := &t.timer[index]
	if t.deferWrites {
		return s.pendingReload
	}
	return s.reload
}

func controlMask(index int) uint16 {
	if index == 0 {
		return controlPrescalerMask | controlIRQ | controlEnable
	}
	return controlPrescalerMask | controlCountUp | controlIRQ | controlEnable
}

func (t *Timers) writeControl(index int, value uint16) {
	s := &t.timer[index]
	s.pendingControl = value & controlMask(index)
	if t.deferWrites {
		// The CPU core invokes MMIO writes before consuming the transfer's bus
		// phase. Starts/config changes commit one scheduler cycle later.
		delay := uint64(1)
		t.scheduler.Schedule(delay, gbascheduler.PriorityLate, func() {
			t.applyControl(index, s.pendingControl, true)
		})
		return
	}
	t.applyControl(index, s.pendingControl, false)
}

// applyControl mirrors the event-based timer model used by mature GBA cores:
// the prescaler is a free-running divider of the global master clock, so starts
// and frequency changes align to scheduler time instead of a timer-local phase.
func (t *Timers) applyControl(index int, value uint16, cpuWrite bool) {
	s := &t.timer[index]
	oldControl := s.control
	newControl := value & controlMask(index)
	oldEnabled := oldControl&controlEnable != 0
	newEnabled := newControl&controlEnable != 0
	oldCascade := index > 0 && oldControl&controlCountUp != 0
	newCascade := index > 0 && newControl&controlCountUp != 0

	// Materialize the old independently-clocked timer before changing its mode.
	// Count-up timers have no local clock to materialize.
	if oldEnabled && !oldCascade {
		t.syncCounter(index)
		t.cancelOverflow(index)
	}

	s.control = newControl
	s.pendingControl = newControl

	bit := uint8(1 << index)
	if newEnabled {
		t.active |= bit
	} else {
		t.active &^= bit
	}

	if !newEnabled {
		return
	}

	if oldEnabled {
		// Rewriting control while enabled preserves the live counter, but a
		// non-cascade timer is realigned to the global prescaler phase.
		if !newCascade {
			offset := int64(t.scheduler.Now() % uint64(prescalers[newControl&controlPrescalerMask]))
			t.startChannel(index, offset)
		}
		return
	}

	// A 0->1 enable edge loads the reload latch. Cascade timers begin waiting
	// for their parent immediately; ordinary timers spend one extra cycle
	// loading on a CPU write before their first prescaler tick can count.
	if newCascade {
		s.counter = s.reload
		s.timestampStarted = t.scheduler.Now()
		return
	}

	divisor := uint64(prescalers[newControl&controlPrescalerMask])
	offset := int64(t.scheduler.Now() % divisor)
	stoppedCounter := s.counter

	if cpuWrite {
		// Hardware edge case: when the stopped counter is already FFFF and the
		// enable event lands exactly on a prescaler tick, that tick can occur
		// during the reload-load cycle before the new reload value is latched.
		if stoppedCounter == 0xffff && offset == 0 {
			t.startChannel(index, 0)
			return
		}
		s.counter = s.reload
		t.startChannel(index, offset-1)
		return
	}

	// Direct/debug writes do not model the CPU bus delay. Keep their historical
	// immediate-start behavior while still using global prescaler alignment.
	s.counter = s.reload
	t.startChannel(index, offset)
}

func (t *Timers) startChannel(index int, cycleOffset int64) {
	s := &t.timer[index]
	divisor := int64(prescalers[s.control&controlPrescalerMask])
	cycles := int64(0x10000-uint32(s.counter))*divisor - cycleOffset
	if cycles <= 0 {
		panic("gba timer: non-positive overflow delay")
	}

	now := t.scheduler.Now()
	if cycleOffset >= 0 {
		s.timestampStarted = now - uint64(cycleOffset)
	} else {
		s.timestampStarted = now + uint64(-cycleOffset)
	}

	s.overflowEvent = t.scheduler.Schedule(uint64(cycles), gbascheduler.PriorityEarly, func() {
		t.onOverflow(index)
	})
}

func (t *Timers) cancelOverflow(index int) {
	s := &t.timer[index]
	if s.overflowEvent != 0 {
		t.scheduler.Cancel(s.overflowEvent)
		s.overflowEvent = 0
	}
}

func (t *Timers) onOverflow(index int) {
	s := &t.timer[index]
	s.overflowEvent = 0
	if s.control&controlEnable == 0 || (index > 0 && s.control&controlCountUp != 0) {
		return
	}

	s.counter = s.reload
	s.timestampStarted = t.scheduler.Now()
	s.lastTickAt = t.scheduler.Now()
	s.lastOverflowAt = t.scheduler.Now()
	if t.hooks.Overflow != nil {
		t.hooks.Overflow(index, 1)
	}
	if s.control&controlIRQ != 0 && t.irq != nil {
		t.irq.Request(irqSources[index])
	}
	t.cascade(index + 1)
	t.startChannel(index, 0)
}

func (t *Timers) cascade(index int) {
	if index >= len(t.timer) {
		return
	}
	s := &t.timer[index]
	if s.control&controlEnable == 0 || s.control&controlCountUp == 0 {
		return
	}

	now := t.scheduler.Now()
	s.lastTickAt = now
	if s.counter != 0xffff {
		s.counter++
		return
	}

	s.counter = s.reload
	s.lastOverflowAt = now
	if t.hooks.Overflow != nil {
		t.hooks.Overflow(index, 1)
	}
	if s.control&controlIRQ != 0 && t.irq != nil {
		t.irq.Request(irqSources[index])
	}
	t.cascade(index + 1)
}

func (t *Timers) syncCounter(index int) {
	s := &t.timer[index]
	if s.control&controlEnable == 0 || (index > 0 && s.control&controlCountUp != 0) {
		return
	}

	now := t.scheduler.Now()
	if now <= s.timestampStarted {
		return
	}

	divisor := uint64(prescalers[s.control&controlPrescalerMask])
	ticks := (now - s.timestampStarted) / divisor
	if ticks == 0 {
		return
	}

	s.counter += uint16(ticks)
	s.timestampStarted += ticks * divisor
	s.lastTickAt = s.timestampStarted
}

// NeedsAdvance reports whether timer hardware has live state or queued events.
// It is retained for callers/tests; scheduler time itself must advance even
// when this returns false so later timer starts observe the global clock phase.
func (t *Timers) NeedsAdvance() bool {
	if t.active != 0 {
		return true
	}
	_, ok := t.scheduler.Next()
	return ok
}

// CyclesUntilEvent returns the distance to the next timestamped timer event.
func (t *Timers) CyclesUntilEvent() uint32 {
	at, ok := t.scheduler.Next()
	if !ok {
		return math.MaxUint32
	}
	now := t.scheduler.Now()
	if at <= now {
		return 0
	}
	delta := at - now
	if delta > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(delta)
}

// Advance advances the timer master timestamp. Live counters are derived lazily
// from this timestamp; only scheduled overflow/register events need dispatch.
func (t *Timers) Advance(cycles uint32) {
	t.scheduler.Advance(uint64(cycles))
}

// finalTickOverflows and advanceCounter remain as compact arithmetic helpers
// used by regression tests and by callers that need batched timer math.
func finalTickOverflows(counter, reload uint16, ticks uint64) bool {
	if ticks == 0 {
		return false
	}
	untilOverflow := uint64(0x10000 - uint32(counter))
	if ticks < untilOverflow {
		return false
	}
	remaining := ticks - untilOverflow
	period := uint64(0x10000 - uint32(reload))
	return remaining%period == 0
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

// Reset clears timer reload/counter/control state.
func (t *Timers) Reset() {
	for index := range t.timer {
		t.cancelOverflow(index)
	}
	t.timer = [4]state{}
	t.active = 0
	t.deferWrites = false
}

// Active reports whether at least one timer is enabled.
func (t *Timers) Active() bool { return t.active != 0 }

// Counter returns the current live/frozen counter for tests/debugging.
func (t *Timers) Counter(index int) uint16 {
	t.syncCounter(index)
	return t.timer[index].counter
}

// CounterForCPURead returns the timer value at GomeBoy's CPU data-sampling
// phase. The CPU core has already consumed the current instruction-fetch phase
// before issuing this data callback, so an ordinary timer/cascade tick on that
// exact timestamp is sampled one phase earlier. An overflow event is different:
// overflow/reload has already won the same-cycle scheduler priority and is
// visible to an ordinary timer read.
func (t *Timers) CounterForCPURead(index int) uint16 {
	t.syncCounter(index)
	s := &t.timer[index]
	if s.control&controlEnable == 0 {
		return s.counter
	}

	now := t.scheduler.Now()
	if index > 0 && s.control&controlCountUp != 0 {
		if s.lastTickAt != now {
			return s.counter
		}
		if s.lastOverflowAt == now {
			return 0xffff
		}
		return s.counter - 1
	}

	if s.lastTickAt == now && s.lastOverflowAt != now {
		return s.counter - 1
	}
	return s.counter
}

// Reload returns the programmed reload latch.
func (t *Timers) Reload(index int) uint16 { return t.timer[index].reload }

// Control returns the masked TMxCNT_H value.
func (t *Timers) Control(index int) uint16 { return t.timer[index].control }

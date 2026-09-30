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
// One callback may represent multiple overflows when Advance is called with a
// large cycle interval.
type Hooks struct {
	Overflow func(timer int, count uint32)
}

type state struct {
	reload            uint16
	counter           uint16
	control           uint16
	phase             uint32
	timestampStarted  uint64
	lastTickOverflow  bool
	overflowEvent     gbascheduler.Handle
}

type pendingWriteKind uint8

const (
	pendingReload pendingWriteKind = iota
	pendingControl
)

type pendingWrite struct {
	index int
	kind  pendingWriteKind
	value uint16
}

// Timers owns TM0-TM3 register state and deterministic cycle advancement.
type Timers struct {
	irq   IRQSink
	hooks Hooks
	scheduler *gbascheduler.Scheduler

	timer  [4]state
	active uint8

	deferWrites      bool
	pendingWrites    []pendingWrite
	pendingBusWrites []pendingWrite
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
	if scheduler == nil { panic("gba timer: nil scheduler") }
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
			func() uint16 { return t.timer[index].counter },
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

// BeginWriteAccess marks timer writes performed by a CPU instruction. Timer
// starts and ordinary control changes remain deferred to EndWriteAccess, while
// disabling a running timer commits at the end of the individual I/O bus
// access via EndWriteBusAccess.
func (t *Timers) BeginWriteAccess() {
	if t.deferWrites {
		return
	}
	t.deferWrites = true
}

// EndWriteBusAccess commits timer side effects that become visible only after
// the current CPU I/O transfer has consumed its bus cycles. In particular, a
// TMxCNT_H disable keeps the timer running through the store itself but is
// visible before the next CPU phase.
func (t *Timers) EndWriteBusAccess() {
	// CPU timer writes are timestamped scheduler events. The bus access itself
	// advances the scheduler and commits any +1-cycle write at the exact edge.
}

// EndWriteAccess commits control changes whose hardware effect is delayed until
// the CPU instruction boundary. Direct bus/debug writes remain immediate
// because they do not call BeginWriteAccess.
func (t *Timers) EndWriteAccess() {
	if !t.deferWrites {
		return
	}
	t.deferWrites = false
}

func (t *Timers) writeReload(index int, value uint16) {
	if t.deferWrites {
		// Timer register writes become visible one master-clock cycle after the
		// CPU write. Reload commits before control when both halves of TMxCNT are
		// written together, but after an overflow on the same timestamp.
		t.scheduler.Schedule(1, gbascheduler.PriorityNormal, func() {
			t.timer[index].reload = value
		})
		return
	}
	t.timer[index].reload = value
}

func (t *Timers) reloadForWrite(index int) uint16 {
	return t.timer[index].reload
}

func controlMask(index int) uint16 {
	if index == 0 {
		return controlPrescalerMask | controlIRQ | controlEnable
	}
	return controlPrescalerMask | controlCountUp | controlIRQ | controlEnable
}

func (t *Timers) writeControl(index int, value uint16) {
	if t.deferWrites {
		// Control takes effect one cycle after the write and after a same-cycle
		// reload event. Late priority also keeps overflow edges ahead of control
		// changes at identical timestamps.
		t.scheduler.Schedule(1, gbascheduler.PriorityLate, func() {
			t.applyControlWithStartDelay(index, value, 1)
		})
		return
	}
	t.applyControl(index, value)
}

func (t *Timers) applyControl(index int, value uint16) {
	t.applyControlWithStartDelay(index, value, 0)
}

func (t *Timers) applyControlWithStartDelay(index int, value uint16, startDelay uint64) {
	s := &t.timer[index]
	oldControl := s.control
	newControl := value & controlMask(index)
	oldEnabled := oldControl&controlEnable != 0
	newEnabled := newControl&controlEnable != 0

	// Materialize time under the old mode before changing enable, cascade, or
	// prescaler bits. Doing this after assigning the new control loses the final
	// running tick on disable and applies divisor changes retroactively.
	if oldEnabled && oldControl != newControl {
		t.syncCounter(index)
		t.cancelOverflow(index)
	}

	s.control = newControl
	bit := uint8(1 << index)
	if newEnabled {
		t.active |= bit
	} else {
		t.active &^= bit
	}

	if !oldEnabled && newEnabled {
		s.counter = s.reload
		s.phase = 0
		if index > 0 && s.control&controlCountUp != 0 {
			s.timestampStarted = t.scheduler.Now()
		} else {
			// A CPU enable write first loads the reload latch into the counter.
			// Independently clocked timers begin counting on the following
			// master cycle; direct/debug writes keep their existing immediate
			// semantics by passing a zero startDelay.
			s.timestampStarted = t.scheduler.Now() + startDelay
		}
		t.scheduleOverflow(index)
		return
	}
	if oldEnabled && !newEnabled {
		s.phase = 0
		return
	}

	// Keep the accumulated prescaler position valid when software changes the
	// divisor while the timer remains enabled.
	if newEnabled && oldControl != newControl && s.control&controlCountUp == 0 {
		divisor := prescalers[s.control&controlPrescalerMask]
		if divisor != 0 {
			s.phase %= divisor
		}
		s.timestampStarted = t.scheduler.Now()
		t.scheduleOverflow(index)
	}
}

func (t *Timers) cancelOverflow(index int) {
	s := &t.timer[index]
	if s.overflowEvent != 0 {
		t.scheduler.Cancel(s.overflowEvent)
		s.overflowEvent = 0
	}
}

func (t *Timers) scheduleOverflow(index int) {
	s := &t.timer[index]
	if s.control&controlEnable == 0 || (index > 0 && s.control&controlCountUp != 0) { return }
	divisor := uint64(prescalers[s.control&controlPrescalerMask])
	ticks := uint64(0x10000 - uint32(s.counter))
	delay := ticks*divisor - uint64(s.phase)
	if s.timestampStarted > t.scheduler.Now() {
		delay += s.timestampStarted - t.scheduler.Now()
	}
	s.overflowEvent = t.scheduler.Schedule(delay, gbascheduler.PriorityEarly, func() { t.onOverflow(index) })
}

func (t *Timers) onOverflow(index int) {
	s := &t.timer[index]
	s.overflowEvent = 0
	if s.control&controlEnable == 0 { return }
	s.counter = s.reload
	s.phase = 0
	s.timestampStarted = t.scheduler.Now()
	s.lastTickOverflow = true
	if t.hooks.Overflow != nil { t.hooks.Overflow(index, 1) }
	if s.control&controlIRQ != 0 && t.irq != nil { t.irq.Request(irqSources[index]) }
	t.cascade(index + 1)
	t.scheduleOverflow(index)
}

func (t *Timers) cascade(index int) {
	if index >= len(t.timer) { return }
	s := &t.timer[index]
	if s.control&controlEnable == 0 || s.control&controlCountUp == 0 { return }
	if s.counter != 0xffff { s.counter++; s.lastTickOverflow = false; return }
	s.counter = s.reload
	s.lastTickOverflow = true
	if t.hooks.Overflow != nil { t.hooks.Overflow(index, 1) }
	if s.control&controlIRQ != 0 && t.irq != nil { t.irq.Request(irqSources[index]) }
	t.cascade(index + 1)
}

func (t *Timers) syncCounter(index int) {
	s := &t.timer[index]
	if s.control&controlEnable == 0 || (index > 0 && s.control&controlCountUp != 0) {
		return
	}
	now := t.scheduler.Now()
	if now <= s.timestampStarted {
		if now < s.timestampStarted {
			s.lastTickOverflow = false
		}
		return
	}
	elapsed := now - s.timestampStarted
	// lastTickOverflow describes the exact master-clock edge on which an
	// overflow occurred. Once scheduler time moves beyond that edge, CPU reads
	// must no longer expose the pre-overflow FFFF value.
	if elapsed != 0 {
		s.lastTickOverflow = false
	}
	divisor := uint64(prescalers[s.control&controlPrescalerMask])
	total := uint64(s.phase) + elapsed
	ticks := total / divisor
	s.phase = uint32(total % divisor)
	if ticks != 0 {
		s.counter += uint16(ticks)
	}
	s.timestampStarted = now
}

// NeedsAdvance reports whether timer-local master time must be advanced.
// Pending register-write events count even when no timer is enabled yet.
func (t *Timers) NeedsAdvance() bool {
	if t.active != 0 {
		return true
	}
	_, ok := t.scheduler.Next()
	return ok
}

// CyclesUntilEvent returns the distance to the next timestamped timer event.
// Overflow and delayed register writes share the same scheduler, so this is
// authoritative for both enabled timers and writes that will enable one.
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

// Advance advances all enabled timers by GBA master-clock cycles.
//
// Normal timers derive ticks from their prescaler. Count-up timers 1-3 ignore
// the prescaler and receive one tick per overflow of the previous timer.
func (t *Timers) Advance(cycles uint32) {
	if cycles == 0 { return }
	// The event scheduler is now authoritative for timer time. Overflow events
	// split long advances at their exact master-clock timestamp.
	t.scheduler.Advance(uint64(cycles))
	for index := range t.timer { t.syncCounter(index) }
	return

	/* legacy chunk progression retained temporarily during migration
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
			s.lastTickOverflow = false
			continue
		}

		s.lastTickOverflow = finalTickOverflows(s.counter, s.reload, ticks)
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
	*/
}

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

// Reset clears timer reload/counter/control/prescaler state.
func (t *Timers) Reset() {
	t.timer = [4]state{}
	t.active = 0
	t.deferWrites = false
	t.pendingWrites = t.pendingWrites[:0]
	t.pendingBusWrites = t.pendingBusWrites[:0]
}

// Active reports whether at least one timer is enabled.
func (t *Timers) Active() bool { return t.active != 0 }

// Counter returns the current live/frozen counter for tests/debugging.
func (t *Timers) Counter(index int) uint16 { t.syncCounter(index); return t.timer[index].counter }

// CounterForCPURead returns the timer value visible to a CPU data read at the
// current bus edge. An ordinary timer increment on that same edge is observed
// after the read sample, while an overflow/reload edge is already visible.
func (t *Timers) CounterForCPURead(index int) uint16 {
	t.syncCounter(index)
	s := &t.timer[index]
	if s.control&controlEnable == 0 {
		return s.counter
	}
	if index > 0 && s.control&controlCountUp != 0 {
		// A cascade tick is sourced by the previous timer's overflow at the same
		// master-clock edge. CPU data reads sample before that count-up tick, just
		// like they sample before an ordinary prescaler tick.
		parent := &t.timer[index-1]
		if !parent.lastTickOverflow {
			return s.counter
		}
		if s.lastTickOverflow {
			return 0xffff
		}
		return s.counter - 1
	}
	divisor := prescalers[s.control&controlPrescalerMask]
	if divisor == 0 || s.phase != 0 {
		return s.counter
	}
	if s.lastTickOverflow {
		// Overflow events have earlier same-timestamp priority than CPU-visible
		// timer reads, so the reloaded counter is observable on this edge.
		return s.counter
	}
	return s.counter - 1
}

// Reload returns the programmed reload latch.
func (t *Timers) Reload(index int) uint16 { return t.timer[index].reload }

// Control returns the masked TMxCNT_H value.
func (t *Timers) Control(index int) uint16 { return t.timer[index].control }

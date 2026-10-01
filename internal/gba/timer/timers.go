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
	reload         uint16
	counter        uint16
	control        uint16
	lastEvent      uint64
	lastTickAt     uint64
	lastOverflowAt uint64
	overflowEvent  gbascheduler.Handle
}

type pendingWrite struct {
	index int
	value uint16
}

// Timers owns TM0-TM3 register state and deterministic cycle advancement.
type Timers struct {
	irq       IRQSink
	hooks     Hooks
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

// BeginWriteAccess marks timer writes performed by a CPU instruction. Reload
// latches are bus-phase visible, while control transitions commit at the same
// CPU boundaries used by the established timer model.
func (t *Timers) BeginWriteAccess() {
	if t.deferWrites {
		return
	}
	t.deferWrites = true
}

// EndWriteBusAccess commits a running timer disable at the end of the I/O bus
// transfer. The commit is a same-timestamp scheduler event so timer overflow
// and control edges have deterministic ordering without inventing an extra
// master cycle.
func (t *Timers) EndWriteBusAccess() {
	if len(t.pendingBusWrites) == 0 {
		return
	}
	pending := t.pendingBusWrites
	t.pendingBusWrites = t.pendingBusWrites[:0]
	for _, write := range pending {
		t.commitControl(write.index, write.value)
	}
}

// EndWriteAccess commits starts and other control changes at the instruction
// boundary. This preserves the CPU-visible timing that already scores best on
// the pinned suite while still representing the effect as a scheduler event.
func (t *Timers) EndWriteAccess() {
	if !t.deferWrites {
		return
	}
	t.EndWriteBusAccess()
	t.deferWrites = false
	pending := t.pendingWrites
	t.pendingWrites = t.pendingWrites[:0]
	for _, write := range pending {
		t.commitControl(write.index, write.value)
	}
}

func (t *Timers) commitControl(index int, value uint16) {
	t.scheduler.Schedule(0, gbascheduler.PriorityLate, func() {
		t.applyControl(index, value)
	})
	// Dispatch this boundary event without moving master time. Any earlier
	// priority hardware edge already due at this timestamp wins first.
	t.scheduler.Advance(0)
}

func (t *Timers) writeReload(index int, value uint16) {
	// TMxCNT_L is a reload latch, not a start/stop control. CPU writes are
	// visible in the I/O bus phase, including to an overflow on that transfer.
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
	value &= controlMask(index)
	if t.deferWrites {
		// Disables remain active through the store's bus cycles and become
		// visible at transfer completion. Starts/config changes commit at the
		// instruction boundary, matching the established CPU timer phase.
		if t.timer[index].control&controlEnable != 0 && value&controlEnable == 0 {
			t.pendingBusWrites = append(t.pendingBusWrites, pendingWrite{index: index, value: value})
			return
		}
		t.pendingWrites = append(t.pendingWrites, pendingWrite{index: index, value: value})
		return
	}
	t.applyControl(index, value)
}

// applyControl first materializes the old timer at the current master timestamp,
// then applies the new mode. Independently-clocked timers are anchored to the
// free-running global prescaler grid, following the event/deadline model used
// by mature GBA cores rather than a timer-local Advance() phase.
func (t *Timers) applyControl(index int, value uint16) {
	s := &t.timer[index]
	oldControl := s.control
	newControl := value & controlMask(index)
	oldEnabled := oldControl&controlEnable != 0
	newEnabled := newControl&controlEnable != 0
	oldCascade := index > 0 && oldControl&controlCountUp != 0
	newCascade := index > 0 && newControl&controlCountUp != 0

	if oldEnabled && !oldCascade {
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

	if !newEnabled {
		return
	}

	if !oldEnabled {
		s.counter = s.reload
	}

	if newCascade {
		// A newly enabled count-up timer loads reload. A mode change while
		// already enabled preserves the materialized live counter.
		s.lastEvent = t.scheduler.Now()
		return
	}

	// Re-anchor only when a start or clock-source configuration changes.
	// IRQ-only rewrites keep the existing counter/deadline phase.
	oldClock := oldControl & (controlPrescalerMask | controlCountUp)
	newClock := newControl & (controlPrescalerMask | controlCountUp)
	if !oldEnabled || oldCascade || oldClock != newClock {
		t.anchorChannel(index)
	} else {
		t.scheduleOverflow(index)
	}
}

func (t *Timers) anchorChannel(index int) {
	s := &t.timer[index]
	// Timer enable resets the local prescaler phase. All later counter
	// materialization and overflow deadlines are measured from this timestamp,
	// so the event model is independent of Advance() chunking.
	s.lastEvent = t.scheduler.Now()
	t.scheduleOverflow(index)
}

func (t *Timers) scheduleOverflow(index int) {
	s := &t.timer[index]
	if s.control&controlEnable == 0 || (index > 0 && s.control&controlCountUp != 0) {
		return
	}
	divisor := uint64(prescalers[s.control&controlPrescalerMask])
	deadline := s.lastEvent + uint64(0x10000-uint32(s.counter))*divisor
	now := t.scheduler.Now()
	if deadline <= now {
		// A live timer's due overflow should normally have been dispatched by
		// Scheduler.AdvanceTo. Keep restore/control paths deterministic if a
		// snapshot lands exactly on the boundary.
		deadline = now + divisor
	}
	s.overflowEvent = t.scheduler.Schedule(deadline-now, gbascheduler.PriorityEarly, func() {
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
	s.lastEvent = t.scheduler.Now()
	s.lastTickAt = t.scheduler.Now()
	s.lastOverflowAt = t.scheduler.Now()
	if t.hooks.Overflow != nil {
		t.hooks.Overflow(index, 1)
	}
	if s.control&controlIRQ != 0 && t.irq != nil {
		t.irq.Request(irqSources[index])
	}
	t.cascade(index + 1)
	t.scheduleOverflow(index)
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
	if now <= s.lastEvent {
		return
	}
	divisor := uint64(prescalers[s.control&controlPrescalerMask])
	ticks := (now - s.lastEvent) / divisor
	if ticks == 0 {
		return
	}

	// Overflow deadlines are explicit scheduler events, so a normal sync never
	// crosses a wrap. Keep the arithmetic bounded defensively for restored state.
	untilOverflow := uint64(0x10000 - uint32(s.counter))
	if ticks >= untilOverflow {
		ticks = untilOverflow - 1
	}
	if ticks == 0 {
		return
	}
	s.counter += uint16(ticks)
	s.lastEvent += ticks * divisor
	s.lastTickAt = s.lastEvent
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
	t.pendingWrites = t.pendingWrites[:0]
	t.pendingBusWrites = t.pendingBusWrites[:0]
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

	if s.lastTickAt == now {
		if s.lastOverflowAt == now {
			// The overflow/reload event has updated internal state, but a CPU data
			// read sampling this exact master-clock edge still sees the pre-edge
			// counter value. This is the same rule used for cascade overflow reads.
			return 0xffff
		}
		return s.counter - 1
	}
	return s.counter
}

// Reload returns the programmed reload latch.
func (t *Timers) Reload(index int) uint16 { return t.timer[index].reload }

// Control returns the masked TMxCNT_H value.
func (t *Timers) Control(index int) uint16 { return t.timer[index].control }

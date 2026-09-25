package timer

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

func newTestTimers(t *testing.T, hooks Hooks) (*Timers, *bus.Bus, *gbairq.Controller) {
	t.Helper()
	b := bus.New(nil, nil)
	irq := gbairq.New(b, nil)
	timers := New(b, irq, hooks)
	return timers, b, irq
}

func timerLow(index int) uint32 {
	return bus.IOStart + firstTimerOffset + uint32(index*timerStride)
}

func timerHigh(index int) uint32 {
	return timerLow(index) + 2
}

func TestTimerRegistersAndControlMasks(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})

	b.Write16(timerLow(0), 0x1234, bus.Access{})
	if got := timers.Reload(0); got != 0x1234 {
		t.Fatalf("TM0 reload = %04x, want 1234", got)
	}
	if got, _ := b.Read16(timerLow(0), bus.Access{}); got != 0 {
		t.Fatalf("stopped TM0 counter = %04x, want reset counter 0000", got)
	}

	b.Write16(timerHigh(0), 0xffff, bus.Access{})
	if got := timers.Control(0); got != 0x00c3 {
		t.Fatalf("TM0 control = %04x, want c3 with count-up bit ignored", got)
	}
	if got := timers.Counter(0); got != 0x1234 {
		t.Fatalf("TM0 start counter = %04x, want reload 1234", got)
	}

	b.Write16(timerHigh(1), 0xffff, bus.Access{})
	if got := timers.Control(1); got != 0x00c7 {
		t.Fatalf("TM1 control = %04x, want c7", got)
	}
}

func TestTimerStartLoadsReloadAndStoppedTimerFreezes(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})
	b.Write16(timerLow(0), 0xfff0, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})

	timers.Advance(5)
	if got := timers.Counter(0); got != 0xfff5 {
		t.Fatalf("running counter = %04x, want fff5", got)
	}

	// Reload writes while running do not change the live counter.
	b.Write16(timerLow(0), 0x8000, bus.Access{})
	if got := timers.Counter(0); got != 0xfff5 {
		t.Fatalf("reload write changed live counter = %04x", got)
	}

	b.Write16(timerHigh(0), 0, bus.Access{})
	timers.Advance(1000)
	if got := timers.Counter(0); got != 0xfff5 {
		t.Fatalf("stopped counter advanced = %04x", got)
	}

	// A new 0->1 start edge copies the new reload latch.
	b.Write16(timerHigh(0), controlEnable, bus.Access{})
	if got := timers.Counter(0); got != 0x8000 {
		t.Fatalf("restart counter = %04x, want new reload 8000", got)
	}
}

func TestTimerReloadByteWritesMergeAgainstReloadNotCounter(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})
	b.Write16(timerLow(0), 0x1234, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})
	timers.Advance(5) // counter=1239, reload still 1234

	b.Write8(timerLow(0), 0xaa, bus.Access{})
	if got := timers.Reload(0); got != 0x12aa {
		t.Fatalf("low-byte reload = %04x, want 12aa", got)
	}
	b.Write8(timerLow(0)+1, 0xbb, bus.Access{})
	if got := timers.Reload(0); got != 0xbbaa {
		t.Fatalf("high-byte reload = %04x, want bbaa", got)
	}
	if got := timers.Counter(0); got != 0x1239 {
		t.Fatalf("reload byte writes changed live counter = %04x", got)
	}
}

func TestTimer32BitWriteUsesNewReloadOnStart(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})

	value := uint32(controlEnable)<<16 | 0xff00
	b.Write32(timerLow(0), value, bus.Access{})
	if got := timers.Reload(0); got != 0xff00 {
		t.Fatalf("32-bit reload = %04x", got)
	}
	if got := timers.Counter(0); got != 0xff00 {
		t.Fatalf("32-bit start counter = %04x, want ff00", got)
	}
}

func TestTimerPrescalers(t *testing.T) {
	cases := []struct {
		name    string
		selectv uint16
		divisor uint32
	}{
		{"F1", 0, 1},
		{"F64", 1, 64},
		{"F256", 2, 256},
		{"F1024", 3, 1024},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			timers, b, _ := newTestTimers(t, Hooks{})
			b.Write16(timerLow(0), 0, bus.Access{})
			b.Write16(timerHigh(0), controlEnable|tc.selectv, bus.Access{})

			if tc.divisor > 1 {
				timers.Advance(tc.divisor - 1)
				if got := timers.Counter(0); got != 0 {
					t.Fatalf("counter before divisor = %04x, want 0000", got)
				}
			}
			timers.Advance(1)
			if got := timers.Counter(0); got != 1 {
				t.Fatalf("counter at divisor = %04x, want 0001", got)
			}

			timers.Advance(tc.divisor * 3)
			if got := timers.Counter(0); got != 4 {
				t.Fatalf("counter after four ticks = %04x, want 0004", got)
			}
		})
	}
}

func TestTimerOverflowReloadsAndCanBatchMultipleOverflows(t *testing.T) {
	var hookTimer int
	var hookCount uint32
	timers, b, _ := newTestTimers(t, Hooks{
		Overflow: func(index int, count uint32) {
			hookTimer = index
			hookCount += count
		},
	})

	b.Write16(timerLow(0), 0xfffc, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})

	timers.Advance(10)
	// Period is four cycles: overflow at cycles 4 and 8, then two ticks after
	// the second reload.
	if got := timers.Counter(0); got != 0xfffe {
		t.Fatalf("counter after batched overflows = %04x, want fffe", got)
	}
	if hookTimer != 0 || hookCount != 2 {
		t.Fatalf("overflow hook timer=%d count=%d, want timer0/count2", hookTimer, hookCount)
	}
}

func TestTimerCascadeCountsPreviousOverflows(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})

	// Timer0 overflows every two cycles.
	b.Write16(timerLow(0), 0xfffe, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})

	// Timer1 increments once per Timer0 overflow and itself overflows every
	// three incoming ticks.
	b.Write16(timerLow(1), 0xfffd, bus.Access{})
	b.Write16(timerHigh(1), controlEnable|controlCountUp, bus.Access{})

	timers.Advance(12) // six TM0 overflows -> two TM1 overflows
	if got := timers.Counter(0); got != 0xfffe {
		t.Fatalf("TM0 counter = %04x, want fffe", got)
	}
	if got := timers.Counter(1); got != 0xfffd {
		t.Fatalf("TM1 counter = %04x, want reload fffd after two overflows", got)
	}
}

func TestTimerCascadeChainsAcrossAllFourTimers(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})

	for i := 0; i < 4; i++ {
		b.Write16(timerLow(i), 0xffff, bus.Access{})
		control := uint16(controlEnable)
		if i > 0 {
			control |= controlCountUp
		}
		b.Write16(timerHigh(i), control, bus.Access{})
	}

	timers.Advance(1)
	for i := 0; i < 4; i++ {
		if got := timers.Counter(i); got != 0xffff {
			t.Fatalf("TM%d counter=%04x, want ffff after cascaded overflow/reload", i, got)
		}
	}
}

func TestCascadeModeIgnoresPrescaler(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})

	b.Write16(timerLow(0), 0xfffe, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})
	b.Write16(timerLow(1), 0x1000, bus.Access{})
	b.Write16(timerHigh(1), controlEnable|controlCountUp|3, bus.Access{})

	timers.Advance(1)
	if got := timers.Counter(1); got != 0x1000 {
		t.Fatalf("cascade timer advanced without parent overflow: %04x", got)
	}
	timers.Advance(1)
	if got := timers.Counter(1); got != 0x1001 {
		t.Fatalf("cascade timer did not increment on parent overflow: %04x", got)
	}
}

func TestTimerOverflowRequestsInterrupt(t *testing.T) {
	timers, b, irq := newTestTimers(t, Hooks{})

	b.Write16(timerLow(2), 0xffff, bus.Access{})
	b.Write16(timerHigh(2), controlEnable|controlIRQ, bus.Access{})
	timers.Advance(1)

	if got := irq.IF(); got != uint16(gbairq.Timer2) {
		t.Fatalf("IF after TM2 overflow = %04x, want Timer2", got)
	}
}

func TestCascadedTimerOverflowRequestsOwnInterrupt(t *testing.T) {
	timers, b, irq := newTestTimers(t, Hooks{})

	b.Write16(timerLow(0), 0xffff, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})
	b.Write16(timerLow(1), 0xffff, bus.Access{})
	b.Write16(timerHigh(1), controlEnable|controlCountUp|controlIRQ, bus.Access{})

	timers.Advance(1)
	if got := irq.IF(); got != uint16(gbairq.Timer1) {
		t.Fatalf("IF after cascaded TM1 overflow = %04x, want Timer1", got)
	}
}

func TestDisabledTimerIRQBitDoesNotRequestInterrupt(t *testing.T) {
	timers, b, irq := newTestTimers(t, Hooks{})
	b.Write16(timerLow(0), 0xffff, bus.Access{})
	b.Write16(timerHigh(0), controlEnable, bus.Access{})
	timers.Advance(1)
	if got := irq.IF(); got != 0 {
		t.Fatalf("IF = %04x, want no timer IRQ", got)
	}
}

func TestTimerResetClearsState(t *testing.T) {
	timers, b, _ := newTestTimers(t, Hooks{})
	b.Write16(timerLow(0), 0xff00, bus.Access{})
	b.Write16(timerHigh(0), controlEnable|controlIRQ, bus.Access{})
	timers.Advance(3)

	timers.Reset()
	for i := 0; i < 4; i++ {
		if timers.Counter(i) != 0 || timers.Reload(i) != 0 || timers.Control(i) != 0 {
			t.Fatalf("TM%d reset state counter=%04x reload=%04x control=%04x",
				i, timers.Counter(i), timers.Reload(i), timers.Control(i))
		}
	}
}

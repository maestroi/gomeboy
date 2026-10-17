package timer

import (
	"reflect"
	"testing"

	gbascheduler "github.com/maestroi/gomeboy/internal/gba/scheduler"
)

func TestStateRoundTripActiveTimersAndDeferredWrites(t *testing.T) {
	s := gbascheduler.New()
	s.Advance(100)
	timers := &Timers{
		scheduler:   s,
		active:      0x0b,
		deferWrites: true,
		pendingWrites: []pendingWrite{
			{index: 0, value: 0x00c0},
			{index: 1, value: 0x00c4},
		},
		pendingBusWrites: []pendingWrite{
			{index: 3, value: 0},
		},
	}
	timers.timer[0] = state{
		reload: 0xff00, counter: 0xff80, control: 0x00c0,
		lastEvent: 96, lastTickAt: 99, lastOverflowAt: 64,
	}
	timers.timer[1] = state{
		reload: 0x2200, counter: 0x2233, control: 0x00c4,
		lastEvent: 100,
	}

	want := timers.Snapshot()
	timers.timer = [4]state{}
	timers.active = 0
	timers.deferWrites = false
	timers.pendingWrites = nil
	timers.pendingBusWrites = nil
	timers.Restore(want)

	if got := timers.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("timer state after restore = %#v, want %#v", got, want)
	}
}

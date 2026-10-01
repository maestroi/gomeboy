package timer

import (
	"reflect"
	"testing"
)

func TestStateRoundTripActiveTimersAndDeferredWrites(t *testing.T) {
	timers := &Timers{
		active:      0x0b,
		deferWrites: true,
		pendingWrites: []pendingWrite{
			{index: 0, kind: pendingReload, value: 0xff00},
			{index: 1, kind: pendingControl, value: 0x00c4},
		},
		pendingBusWrites: []pendingWrite{
			{index: 3, kind: pendingReload, value: 0x1234},
		},
	}
	timers.timer[0] = state{reload: 0xff00, counter: 0xff80, control: 0x00c0, phase: 3, lastTickOverflow: true}
	timers.timer[1] = state{reload: 0x2200, counter: 0x2233, control: 0x00c4, phase: 1}

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

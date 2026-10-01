package dma

import (
	"reflect"
	"testing"
)

func TestStateRoundTripActiveTransfer(t *testing.T) {
	d := &DMA{
		pending:   0x06,
		active:    0x04,
		servicing: true,
	}
	d.ch[2] = channel{
		sourceInitial: 0x02000100, destInitial: 0x03000200, countInitial: 16, control: 0xa640,
		sourceCurrent: 0x02000110, destCurrent: 0x03000210, countCurrent: 12,
		destReload: 0x03000200, countReload: 16, lastCycles: 7, lastUnits: 2,
		dataLatch: 0x12345678, disableAfterRun: true, transferActive: true,
		completionPending: true, transferRemaining: 12, transferUnits: 4,
		transferWidth: 4, transferSourceMode: 0x80, transferDestMode: 0x20,
		transferStartSource: 0x02000100, transferStartDest: 0x03000200,
		transferCycles: 11,
	}

	want := d.Snapshot()
	d.pending = 0
	d.active = 0
	d.servicing = false
	d.ch[2] = channel{}
	d.Restore(want)

	if got := d.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DMA state after restore = %#v, want %#v", got, want)
	}
}

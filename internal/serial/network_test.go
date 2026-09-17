package serial

import (
	"net"
	"testing"
	"time"
)

func TestNetworkDeviceDirectBitExchange(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	left := NewDirectNetworkDevice(leftConn, time.Second)
	right := NewDirectNetworkDevice(rightConn, time.Second)
	defer left.Close()
	defer right.Close()

	result := make(chan struct {
		bit bool
		err error
	}, 1)
	go func() {
		bit, err := left.ExchangeBit(true)
		result <- struct {
			bit bool
			err error
		}{bit: bit, err: err}
	}()

	deadline := time.Now().Add(time.Second)
	var pulse ClockPulse
	var ok bool
	for time.Now().Before(deadline) {
		if pulse, ok = right.PollClock(); ok {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ok {
		t.Fatal("right peer did not receive clock pulse")
	}
	if !pulse.Incoming {
		t.Fatal("clock pulse did not contain outgoing bit")
	}
	if err := right.ReplyClock(pulse, false); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.bit {
			t.Fatal("left peer got true, want false")
		}
	case <-time.After(time.Second):
		t.Fatal("bit exchange timed out")
	}
}

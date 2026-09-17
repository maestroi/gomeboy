package linkcable

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (string, context.CancelFunc) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := NewServer()
	go func() {
		_ = s.Serve(ctx, ln)
	}()
	return ln.Addr().String(), func() { cancel(); _ = ln.Close() }
}

func TestBrokerPairsTwoEmulators(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	a, err := Dial(ctx, addr, "room", "red")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Dial(ctx, addr, "room", "blue")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	type got struct {
		b   byte
		err error
	}
	chA := make(chan got, 1)
	chB := make(chan got, 1)
	go func() { x, e := a.Exchange(ctx, 0x12, true); chA <- got{x, e} }()
	go func() { x, e := b.Exchange(ctx, 0xA5, false); chB <- got{x, e} }()

	ga, gb := <-chA, <-chB
	if ga.err != nil || gb.err != nil {
		t.Fatalf("errors: %v %v", ga.err, gb.err)
	}
	if ga.b != 0xA5 || gb.b != 0x12 {
		t.Fatalf("got %#x %#x", ga.b, gb.b)
	}
}

func TestBrokerVirtualPeer(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeVirtual(ctx, addr, "room", "mock", func(_ context.Context, in byte) (byte, error) {
			return in ^ 0xFF, nil
		})
	}()
	time.Sleep(10 * time.Millisecond)
	c, err := Dial(ctx, addr, "room", "red")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.Exchange(ctx, 0x0F, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0xF0 {
		t.Fatalf("got %#x", got)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

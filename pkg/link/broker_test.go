package link

import (
	"net"
	"testing"
	"time"
)

func TestBrokerRoutesClockWithinSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	broker := NewBroker()
	serveDone := make(chan error, 1)
	go func() { serveDone <- broker.Serve(ln) }()
	defer func() {
		_ = ln.Close()
		select {
		case err := <-serveDone:
			if err != nil {
				t.Errorf("broker serve: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("broker did not stop")
		}
	}()

	left := dialTestPeer(t, ln.Addr().String(), "session-a", "left")
	defer left.Close()
	right := dialTestPeer(t, ln.Addr().String(), "session-a", "right")
	defer right.Close()

	leftEnc := NewEncoder(left)
	rightDec := NewDecoder(right)
	if err := leftEnc.Encode(Message{Type: MessageClock, Sequence: 7, Bit: true}); err != nil {
		t.Fatal(err)
	}
	msg, err := rightDec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MessageClock || msg.Sequence != 7 || !msg.Bit || msg.PeerID != "left" {
		t.Fatalf("unexpected routed message: %+v", msg)
	}
}

func dialTestPeer(t *testing.T, address, session, peerID string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	enc := NewEncoder(conn)
	dec := NewDecoder(conn)
	if err := enc.Encode(Message{Type: MessageHello, Session: session, PeerID: peerID}); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	msg, err := dec.Decode()
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if msg.Type != MessageReady {
		conn.Close()
		t.Fatalf("expected ready, got %+v", msg)
	}
	return conn
}

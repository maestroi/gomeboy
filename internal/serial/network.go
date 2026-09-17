package serial

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maestroi/gomeboy/internal/link"
)

const defaultNetworkTimeout = 10 * time.Second

type bitResult struct {
	bit bool
	err error
}

// NetworkDevice adapts the generic link protocol to the serial Device API.
// It works over a direct net.Conn or through the multi-session link broker.
type NetworkDevice struct {
	conn    net.Conn
	enc     *link.Encoder
	dec     *link.Decoder
	timeout time.Duration

	writeMu sync.Mutex
	waitMu  sync.Mutex
	waiters map[uint64]chan bitResult
	seq     atomic.Uint64

	clocks   chan ClockPulse
	metadata chan link.Message
	done     chan struct{}
	closeOne sync.Once

	errMu sync.Mutex
	err   error
}

// NewDirectNetworkDevice creates a serial device over an already-established
// point-to-point connection. Both endpoints use the same protocol but no
// broker hello/session handshake is required.
func NewDirectNetworkDevice(conn net.Conn, timeout time.Duration) *NetworkDevice {
	if timeout <= 0 {
		timeout = defaultNetworkTimeout
	}
	d := &NetworkDevice{
		conn:     conn,
		enc:      link.NewEncoder(conn),
		dec:      link.NewDecoder(conn),
		timeout:  timeout,
		waiters:  make(map[uint64]chan bitResult),
		clocks:   make(chan ClockPulse, 64),
		metadata: make(chan link.Message, 16),
		done:     make(chan struct{}),
	}
	go d.readLoop()
	return d
}

// NewBrokerNetworkDevice joins a broker session. Role and metadata are opaque
// to gomeboy and may be used by higher-level systems to identify a virtual
// peer, game, agent, or test fixture.
func NewBrokerNetworkDevice(conn net.Conn, session, peerID, role string, metadata map[string]string, timeout time.Duration) (*NetworkDevice, error) {
	if session == "" || peerID == "" {
		return nil, errors.New("serial: broker session and peer ID are required")
	}
	if timeout <= 0 {
		timeout = defaultNetworkTimeout
	}

	enc := link.NewEncoder(conn)
	dec := link.NewDecoder(conn)
	if err := enc.Encode(link.Message{
		Type:     link.MessageHello,
		Session:  session,
		PeerID:   peerID,
		Role:     role,
		Metadata: metadata,
	}); err != nil {
		return nil, err
	}
	ready, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	if ready.Type == link.MessageError {
		return nil, errors.New(ready.Error)
	}
	if ready.Type != link.MessageReady {
		return nil, fmt.Errorf("serial: expected broker ready, got %q", ready.Type)
	}

	d := &NetworkDevice{
		conn:     conn,
		enc:      enc,
		dec:      dec,
		timeout:  timeout,
		waiters:  make(map[uint64]chan bitResult),
		clocks:   make(chan ClockPulse, 64),
		metadata: make(chan link.Message, 16),
		done:     make(chan struct{}),
	}
	go d.readLoop()
	return d, nil
}

func DialDirect(address string, timeout time.Duration) (*NetworkDevice, error) {
	conn, err := net.DialTimeout("tcp", address, networkTimeout(timeout))
	if err != nil {
		return nil, err
	}
	return NewDirectNetworkDevice(conn, timeout), nil
}

func DialBroker(address, session, peerID, role string, metadata map[string]string, timeout time.Duration) (*NetworkDevice, error) {
	conn, err := net.DialTimeout("tcp", address, networkTimeout(timeout))
	if err != nil {
		return nil, err
	}
	d, err := NewBrokerNetworkDevice(conn, session, peerID, role, metadata, timeout)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return d, nil
}

func networkTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultNetworkTimeout
	}
	return timeout
}

// Send and Receive satisfy the legacy Device interface. Network-aware serial
// controllers use BitExchanger/ExternalClockDevice instead.
func (d *NetworkDevice) Send() bool       { return true }
func (d *NetworkDevice) Receive(bool)     {}
func (d *NetworkDevice) Metadata() <-chan link.Message { return d.metadata }

func (d *NetworkDevice) ExchangeBit(out bool) (bool, error) {
	seq := d.seq.Add(1)
	wait := make(chan bitResult, 1)

	d.waitMu.Lock()
	d.waiters[seq] = wait
	d.waitMu.Unlock()

	if err := d.send(link.Message{Type: link.MessageClock, Sequence: seq, Bit: out}); err != nil {
		d.removeWaiter(seq)
		return false, err
	}

	timer := time.NewTimer(d.timeout)
	defer timer.Stop()
	select {
	case result := <-wait:
		return result.bit, result.err
	case <-timer.C:
		d.removeWaiter(seq)
		return false, fmt.Errorf("serial: link clock %d timed out", seq)
	case <-d.done:
		return false, d.lastError()
	}
}

func (d *NetworkDevice) PollClock() (ClockPulse, bool) {
	select {
	case pulse := <-d.clocks:
		return pulse, true
	default:
		return ClockPulse{}, false
	}
}

func (d *NetworkDevice) ReplyClock(pulse ClockPulse, out bool) error {
	return d.send(link.Message{Type: link.MessageClockReply, Sequence: pulse.Sequence, Bit: out})
}

// PublishMetadata sends optional out-of-band metadata through a broker. The
// broker never interprets these fields; virtual peers and orchestrators may.
func (d *NetworkDevice) PublishMetadata(metadata map[string]string) error {
	return d.send(link.Message{Type: link.MessageMetadata, Metadata: metadata})
}

func (d *NetworkDevice) Close() error {
	d.closeWithError(net.ErrClosed)
	return nil
}

func (d *NetworkDevice) readLoop() {
	for {
		msg, err := d.dec.Decode()
		if err != nil {
			d.closeWithError(err)
			return
		}

		switch msg.Type {
		case link.MessageClock:
			select {
			case d.clocks <- ClockPulse{Sequence: msg.Sequence, Incoming: msg.Bit}:
			case <-d.done:
				return
			}
		case link.MessageClockReply:
			d.resolve(msg.Sequence, bitResult{bit: msg.Bit})
		case link.MessageMetadata:
			select {
			case d.metadata <- msg:
			default:
				// Metadata is observational. Never stall serial traffic because a
				// consumer is not draining it quickly enough.
			}
		case link.MessageError:
			if msg.Sequence != 0 {
				d.resolve(msg.Sequence, bitResult{err: errors.New(msg.Error)})
			}
		}
	}
}

func (d *NetworkDevice) send(msg link.Message) error {
	select {
	case <-d.done:
		return d.lastError()
	default:
	}

	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if err := d.enc.Encode(msg); err != nil {
		d.closeWithError(err)
		return err
	}
	return nil
}

func (d *NetworkDevice) resolve(seq uint64, result bitResult) {
	d.waitMu.Lock()
	wait := d.waiters[seq]
	delete(d.waiters, seq)
	d.waitMu.Unlock()
	if wait != nil {
		wait <- result
	}
}

func (d *NetworkDevice) removeWaiter(seq uint64) {
	d.waitMu.Lock()
	delete(d.waiters, seq)
	d.waitMu.Unlock()
}

func (d *NetworkDevice) closeWithError(err error) {
	d.closeOne.Do(func() {
		d.errMu.Lock()
		d.err = err
		d.errMu.Unlock()
		close(d.done)
		_ = d.conn.Close()

		d.waitMu.Lock()
		for seq, wait := range d.waiters {
			delete(d.waiters, seq)
			wait <- bitResult{err: err}
		}
		d.waitMu.Unlock()
	})
}

func (d *NetworkDevice) lastError() error {
	d.errMu.Lock()
	defer d.errMu.Unlock()
	if d.err == nil {
		return net.ErrClosed
	}
	return d.err
}

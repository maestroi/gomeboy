package linkcable

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder

	mu      sync.Mutex
	nextSeq uint64
	pending map[uint64]chan Result
	closed  chan struct{}
	err     error
}

func Dial(ctx context.Context, address, session, endpoint string) (*Client, error) {
	if address == "" || session == "" || endpoint == "" {
		return nil, errors.New("linkcable: address, session and endpoint are required")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("linkcable: dial %s: %w", address, err)
	}
	c := &Client{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		dec:     json.NewDecoder(bufio.NewReader(conn)),
		pending: make(map[uint64]chan Result),
		closed:  make(chan struct{}),
	}
	if err := c.enc.Encode(message{Type: msgHello, Version: ProtocolVersion, Session: session, Endpoint: endpoint, Role: RoleEmulator}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("linkcable: send hello: %w", err)
	}
	var reply message
	if err := c.dec.Decode(&reply); err != nil {
		conn.Close()
		return nil, fmt.Errorf("linkcable: read welcome: %w", err)
	}
	if reply.Type == msgError {
		conn.Close()
		return nil, errors.New(reply.Error)
	}
	if reply.Type != msgWelcome || reply.Version != ProtocolVersion {
		conn.Close()
		return nil, fmt.Errorf("linkcable: invalid welcome %#v", reply)
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) Exchange(ctx context.Context, out byte, internalClock bool) (byte, error) {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return 0, err
	}
	c.nextSeq++
	seq := c.nextSeq
	ch := make(chan Result, 1)
	c.pending[seq] = ch
	if err := c.enc.Encode(message{Type: msgReady, Seq: seq, Out: out, InternalClock: internalClock}); err != nil {
		delete(c.pending, seq)
		c.mu.Unlock()
		return 0, fmt.Errorf("linkcable: send ready: %w", err)
	}
	c.mu.Unlock()

	select {
	case r := <-ch:
		return r.Byte, r.Err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
		return 0, ctx.Err()
	case <-c.closed:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = net.ErrClosed
		}
		return 0, err
	}
}

func (c *Client) SendMetadata(metadata map[string]json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	if err := c.enc.Encode(message{Type: msgMetadata, Metadata: metadata}); err != nil {
		return fmt.Errorf("linkcable: send metadata: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) readLoop() {
	for {
		var m message
		if err := c.dec.Decode(&m); err != nil {
			c.fail(err)
			return
		}
		switch m.Type {
		case msgResult:
			c.mu.Lock()
			ch := c.pending[m.Seq]
			delete(c.pending, m.Seq)
			c.mu.Unlock()
			if ch != nil {
				ch <- Result{Byte: m.In}
			}
		case msgError:
			c.fail(errors.New(m.Error))
			return
		}
	}
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
		for seq, ch := range c.pending {
			delete(c.pending, seq)
			ch <- Result{Err: err}
		}
		close(c.closed)
	}
	c.mu.Unlock()
}

type VirtualHandler func(context.Context, byte) (byte, error)

func ServeVirtual(ctx context.Context, address, session, endpoint string, handler VirtualHandler) error {
	if handler == nil {
		return errors.New("linkcable: virtual handler is required")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("linkcable: dial %s: %w", address, err)
	}
	defer conn.Close()
	stopClose := make(chan struct{})
	defer close(stopClose)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopClose:
		}
	}()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))
	if err := enc.Encode(message{Type: msgHello, Version: ProtocolVersion, Session: session, Endpoint: endpoint, Role: RoleVirtual}); err != nil {
		return err
	}
	var welcome message
	if err := dec.Decode(&welcome); err != nil {
		return err
	}
	if welcome.Type == msgError {
		return errors.New(welcome.Error)
	}
	if welcome.Type != msgWelcome {
		return fmt.Errorf("linkcable: expected welcome, got %q", welcome.Type)
	}
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if m.Type != msgExchange {
			continue
		}
		in, err := handler(ctx, m.Out)
		if err != nil {
			if e := enc.Encode(message{Type: msgExchangeResult, RequestID: m.RequestID, Error: err.Error()}); e != nil {
				return e
			}
			continue
		}
		if err := enc.Encode(message{Type: msgExchangeResult, RequestID: m.RequestID, In: in}); err != nil {
			return err
		}
	}
}

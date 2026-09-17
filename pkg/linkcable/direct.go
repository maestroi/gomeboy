package linkcable

import (
	"context"
	"fmt"
	"net"
	"sync"
)

const DirectSession = "_direct"

// Broker is a running link-cable broker. It can be used as a standalone
// multi-session broker or embedded in one emulator process for direct P2P.
type Broker struct {
	ln     net.Listener
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
}

// Listen starts a broker on address. Close stops it.
func Listen(address string) (*Broker, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("linkcable: listen on %s: %w", address, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Broker{ln: ln, cancel: cancel, done: make(chan error, 1)}
	go func() {
		b.done <- NewServer().Serve(ctx, ln)
	}()
	return b, nil
}

func (b *Broker) Addr() net.Addr { return b.ln.Addr() }

// DialAddress returns an address suitable for clients in the same process.
// A wildcard listener is rewritten to loopback while retaining its port.
func (b *Broker) DialAddress() string {
	host, port, err := net.SplitHostPort(b.ln.Addr().String())
	if err != nil {
		return b.ln.Addr().String()
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func (b *Broker) Close() error {
	var err error
	b.once.Do(func() {
		b.cancel()
		_ = b.ln.Close()
		err = <-b.done
	})
	return err
}

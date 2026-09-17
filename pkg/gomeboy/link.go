package gomeboy

import (
	"errors"
	"net"
	"time"

	"github.com/maestroi/gomeboy/internal/serial"
	"github.com/maestroi/gomeboy/pkg/link"
)

// NetworkLink is a network-backed Game Boy serial peer. Keep the handle alive
// for as long as it is attached to an Emulator and close it when the session
// ends.
type NetworkLink struct {
	device *serial.NetworkDevice
}

// DialBrokerLink connects to a multi-session gomeboy link broker. Role and
// metadata are opaque to the broker and intended for orchestrators and virtual
// peers (for example role="emulator" or role="virtual").
func DialBrokerLink(address, session, peerID, role string, metadata map[string]string, timeout time.Duration) (*NetworkLink, error) {
	device, err := serial.DialBroker(address, session, peerID, role, metadata, timeout)
	if err != nil {
		return nil, err
	}
	return &NetworkLink{device: device}, nil
}

// DialDirectLink establishes a direct point-to-point TCP link without a
// broker. The listening side can use NewDirectLink with an accepted net.Conn.
func DialDirectLink(address string, timeout time.Duration) (*NetworkLink, error) {
	device, err := serial.DialDirect(address, timeout)
	if err != nil {
		return nil, err
	}
	return &NetworkLink{device: device}, nil
}

// NewDirectLink wraps an already-established point-to-point connection. This
// is useful for the listening side of a direct link or for tests using
// net.Pipe.
func NewDirectLink(conn net.Conn, timeout time.Duration) *NetworkLink {
	return &NetworkLink{device: serial.NewDirectNetworkDevice(conn, timeout)}
}

// AttachLink connects this emulator's serial controller to link. Call it after
// a ROM has been loaded. A reset rebuilds the serial controller, so callers
// that reset an active linked emulator should attach the same link again.
func (e *Emulator) AttachLink(networkLink *NetworkLink) error {
	if networkLink == nil || networkLink.device == nil {
		return errors.New("gomeboy: nil network link")
	}
	if e == nil || e.gb == nil || e.gb.Serial == nil {
		return errors.New("gomeboy: emulator serial controller is not initialized")
	}
	e.gb.Serial.Attach(networkLink.device)
	return nil
}

// ConnectBrokerLink is a convenience helper that dials and immediately
// attaches a brokered link to the emulator.
func (e *Emulator) ConnectBrokerLink(address, session, peerID, role string, metadata map[string]string, timeout time.Duration) (*NetworkLink, error) {
	networkLink, err := DialBrokerLink(address, session, peerID, role, metadata, timeout)
	if err != nil {
		return nil, err
	}
	if err := e.AttachLink(networkLink); err != nil {
		_ = networkLink.Close()
		return nil, err
	}
	return networkLink, nil
}

// ConnectDirectLink is a convenience helper that dials and immediately
// attaches a point-to-point link to the emulator.
func (e *Emulator) ConnectDirectLink(address string, timeout time.Duration) (*NetworkLink, error) {
	networkLink, err := DialDirectLink(address, timeout)
	if err != nil {
		return nil, err
	}
	if err := e.AttachLink(networkLink); err != nil {
		_ = networkLink.Close()
		return nil, err
	}
	return networkLink, nil
}

// PublishMetadata sends out-of-band metadata to the other broker endpoint.
// It does not affect serial timing and is ignored by direct peers that do not
// consume metadata.
func (l *NetworkLink) PublishMetadata(metadata map[string]string) error {
	if l == nil || l.device == nil {
		return errors.New("gomeboy: nil network link")
	}
	return l.device.PublishMetadata(metadata)
}

// Metadata returns broker metadata sent by the other endpoint.
func (l *NetworkLink) Metadata() <-chan link.Message {
	if l == nil || l.device == nil {
		return nil
	}
	return l.device.Metadata()
}

func (l *NetworkLink) Close() error {
	if l == nil || l.device == nil {
		return nil
	}
	return l.device.Close()
}

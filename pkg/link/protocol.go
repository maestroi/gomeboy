package link

import (
	"encoding/json"
	"fmt"
	"io"
)

const ProtocolVersion = 1

const (
	MessageHello      = "hello"
	MessageReady      = "ready"
	MessageClock      = "clock"
	MessageClockReply = "clock_reply"
	MessageMetadata   = "metadata"
	MessageError      = "error"
)

// Message is the transport-level envelope used by both direct peers and the
// broker. The broker deliberately treats payload metadata as opaque so game-
// specific virtual peers can evolve independently of the emulator.
type Message struct {
	Version  int               `json:"version,omitempty"`
	Type     string            `json:"type"`
	Session  string            `json:"session,omitempty"`
	PeerID   string            `json:"peer_id,omitempty"`
	Role     string            `json:"role,omitempty"`
	Sequence uint64            `json:"sequence,omitempty"`
	Bit      bool              `json:"bit,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Error    string            `json:"error,omitempty"`
}

type Encoder struct{ enc *json.Encoder }
type Decoder struct{ dec *json.Decoder }

func NewEncoder(w io.Writer) *Encoder { return &Encoder{enc: json.NewEncoder(w)} }
func NewDecoder(r io.Reader) *Decoder { return &Decoder{dec: json.NewDecoder(r)} }

func (e *Encoder) Encode(m Message) error {
	if m.Version == 0 {
		m.Version = ProtocolVersion
	}
	return e.enc.Encode(m)
}

func (d *Decoder) Decode() (Message, error) {
	var m Message
	if err := d.dec.Decode(&m); err != nil {
		return Message{}, err
	}
	if m.Version != 0 && m.Version != ProtocolVersion {
		return Message{}, fmt.Errorf("link: unsupported protocol version %d", m.Version)
	}
	return m, nil
}

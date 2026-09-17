package linkcable

import "encoding/json"

const ProtocolVersion = 1

type Role string

const (
	RoleEmulator Role = "emulator"
	RoleVirtual  Role = "virtual"
)

const (
	msgHello          = "hello"
	msgWelcome        = "welcome"
	msgReady          = "ready"
	msgResult         = "result"
	msgExchange       = "exchange"
	msgExchangeResult = "exchange_result"
	msgMetadata       = "metadata"
	msgPeerMetadata   = "peer_metadata"
	msgError          = "error"
)

type message struct {
	Type          string                     `json:"type"`
	Version       int                        `json:"version,omitempty"`
	Session       string                     `json:"session,omitempty"`
	Endpoint      string                     `json:"endpoint,omitempty"`
	Role          Role                       `json:"role,omitempty"`
	Seq           uint64                     `json:"seq,omitempty"`
	RequestID     uint64                     `json:"request_id,omitempty"`
	Out           uint8                      `json:"out,omitempty"`
	In            uint8                      `json:"in,omitempty"`
	InternalClock bool                       `json:"internal_clock,omitempty"`
	Metadata      map[string]json.RawMessage `json:"metadata,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

type Result struct {
	Byte uint8
	Err  error
}

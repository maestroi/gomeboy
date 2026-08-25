package web

import (
	"encoding/json"
	"reflect"
	"testing"
)

// newTestPlayer returns a Player wired to a fresh hub with a buffered
// broadcast channel, so tests can observe broadcasts without running
// the hub loop.
func newTestPlayer(t *testing.T) *Player {
	t.Helper()

	h := &hub{
		broadcast:  make(chan []byte, 1),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}

	return &Player{hub: h}
}

func TestPlayer_PublishAgentState_BroadcastsJSON(t *testing.T) {
	p := newTestPlayer(t)

	state := AgentState{
		Step:        42,
		Goal:        "reach the lab",
		LastAction:  "press A",
		Observation: "standing at a door",
		Status:      AgentRunning,
	}

	p.PublishAgentState(state)

	msg, ok := <-p.hub.broadcast
	if !ok {
		t.Fatal("expected a broadcast message, got none")
	}

	if msg[0] != AgentUpdate {
		t.Fatalf("expected message type %d (AgentUpdate), got %d", AgentUpdate, msg[0])
	}

	if msg[1] != p.playerByte {
		t.Fatalf("expected player byte %d, got %d", p.playerByte, msg[1])
	}

	payload := msg[2:] // skip type byte and player byte

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("broadcast payload is not a JSON object: %v", err)
	}

	wantKeys := map[string]bool{"step": true, "goal": true, "last_action": true, "observation": true, "status": true}
	if len(m) != len(wantKeys) {
		t.Fatalf("expected JSON keys %v, got %v", wantKeys, m)
	}
	for k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Fatalf("expected JSON key %q, got %v", k, m)
		}
	}

	var got AgentState
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("broadcast payload does not decode into AgentState: %v", err)
	}

	if !reflect.DeepEqual(got, state) {
		t.Fatalf("expected %#+v, got %#+v", state, got)
	}
}

func TestPlayer_SatisfiesAgentPublisher(t *testing.T) {
	var _ AgentPublisher = (*Player)(nil)
}

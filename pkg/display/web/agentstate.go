package web

import (
	"encoding/json"
)

// AgentStatus is the running state of the agent driving the emulator.
type AgentStatus string

const (
	AgentIdle    AgentStatus = "idle"
	AgentRunning AgentStatus = "running"
	AgentPaused  AgentStatus = "paused"
	AgentError   AgentStatus = "error"
)

// AgentState is a snapshot of the agent's state, broadcast to all
// connected web clients as JSON.
type AgentState struct {
	Step        uint64      `json:"step"`
	Goal        string      `json:"goal"`
	LastAction  string      `json:"last_action"`
	Observation string      `json:"observation"`
	Status      AgentStatus `json:"status"`
}

// AgentPublisher publishes agent state to the web hub.
type AgentPublisher interface {
	PublishAgentState(AgentState)
}

// PublishAgentState broadcasts the agent state to all connected web
// clients as JSON, tagged with the AgentUpdate message type. It never
// blocks: if the hub's broadcast channel is full the state is dropped.
func (p *Player) PublishAgentState(s AgentState) {
	data, err := json.Marshal(s)
	if err != nil {
		// AgentState only contains an integer, strings and an enum:
		// marshalling cannot fail.
		panic(err)
	}

	select {
	case p.hub.broadcast <- p.createMessage(AgentUpdate, data):
	default:
	}
}

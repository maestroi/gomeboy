# Agent-driven emulator, bridged into the existing web overlay

Status: approved (design), ready for implementation planning.

## Problem

We want an LLM agent to play a Game Boy ROM (Pokémon Red, to start) while
a human watches and debugs it in a browser. `pkg/gomeboy` already provides
a clean, headless, step-driven embeddable emulator. `pkg/display/web` +
the Svelte client (`gomeboy-web/`) already provide a real-time spectator/
player experience over WebSocket, with a working Docker/swarm deploy.
Neither piece currently talks to the other, and neither has a concept of
"agent state" (goal / last action / observation) to show a human observer.

## Governing invariant

> The web display is an observer/controller, never the source of emulator
> timing. Emulator advancement remains owned by the agent loop.

This is the one rule everything else in this design follows. The adapter
described below must never call `StepFrame`/`StepFrames` on its own
initiative — only in direct response to the agent loop's own calls. If a
future requirement needs the display layer to drive stepping (e.g. a "let
a human take over" mode), that is a new decision, not an extension of this
adapter.

## Architecture

```
cmd/gomeboy-agent (new binary)
│
│  agent loop (not built in this plan — the loop's decision logic /
│  LLM integration is a separate, later piece of work)
│    │
│    ├── decides input           ─┐
│    ├── Emulator.Press/Release   │  pkg/gomeboy.Emulator
│    ├── Emulator.StepFrame()     │  (existing, untouched)
│    ├── Emulator.Read8/Frame()   │
│    ├── adapter.PublishFrame()   ─┘
│    └── publisher.PublishAgentState(...)
│         │                              │
│         ▼                              ▼
│    webbridge.Adapter (new, small) AgentPublisher (new, small,
│      - satisfies                  implemented by the web hub)
│        pkg/emulator.Controller      - PublishAgentState(AgentState)
│      - pushes Frame() onto fb       - independent of the bridge
│        chan []byte
│      - never calls StepFrame itself
│         │                              │
│         └──────────────┬───────────────┘
│                         ▼
│    pkg/display/web hub (existing, unmodified wiring:
│    Driver.Start(controller, fb, pressed, released))
│         │
│         ▼
│    Svelte client (existing WS protocol + one new message type)
│    ┌─────────────┬──────────────┐
│    │  game view  │ agent panel  │ (new AgentPanel.svelte)
│    └─────────────┴──────────────┘
```

## Components

### 1. `pkg/gomeboy` — no changes

Already provides everything the agent loop needs: `StepFrame`,
`Press`/`Release`, `Read8`/`Read`, `Frame()`, `SaveState`/`LoadState`.
Stays generic and free of any web/agent/runtime concerns.

### 2. `webbridge` package (new, small — lives under `pkg/webbridge` or
   inlined in `cmd/gomeboy-agent` if it stays under ~100 lines)

Bridges the emulator's step-driven model into the display layer's push/
channel model, without owning timing. `Adapter` is exactly this bridge —
emulator/display compatibility — and nothing else. Agent-state
publication (goal/action/observation) is a separate concern (see
"Agent state publication" below); the adapter does not own it.

```go
// Emulator is the minimal surface webbridge needs. pkg/gomeboy.Emulator
// satisfies it; tests use a fake instead of a real ROM.
type Emulator interface {
    Frame() gomeboy.Frame // or []byte, whichever pkg/gomeboy already returns
}

// Adapter satisfies pkg/emulator.Controller for an Emulator. It never
// advances the emulator itself; PublishFrame is called by the agent loop
// after it has already stepped.
type Adapter struct {
    emu    Emulator
    fb     chan<- []byte
    paused atomic.Bool
}

func NewAdapter(emu Emulator, fb chan<- []byte) *Adapter

// Controller interface methods (pkg/emulator.Controller):
func (a *Adapter) LoadROM(string) error
func (a *Adapter) Pause()         // sets paused; does not sleep, does not block
func (a *Adapter) Resume()        // clears paused
func (a *Adapter) Paused() bool
func (a *Adapter) Initialised() bool
func (a *Adapter) QuickSave() error
func (a *Adapter) QuickLoad() error
func (a *Adapter) SetSpeed(int)
func (a *Adapter) Speed() int

// PublishFrame pushes the emulator's current frame to the display hub.
// Called by the agent loop after StepFrame; a no-op (frame dropped) if
// a.Paused() or the fb channel is full — never blocks the agent loop.
func (a *Adapter) PublishFrame()
```

**Pause semantics are advisory/cooperative, not enforced.** `Pause()`
only flips a flag; it does not sleep, does not spawn a ticker, and does
not itself prevent anything. `PublishFrame` becomes a no-op while paused,
but the agent loop MUST check `Paused()` itself before every emulator
advancement operation (`Press`/`Release`/`StepFrame`) — `Pause()` does
not guarantee the emulator cannot advance. This is a deliberate
consequence of the governing invariant (the adapter never steps), stated
explicitly so a future reader doesn't assume `Pause()` is authoritative.

**No hidden realtime loop:** the adapter has no goroutine that calls
`StepFrame`, no ticker, nothing that advances emulation without the agent
loop initiating it. The 60Hz ticker pattern used in `cmd/gomeboy-web`'s
`main.go` (driving frames on a timer) is explicitly not reused here.

### 3. `AgentState` protocol message (generic, game-agnostic)

New WS message type in `pkg/display/web` (next value in the existing
`Type` enum in `player.go`, mirrored in `game.ts`'s `EventType`):

```go
type AgentStatus string

const (
    AgentIdle    AgentStatus = "idle"
    AgentRunning AgentStatus = "running"
    AgentPaused  AgentStatus = "paused"
    AgentError   AgentStatus = "error"
)

type AgentState struct {
    Step        uint64
    Goal        string
    LastAction  string
    Observation string
    Status      AgentStatus
}
```

`Status` is a typed enum, not an arbitrary string, so Go and Svelte can't
drift on spelling (`"paused"` vs `"Paused"`). No timestamp field for v1 —
`Step` already gives ordering; the browser can timestamp on receipt if it
ever needs wall-clock display. No Pokémon-specific fields (no map ID, no
coordinates). If a later project wants to show semantic Pokémon state, it
publishes that as part of `Observation` (a string) or as a separate,
later protocol extension — not by growing this struct today.

### Agent state publication (separate from `webbridge`)

Publishing `AgentState` is not the bridge's job — the bridge is
"step-driven emulator → existing display/controller", full stop. A small
publisher interface, implemented by the web hub/driver, owns it instead:

```go
// in pkg/display/web
type AgentPublisher interface {
    PublishAgentState(AgentState)
}
```

The agent loop holds both `*webbridge.Adapter` (for `PublishFrame` and
`Controller` semantics) and an `AgentPublisher` (for state), and calls
each independently. Nothing about agent-state publication routes through
`webbridge.Adapter`.

### 4. `cmd/gomeboy-agent` (new binary, separate from `cmd/gomeboy-web`)

Wires `pkg/gomeboy.Emulator` + `webbridge.Adapter` + `pkg/display/web`
together, the same way `cmd/gomeboy-web/main.go` wires `gameboy.GameBoy` +
the web driver today. Does not add an `-agent` flag to `gomeboy-web` —
kept as its own binary so the agent runtime (eventually: the actual
decision loop / LLM calls) can grow independently of the plain spectator
binary.

This plan does not include the agent's decision logic (what to do with
an LLM, prompting, Pokémon-specific reasoning) — only the plumbing that
lets such a loop, once written, drive the emulator and be observed in the
browser. The loop in this plan is a stub that steps forward and presses
no buttons, enough to prove the bridge end-to-end.

### 5. Svelte: `AgentPanel.svelte` (new, sibling to `Controls.svelte`)

Subscribes to `AgentState` messages, renders goal/last action/
observation/status. Does not replace or modify `Controls.svelte`.

## Data flow (steady state)

The agent command's main loop owns pacing and the pause check — not the
adapter, not the publisher:

```go
for ctx.Err() == nil {
    if adapter.Paused() {
        time.Sleep(pollInterval)
        continue
    }

    // eventually: decision := agent.Decide(...); emu.Press(decision)
    emu.StepFrame()
    adapter.PublishFrame()
    publisher.PublishAgentState(webweb.AgentState{Step: step, ...})

    step++
}
```

1. Agent loop calls `Press`/`Release`/`StepFrame` on the emulator some
   number of times, checking `adapter.Paused()` before each cycle.
2. Agent loop calls `adapter.PublishFrame()` (bridge) and, when it has a
   new decision, `publisher.PublishAgentState(...)` (separate publisher)
   — two independent calls, not one combined step.
3. Adapter pushes the frame onto the existing `fb` channel the hub
   already reads via `Driver.Start`; the publisher broadcasts
   `AgentState` to all connected WS clients via the hub's existing
   broadcast channel.
4. Svelte client renders the frame in the existing game view and the new
   state in `AgentPanel`.
5. A human browser client's button presses (if any connect) still flow
   through the existing `pressed`/`released` channels into
   `Emulator.Press`/`Release`, same as agent input, with no arbitration
   between the two. **Consequence, stated explicitly:** if a human and
   the agent both connect, their inputs combine into shared button state
   with no ordering or precedence — this can look like erratic input.
   That's an accepted limitation of this plumbing stage, not a bridge
   bug; input arbitration is intentionally deferred to a later plan.

## Error handling

- If the agent loop panics/exits, `cmd/gomeboy-agent` should let the
  process crash normally (no special recovery) — restart policy is a
  deploy concern (already handled by the existing swarm
  `restart_policy: on-failure`), not this design's.
- `PublishFrame`/`PublishState` never block: both use non-blocking sends
  (`select` with `default`) matching the existing `fb` channel's usage
  in `cmd/gomeboy-web/main.go`.

## Testing

- `webbridge.Adapter` is tested against a fake implementing the small
  `webbridge.Emulator` interface (just `Frame()`), not against a real
  ROM, so unit tests don't depend on ROM fixtures. The narrow interface
  is what makes this fake trivial — the adapter can't accidentally
  couple itself to more of `pkg/gomeboy` than `Frame()`.
- One real-ROM integration test (loads a small test ROM via
  `pkg/gomeboy.Emulator`, steps it, asserts a frame reaches the `fb`
  channel via the real adapter, and `AgentState` reaches the hub's
  broadcast channel via the publisher) covers the real wiring end-to-end,
  matching the existing `pkg/gomeboy` test style (`bench_test.go`,
  `gomeboy_test.go` already use a fixture ROM).
- No test harness exists for the Svelte client; `AgentPanel.svelte` is
  manual-QA'd like `Controls.svelte` already is.

## Explicit exclusions (this plan)

- No changes to `pkg/gomeboy` (stays generic, untouched).
- No removal of Fyne/glfw drivers — nothing depends on them, left as-is.
- No `pkg/pokemon` RAM decoder — agent gets raw `Read8`/`Read`; semantic
  Pokémon state decoding is a separate, later sub-project.
- No Ebitengine/native overlay — the web stack already covers spectating;
  revisit only if it proves insufficient.
- No agent decision logic / LLM integration — this plan is the plumbing
  only.

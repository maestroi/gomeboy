# Agent-driven emulator, bridged into the existing web overlay

Status: approved (design), not yet planned/implemented.

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
│    └── publish AgentState       ─┘
│         │
│         ▼
│    webbridge.Adapter (new, small)
│      - satisfies pkg/emulator.Controller
│      - pushes Emulator.Frame() onto fb chan []byte after each step
│      - exposes AgentState publication as a plain method call
│      - never calls StepFrame itself
│         │
│         ▼
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

Bridges `pkg/gomeboy.Emulator`'s step-driven model into the display
layer's push/channel model, without owning timing.

```go
// Adapter satisfies pkg/emulator.Controller for a pkg/gomeboy.Emulator.
// It never advances the emulator itself; PublishFrame/PublishState are
// called by the agent loop after it has already stepped.
type Adapter struct {
    emu     *gomeboy.Emulator
    fb      chan<- []byte
    paused  bool // set only by Pause()/Resume(); read by agent loop
}

func NewAdapter(emu *gomeboy.Emulator, fb chan<- []byte) *Adapter

// Controller interface methods (pkg/emulator.Controller):
func (a *Adapter) LoadROM(string) error
func (a *Adapter) Pause()         // sets paused=true; does not sleep, does not block
func (a *Adapter) Resume()        // sets paused=false
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

// PublishState broadcasts agent state (goal/action/observation/status/
// step) to connected clients via the hub.
func (a *Adapter) PublishState(s AgentState)
```

**Pause semantics (precise, per review):** `Pause()` only flips a flag the
agent loop is expected to check before calling `StepFrame` again, and
makes `PublishFrame`/`PublishState` no-ops while set. It does not sleep,
does not spawn a ticker, and does not itself stop anything — the agent
loop remains in control of whether stepping actually happens. This keeps
the adapter honest about not owning timing even in the pause path.

**No hidden realtime loop:** the adapter has no goroutine that calls
`StepFrame`, no ticker, nothing that advances emulation without the agent
loop initiating it. The 60Hz ticker pattern used in `cmd/gomeboy-web`'s
`main.go` (driving frames on a timer) is explicitly not reused here.

### 3. `AgentState` protocol message (generic, game-agnostic)

New WS message type in `pkg/display/web` (next value in the existing
`Type` enum in `player.go`, mirrored in `game.ts`'s `EventType`):

```go
type AgentState struct {
    Step        uint64
    Goal        string
    LastAction  string
    Observation string
    Status      string // e.g. "running", "paused", "error"
}
```

No Pokémon-specific fields (no map ID, no coordinates). If a later
project wants to show semantic Pokémon state, it publishes that as part
of `Observation` (a string) or as a separate, later protocol extension —
not by growing this struct today.

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

1. Agent loop calls `Press`/`Release`/`StepFrame` on `pkg/gomeboy.Emulator`
   some number of times.
2. Agent loop calls `adapter.PublishFrame()` and, when it has a new
   decision, `adapter.PublishState(...)`.
3. Adapter pushes the frame onto the existing `fb` channel the hub
   already reads via `Driver.Start`; broadcasts `AgentState` to all
   connected WS clients via the hub's existing broadcast channel.
4. Svelte client renders the frame in the existing game view and the new
   state in `AgentPanel`.
5. A human browser client's button presses (if any connect) still flow
   through the existing `pressed`/`released` channels into
   `Emulator.Press`/`Release` — human and agent input are not
   distinguished at this layer; that's out of scope here.

## Error handling

- If the agent loop panics/exits, `cmd/gomeboy-agent` should let the
  process crash normally (no special recovery) — restart policy is a
  deploy concern (already handled by the existing swarm
  `restart_policy: on-failure`), not this design's.
- `PublishFrame`/`PublishState` never block: both use non-blocking sends
  (`select` with `default`) matching the existing `fb` channel's usage
  in `cmd/gomeboy-web/main.go`.

## Testing

- `webbridge.Adapter` is tested against a small fake satisfying whatever
  minimal interface the adapter actually needs from the emulator (frame
  bytes + pause state), not against a real ROM, so tests don't depend on
  ROM fixtures. One real-ROM integration test (loads a small test ROM via
  `pkg/gomeboy`, steps it, asserts a frame reaches the `fb` channel and
  `AgentState` reaches the hub's broadcast channel) covers the real wiring
  end-to-end, matching the existing `pkg/gomeboy` test style
  (`bench_test.go`, `gomeboy_test.go` already use a fixture ROM).
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

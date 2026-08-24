# RUNNOTES — Task 2: AgentState / AgentPublisher on the web hub

## Done
- `pkg/display/web/agentstate.go`: `AgentStatus` (idle/running/paused/error),
  `AgentState{Step,Goal,LastAction,Observation,Status}` with JSON tags
  `step`,`goal`,`last_action`,`observation`,`status`; `AgentPublisher`
  interface; `(*Player).PublishAgentState` broadcasts
  `createMessage(AgentUpdate, json)` via non-blocking `select` send
  (drops if hub broadcast full, per design spec's never-block rule).
- `pkg/display/web/events.go`: `AgentUpdate` appended as LAST value of the
  Type const block (= 16). Frame=0 ... PlayerIdentify=15 unchanged (wire protocol).
- `pkg/display/web/agentstate_test.go` (TDD: red first, then green):
  `newTestPlayer` builds a Player with a fresh hub (buffered broadcast chan);
  `TestPlayer_PublishAgentState_BroadcastsJSON` asserts message bytes
  [AgentUpdate, playerByte, json], exact JSON keys, round-trip equality;
  `TestPlayer_SatisfiesAgentPublisher` is a compile-time assertion.
- `go test -race ./pkg/display/web/` passes. Full `go test ./...`: all ok
  except `tests` pkg (TestAge, Test_Acid2) — PRE-EXISTING fixture failures
  (missing `tests/roms/age`, acid-hell baseline diff); verified via
  `git stash` that they fail without my changes.

## Next task must know
- Plan file `docs/superpowers/plans/2026-08-24-agent-web-overlay.md` still
  does not exist (only the design spec in docs/superpowers/specs/). JSON tags
  were chosen here (snake_case); if a later plan pins different tags, the test
  `TestPlayer_PublishAgentState_BroadcastsJSON` asserts them.
- Package `init()` (player.go) starts the real hub: binds :8090 and runs the
  hub loop in tests — port must be free when running `go test ./pkg/display/web`.
- Message framing: every WS message is `[Type, playerByte, payload...]`;
  AgentUpdate payload is JSON (unlike the binary frame protocol).
- Svelte side (AgentPanel.svelte, game.ts EventType mirror) is still TODO;
  client must handle a new message type byte 0x10 whose payload is JSON.
- `cmd/gomeboy-agent` binary does not exist yet.

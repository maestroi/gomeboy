# RUNNOTES — Task 4: Svelte agent panel (repair run)
## Done
- `src/lib/game.ts`: `AgentUpdate` appended LAST to EventType (index 16, after
  PlayerIdentify — mirrors pkg/display/web/events.go); `AgentStatus` enum
  (idle/running/paused/error); `AgentStateData` interface
  {step,goal,last_action,observation,status}; `Game.agentState` Writable
  (default idle/empty); dispatch case JSON-decodes via TextDecoder.
- `src/components/Player/AgentPanel.svelte`: new panel showing status badge +
  Step/Goal/Last Action/Observation from $agentState, Svelte 4 + scss, styled
  after ClientList.
- `src/components/Player/Player.svelte`: `<AgentPanel/>` mounted as sibling
  right after `<Controls/>` inside the same `{#if controls}` block.
- Verified: npm build clean; svelte-check = 154 pre-existing errors, 0 new
  (vs pristine baseline); live e2e: agent on :8090 + WS probe = FrameSync
  received (game view renders), 481 AgentUpdates/8s, step increasing,
  status "running", keys match Go JSON. Previous run also passed a full
  headless-Chrome DOM check (panel live, step 16465, canvas 160x144).
- Code already in snapshot commit 6c65b19 (preserve-failed-run); this run
  re-verified it byte-identical. Left UNCOMMITTED for the runner, per the
  task-3 pattern.

## Next task must know
- WIRE FORMAT: createMessage always prepends the player byte, so AgentUpdate
  on the wire is [16, playerByte, JSON]. game.ts strips the type byte before
  the switch (line ~300), so the case does `eventData.slice(1)`. Task 2's
  Go test asserts this layout (payload := msg[2:]). Do not "fix" either side.
- AgentUpdate is intentionally NOT in the playerEvents array (no per-player
  event fan-out), same handling as ServerInfo.
- game.ts hardcodes `ws://192.168.1.154:8090/` (pre-existing LAN IP). For
  local browser checks, temporarily point at ws://localhost:8090/ and
  revert before finishing. Port 5173 may be held by an unrelated vite.
- The plan file docs/superpowers/plans/2026-08-24-agent-web-overlay.md still
  does not exist; work was reconstructed from the design spec + instructions.
- ROM fixture: extract tests/roms.zip into tests/roms (done in this run).
  Keep :8090 free for go test ./cmd/gomeboy-agent (web driver init binds it).
- Next likely task: real agent decision loop (LLM) replacing the stub in
  cmd/gomeboy-agent; human+agent input arbitration still unimplemented.

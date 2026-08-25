# RUNNOTES — SWARM-2: swarm stack file + deploy README

## Outcome: SUCCESS
- Created deploy/docker-compose.swarm.yml (single service, compose 3.8).
- Created deploy/README.md (build/tag, deploy, open questions).
- `docker compose -f deploy/docker-compose.swarm.yml config` parses clean
  (exit 0; one benign warning: `version` attribute obsolete — kept on
  purpose to document the v3.8+ requirement).

## Stack shape (deploy/docker-compose.swarm.yml)
- image: gomeboy-web:latest (placeholder; real registry/tag is a deploy-time
  decision via swarmpit-prod, per task).
- command: ["-rom", "/roms/pokemon_red.gb"] — the Dockerfile ENTRYPOINT is
  /gomeboy-web with NO default CMD, so the ROM flag must come from compose
  or the operator. cmd/gomeboy-web/main.go:33 defines -rom, no default path.
- ports: 8090:8090. Named volume `roms` -> /roms (parent dir of the ROM,
  not a single file; .dockerignore already keeps roms out of the image).
- deploy.replicas: 1 (stateful/single-writer). restart_policy:
  on-failure, delay 5s (no crash-loop hammering).
- NO .sav volume, NO ingress/Traefik labels — both left as open questions.

## Open questions (deliberately unresolved, documented in BOTH the compose
file header comments and deploy/README.md §3)
1. Save persistence: .sav/.state written via gameboy WithSaveDir (empty dir
   = beside ROM / CWD; internal/gameboy/gameboy.go:52, state.go:101).
   Spectator saves across restarts? First deployer must decide + add mount.
2. Ingress/TLS: existing swarmpit-prod stack pattern NOT confirmed — the
   swarmpit-prod MCP was not available in this run (only agent-runner MCP
   granted). First deployer must list existing stacks and match labels.

## For SWARM-3 (deploy)
- Do NOT deploy until a human confirms both open questions above.
- Every swarm node must be able to pull gomeboy-web:latest (build on each
  node or push to a shared registry).
- ROM must be copied into the named volume after first deploy (README has
  the `docker run --rm -v gomeboy_roms:/roms ... cp` one-liner). Volume is
  named <stack>_roms (stack name = whatever is passed to `docker stack
  deploy`, e.g. gomeboy).

---

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

## Next task must know
- WIRE FORMAT: createMessage always prepends the player byte, so AgentUpdate
  on the wire is [16, playerByte, JSON]. game.ts strips the type byte before
  the switch, so the case does `eventData.slice(1)`. Task 2's
  Go test asserts this layout (payload := msg[2:]). Do not "fix" either side.
- AgentUpdate is intentionally NOT in the playerEvents array (no per-player
  event fan-out), same handling as ServerInfo.
- game.ts uses `ws://{host}:8090/` and substitutes `location.hostname`.
- Next likely task: real agent decision loop (LLM) replacing the stub in
  cmd/gomeboy-agent; human+agent input arbitration still unimplemented.

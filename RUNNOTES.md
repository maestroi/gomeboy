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

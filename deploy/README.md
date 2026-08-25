# Deploying GomeBoy to a Docker Swarm

This directory holds the swarm stack definition
(`docker-compose.swarm.yml`) for the web emulator image built by the
repository-root `Dockerfile` (SWARM-1).

**This task (SWARM-2) does not deploy anything to the live swarm.** Deployment
is SWARM-3, and it must only happen after a human has confirmed the two open
questions at the bottom of this file.

## 1. Build and tag the image

Build from the repository root (the Dockerfile is multi-stage: Svelte frontend
→ Go web binary → alpine final image, `EXPOSE 8090`,
`ENTRYPOINT ["/gomeboy-web"]` with no default CMD):

```sh
docker build -t gomeboy-web:latest .
```

The compose file references the placeholder tag `gomeboy-web:latest`. Actual
registry/tag naming is an operational decision made at deploy time via
swarmpit-prod — if you push to a registry, re-tag and update the `image:`
field (or override it) accordingly:

```sh
docker tag gomeboy-web:latest <registry>/gomeboy-web:<tag>
docker push <registry>/gomeboy-web:<tag>
```

Every swarm node that may run the task needs the image: either build it on
each node, or push it to a registry all nodes can pull from.

## 2. Deploy the stack

Either via the swarmpit-prod MCP tools (`create_stack_file` with the contents
of `docker-compose.swarm.yml`, then `create_stack`), or plainly:

```sh
docker stack deploy -c deploy/docker-compose.swarm.yml gomeboy
```

The stack runs one replica (`deploy.replicas: 1` — the emulator is
stateful/single-writer; do not raise this) with
`restart_policy: on-failure` and a `5s` delay so a crash loop cannot hammer
the host. Port `8090` is published; the Svelte frontend is served at
`http://<node>:8090/app/` and the websocket at `ws://<node>:8090/`.

### ROM

The stack mounts a named volume at `/roms` (the parent directory of the ROM,
not a single file) and starts with `-rom /roms/pokemon_red.gb`. After the
first deploy, drop a ROM into the volume without rebuilding the image:

```sh
docker run --rm -v gomeboy_roms:/roms -v "$PWD/roms:/src" alpine \
  cp /src/pokemon_red.gb /roms/pokemon_red.gb
```

(If you rename the ROM, also update the `command:` in the compose file so the
filename matches, then re-deploy.)

## 3. Open questions — decide deliberately before deploying

These are intentionally **not** decided in the stack file. They are also
commented at the top of `docker-compose.swarm.yml`. Whoever deploys first
must make both calls explicitly:

1. **Save persistence.** GomeBoy writes `.sav`/`.state` files
   (`internal/gameboy` `WithSaveDir`; with no save dir set they land beside
   the ROM / in the container CWD). The stack currently mounts only the ROM
   directory. Question: do spectator saves need to survive container
   restarts? If yes, add a persistent mount for the save output before
   deploying; if no, say so and leave it as-is.
2. **Ingress / TLS.** No reverse-proxy service or ingress labels are defined
   in the stack. The existing swarmpit-prod stacks use a specific ingress
   pattern that could not be confirmed when this file was written (the
   swarmpit-prod MCP was not available to inspect existing stacks). Before
   exposing port 8090 beyond the swarm nodes, list the existing stacks,
   confirm the pattern, and add the matching labels so this stack matches the
   existing ingress setup.

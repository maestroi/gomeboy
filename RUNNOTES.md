# RUNNOTES — ERR-1: web listener lifecycle, returned failures, safe hub

## What changed
- `pkg/display/web/hub.go`: listener moved out of package init. `newHub`
  is inert; `start()` (sync.Once) binds `*listenAddr`, serves a dedicated
  `http.ServeMux` (WS at `/`, static under `/app/` if GOMEBOY_WEB_STATIC_DIR
  set) and returns `web: listen on <addr>: <err>` on bind failure — no
  log.Fatal/panic. `stop()` (sync.Once) is idempotent. Rejected upgrades
  are logged with remote addr/method/path/user-agent (gorilla v1.5.3 writes
  the HTTP error itself). `info()` logs via the hub logger.
- `pkg/display/web/player.go`: `init()` only builds hub/players and
  `display.Install`s with DriverOption `listen` -> flag `web-listen`
  (default `:8090`, read at start time). `Start` returns the hub's bind
  error; `Stop` calls idempotent `hub.stop()`. `broadcastFrame` extracted;
  encode failures log + drop the frame (no panic); `encode` package var is
  the test seam. ReadPump guards nil gb / short messages.
- `pkg/display/web/hub_test.go`: new — WEB-IMPORT, WEB-BIND, WEB-STOP,
  WEB-STATIC (+upgrade logging), WEB-ERRORS. `agentstate_test.go` dropped
  its `clients` map literal (sync.Map change).

## Gotchas for the next task
- `stop()` closes the listener DIRECTLY, then `server.Shutdown`: Shutdown
  only closes listeners `Serve` has tracked; a stop racing ahead of Serve
  leaks the address (rebind: "address already in use").
- `hub.clients` is now `sync.Map`: the run loop mutates it while handlers
  and client pumps iterate it, and client.go calls `sendAllButClient`
  while holding `hub.mu` (plain map + that mutex would deadlock; client.go
  is not in the modify list).
- The hub `run()` loop lives for process lifetime on purpose: client.go
  pumps send on the unbuffered `unregister` channel in defers; if run()
  exited at stop, those sends would deadlock.
- Pre-existing races (NOT fixed; client.go out of scope): `c.avgLatency`
  written unlocked in ReadPump defer (client.go:34) vs under c.mu in
  WritePump (client.go:140) -> `go test -race` fails on
  TestStaticMountWebSocketRouteAndUpgradeLogging; `p.c`/config flags unlocked.

## Verification (all green)
- `go test ./pkg/display/web/ -v` 8/8 pass; build/vet/gofmt clean; -race
  fails only on the pre-existing client.go avgLatency race above.

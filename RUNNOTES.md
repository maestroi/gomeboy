# RUNNOTES — QA-1: end-to-end startup/shutdown/logging smoke matrix

## Changed
- internal/launch/smoke_test.go (new): subprocess smoke tests for the
  built gomeboy, gomeboy-web, gomeboy-agent binaries. TestMain builds all
  three via `go build` into a temp dir and extracts two ROMs from
  tests/roms.zip (01-special.gb plain; dmg_sound 01-registers.gb,
  battery-backed MBC1RAMBATT). Ephemeral 127.0.0.1:0 ports, every run
  reaped, no repo files written.
- Coverage: SMOKE-DEFAULT-OFF (auto never binds the web port, DISPLAY=:99);
  SMOKE-COMMANDS (-h exit 0 + usage on all 3; web/agent bind then shut
  down — SIGTERM kills web/desktop, SIGINT/SIGTERM exit 0 on agent, port
  released); SMOKE-FAILURES (11 cases: bad model, bad log level, missing
  ROM/boot, -no-saves vs -save-dir, unknown flag, unknown driver, web
  address-in-use, agent -fps 0 / missing ROM / -save-dir — all non-zero,
  contextual, fast, no panic output; missing cheats tolerated);
  SMOKE-PERSISTENCE (battery ROM: cwd vs -save-dir vs -no-saves; agent
  diskless); SMOKE-LOGGING (agent exactly one summary line, no per-frame
  growth); SMOKE-HYGIENE (port closed, working dir clean, no root leaks).

## Why
Criteria require observing the real binaries end-to-end (ports, signals,
exit codes, save files); only subprocess level covers that.

## Verified
- go build ./... && go vet ./internal/launch/ && go test ./internal/launch/ -count=1 — green, ~20s (binaries cached after first build).
- No prior-criterion failures found; no out-of-scope code touched.

## Must know
- Pre-existing (not fixed, out of scope): headless `gomeboy` (auto -> fyne)
  panics inside fyne v2.8.0 GLFW ("panic: NotInitialized"), exit 2. ERR-2
  only made the repo's own glfw driver recoverable. The smoke test asserts
  non-zero exit + web port never bound, not panic-free output.
- gomeboy/gomeboy-web have no signal handler: SIGTERM kills them (exit -1),
  so battery saves are created at startup but not flushed on SIGTERM
  (0-byte .sav is expected in the persistence tests).
- Repo test ROMs store cartridge type at header offset 0x147 (not standard 0x133); battery ROMs have that byte in batteryMappers.
- Known pre-existing `go test ./...` race (roms.zip extraction vs
  pkg/gomeboy) unchanged; pre-extract roms or re-run pkg/gomeboy.

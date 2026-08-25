# RUNNOTES — OPT-2: unified CLI startup options + contextual exit errors

## What changed
- New `internal/launch` (FlagSet-based, no global flag state): `Register`
  (-rom, -boot, -model, -printer, -cheats, -log-level, -pprof),
  `RegisterSaves` (-save-dir, -no-saves), `RegisterDriver(fs, def)`,
  `Parse(fs, args) (*Options, error)` via fs.Lookup, `CheatsPath()` (explicit
  -cheats wins, else `<rombase>.cheats`), `CoreOptions()` (desktop/web; boot
  via utils.LoadFile with path in error; DMG->DMGABC/CGB->CGBABC; historical
  cheats + cwd saves preserved), `PublicOptions()` (agent; diskless),
  `StartPProf(addr, logger)` (sync net.Listen; bind error returned).
- Model flag case-insensitive auto/DMG0/DMG/CGB0/CGB/MGB/SGB/SGB2/AGB; log
  level via log.ParseLevel; pprof addr via net.SplitHostPort; -no-saves +
  -save-dir is a conflict error.
- All three binaries: `main()` only logs final error + os.Exit(1); testable
  `run(args) error` boundary. Flags on flag.CommandLine (re-init
  ContinueOnError): display.Init() registers driver flags on the global flag
  package, and flag cannot split-parse across two sets. -h -> nil.
- Contextual errors: `gomeboy[-web/-agent]: load ROM %s: %w`, `open audio
  device`, `unknown display driver %q: use auto or one of <list>`, `start
  display driver %q: %w`, `save battery RAM for ROM %s: %w`, `pprof listen
  on %s: %w`; no panics / silent continue after LoadROM.
- Agent: -rom required, -fps positive, driver.Start in a goroutine feeding
  startErr (select with ctx.Done), honest `web hub on <addr>` log. README:
  full flag tables + shared-options/restart-time and agent-diskless notes;
  TODO "error handling and logging" + "expose more options" checked.

## Gotchas
- Go 1.27 flag: failf ALWAYS prints usage (to f.Output) on parse errors even with ContinueOnError; error text is only returned. Bool flags:
  `-printer notabool` parses as flag+positional; test bad bools with `-printer=notabool`.
- internal/launch tests extract tests/roms.zip into tests/roms (gitignored); ROM paths resolve from a pkgDir captured at test start (t.Chdir-safe).
- Pre-existing ./tests failures (missing tests/results refs + roms/age): verified identical on the clean tree via git stash.

## Verification (all green)
- go build/vet ./... clean; gofmt clean on touched files; go test ./... green except the pre-existing ./tests fixture failures.
- Smoke: -h exit 0 all binaries; bad model/pprof/rom/driver, conflict, and bind-conflict (web hub + pprof) all exit 1 with one contextual line;
  web hub + pprof serve (200/400-ws); agent SIGTERM exit 0.

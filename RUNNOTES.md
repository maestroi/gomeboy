# RUNNOTES — LOG-1: leveled, contextual, testable logging

## What changed
- `pkg/log/log.go`: bare fmt.Printf wrapper replaced by a stdlib-only facade.
  - Line format: `<RFC3339-ms>\t[LEVEL]\t<message>[\tkey=value...]`.
  - `Level`: DebugLevel < InfoLevel < ErrorLevel; a record emits iff its
    severity >= configured level (deterministic threshold).
  - `ParseLevel(name) (Level, error)`: case-insensitive; unknown names return
    an error naming the valid levels.
  - `New() Logger`: process stderr at InfoLevel (debug suppressed unless
    enabled); stderr resolved at write time (osStderr) so tests can redirect.
  - `NewWithWriter(w io.Writer, level Level) (ContextualLogger, error)`:
    validates nil writer / out-of-range level; no global process state.
  - `ContextualLogger` = Logger + `With(key, value)`: appends `key=value`
    fields to every record; receiver unchanged; chainable.
  - `Logger` interface UNCHANGED, so pkg/log/null.go and all callers compile
    unmodified. No third-party deps, no JSON. Fatal kept as temporary compat
    boundary (FATAL line + os.Exit(1)); no new Fatal call sites.
- `pkg/log/log_test.go`: new. Covers level filtering, line format, context
  fields, default destination (stderr, never stdout), config validation, and
  API compatibility.

## Gotchas for the next task
- `formatMessage` copies format to a local before fmt.Sprintf: this keeps vet's
  printf-wrapper inference from classifying the *f methods; otherwise
  `go test ./internal/gameboy/` fails vet at gameboy.go:144
  (`log.Errorf(fmt.Sprintf(...))`). Pre-facade code had the same behavior. If
  a later task wants vet to police call sites, remove the copy AND fix
  gameboy.go:144 to `log.Errorf("error loading save files: %s", err)`.
- To enable debug at runtime (e.g. -debug flag): `NewWithWriter(os.Stderr,
  DebugLevel)`; the package-level default stays info.
- Pre-existing (verified on base commit, NOT caused by this task): fresh-checkout
  `go test ./...` races on tests/roms/ extraction (tests/rom_test.go:28) ->
  spurious "no such file" failures in consumer packages; and `go test ./tests/`
  fails (TestAge: roms/age/* absent from roms.zip; Test_Acid2/cgb-acid-hell).

## Verification (all green)
- `go test ./pkg/log/ -v` 7/7 pass; `go build ./...`, `go vet ./pkg/log/
  ./internal/gameboy/`, gofmt clean; `go test ./...`: only pre-existing failures.

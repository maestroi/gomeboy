# RUNNOTES — Task 1: webbridge.Adapter

## Done
- Created `pkg/webbridge/adapter.go` + `adapter_test.go` (TDD: tests first, red, impl, green; race-clean).
- `Adapter` satisfies `pkg/emulator.Controller` (compile-asserted). `*gomeboy.Emulator`
  satisfies the minimal `webbridge.Emulator` interface (compile-asserted).
- All 9 planned tests pass: SatisfiesController, InitiallyInitialisedAndNotPaused,
  PauseResumeAreAdvisoryOnly, PublishFrame_SendsCopyOfFrameData,
  PublishFrame_NoopWhenPaused, PublishFrame_NeverBlocksOnFullChannel,
  LoadROM_DelegatesAndTracksInitialised (success+failure subtests),
  QuickSaveQuickLoad_Delegate, SetSpeedSpeed_RoundTrip.

## Design decisions (plan file was missing — see below)
- `webbridge.Emulator` interface = `Frame() gomeboy.Frame`, `LoadROM(string) error`,
  `QuickSave() error`, `QuickLoad() error`. Task requires delegating LoadROM/
  QuickSave/QuickLoad, so they must be on the interface (design spec's "just
  Frame()" comment was stale).
- `Pause()/Resume()` are adapter-local `atomic.Bool` flags (advisory only).
  `gomeboy.Emulator` has no pause methods, so the adapter owns the flag.
- `Initialised()` tracked in adapter: set true on successful LoadROM, false on
  failure or before any load.
- `SetSpeed(int)/Speed() int` are adapter-local `atomic.Int64`, default 1.
  They are NOT part of `emulator.Controller` (checked pkg/emulator/controller.go)
  and gomeboy.Emulator has no speed methods; kept as extra Adapter methods per
  the design spec's method list.
- `PublishFrame()`: returns before `Frame()` if paused; copies `f.RGB` into a
  fresh slice; non-blocking `select` send with `default` drop.

## Next task must know
- `emulator.Controller` has exactly 7 methods (no SetSpeed/Speed). If a later
  task adds speed to Controller, the Adapter already implements it.
- The web hub (`pkg/display/web`) and `cmd/gomeboy-agent` do not exist yet;
  `main.go` still wires `*gameboy.GameBoy` directly as Controller.
- No real-ROM integration test was added (unit tests use the fake, per design).

## Blocker hit
- `docs/superpowers/plans/2026-08-24-agent-web-overlay.md` does not exist on any
  branch (checked `git log --all`, origin/main); only the design spec exists.
  Implemented from the task description + design spec instead of the plan's
  Step 1 code block.

# RUNNOTES — OPT-1: expose model, printer, cheats through pkg/gomeboy options

## What changed (pkg/gomeboy only)
- `gomeboy.go`: new public `Model` string type + constants `ModelAuto, ModelDMG0,
  ModelDMG, ModelCGB0, ModelCGB, ModelMGB, ModelSGB, ModelSGB2, ModelAGB`
  (zero value invalid). `WithModel` maps via `modelMap` to `gameboy.AsModel`
  (DMG->DMGABC, CGB->CGBABC, rest 1:1); auto appends nothing (keeps inference).
  Unknown values fail New: `gomeboy: unknown model %q: use auto, DMG0, ...`.
  `WithPrinter` -> `gameboy.WithPrinter`; `WithCheats(path)` -> `gameboy.WithCheats`
  (explicit path only, no cwd probing). New defaults to `model: ModelAuto`.
  New read-only `Emulator.Model()` accessor: maps `gb.Bus.Model()` back to public
  via `publicModelMap` (Unset/nil-Bus -> ModelAuto). No runtime mutation APIs.
- `gomeboy_test.go`: TestWithModel (8 round-trips), TestWithModelAuto (firstwhite
  is a DMG cart -> ModelDMG), TestWithModelUnknown (5 bad values), TestWithPrinter
  + TestNoPrinterByDefault (type-assert Serial.AttachedDevice),
  TestWithCheatsLoadsOnlyExplicitPath (cwd `firstwhite.cheats` NOT picked up),
  TestWithCheatsMalformed (>64KB line -> parser fails, New still succeeds,
  LoadedCheats empty), TestWithCheatsUnreadable, TestNoDiskIOWithModelAndPrinter.

## Gotchas for the next task
- `e.gb.model` is unexported in internal/gameboy; observe the effective model
  via the exported `gb.Bus.Model()` (io.Bus, set by Bus.Map in Init) — that is
  what `Emulator.Model()` uses.
- Tests that `t.Chdir` must resolve `testROM` to an absolute path FIRST
  (helper `absTestROM`); the relative `../../tests/roms/...` breaks after chdir.
- `tests/roms/` is extracted from `tests/roms.zip` by the tests package init;
  I extracted it manually (zip members land in `tests/roms/<suite>/`).
- Cheats errors are log-only (log.Errorf), never returned from New; observable
  via `gb.Bus.LoadedCheats` / `GameSharkCodes` / `GameGenieCodes`.
- Pre-existing (NOT this task): ./tests Test_Acid2/cgb-acid-hell + TestAge
  (missing roms/age) failures; gofmt dirt in pkg/gomeboy/spectate.go,
  cmd/diag/main.go. POKEMON_RED_ROM tests skip when unset.

## Verification (all green)
- `go test ./pkg/gomeboy/ -v` all pass; `-race` clean; gofmt/vet/build clean.
- `go test ./...` green except the pre-existing ./tests failures above.

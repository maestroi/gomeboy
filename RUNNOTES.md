# RUNNOTES — SHIP-1: merge plan 8ce944eb into main (DELIVERED, CI FAILED)

## What was done
- Merged agent-plan/8ce944eb-b848-475b-8b35-d06569b06003 (4e922ba, tasks 1-7)
  into fetched origin/main (25e841d) in a temp worktree; no conflicts,
  expected ancestry (plan is a direct descendant).
- Integration commit: 7c16e777b7e7fb25ba8225a8e2a0ee7827ca340a
  "Merge agent-plan/8ce944eb: improve errors, logging, and options"
- SHIP-GATE passed on the combined tree: go build ./..., go vet ./...,
  go test ./..., git diff --check vs 25e841d. (Had to copy the gitignored
  tests/roms/ fixtures into the temp worktree; untracked, never committed.)
- Pushed NON-FORCE: 25e841d..7c16e77. Fresh fetch confirms origin/main ==
  7c16e77 and plan SHA 4e922ba is an ancestor. Local main fast-forwarded;
  worktree clean.

## BLOCKER: CI failed on the pushed SHA (blocking outcome, NOT fixed here)
- Run: https://github.com/maestroi/gomeboy/actions/runs/32890084106
  job test_regressions -> failure (compile error, exit 1).
- Root cause (reproduced locally): .github/workflows/test.yaml runs
  `go test -tags test -v tests/*.go -run Test_Regressions`. Explicit .go
  file lists BYPASS build constraints, so both files compile together:
  - tests/regressions_test.go:21  (//go:build test)   skipKnownFailures=false
  - tests/skip_known_test.go:9    (//go:build !test)  skipKnownFailures=true
  -> "skipKnownFailures redeclared in this block".
- `go test ./...` (package mode) honors the tags and passes, which is why
  SHIP-GATE was green. The defect is in task 7 (QA-1) test code.

## Next task must do
- Fix the redeclaration so the CI file-list invocation compiles: distinct
  var names, a shared flag file included by both, or change the workflow
  to `go test -tags test ./tests/`. Fix is a NEW commit; do NOT rewrite
  history or force-push.
- Verify CI green for the new main SHA afterwards.
- Note: tests/roms/ is gitignored, so CI has no ROM fixtures; check
  Test_Regressions subprocess behavior on CI after the compile fix.

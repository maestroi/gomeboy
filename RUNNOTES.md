# RUNNOTES — WEB-1: Web is an explicit opt-in for the normal binary

## Changed
- pkg/display/driver.go: GetDriver("auto") no longer returns
  InstalledDrivers[0] (init-order dependent, could be web). New
  autoDriver() picks by fixed autoPriority = ["fyne", "glfw"], then any
  other non-web driver in registration order; webDriverName = "web" is
  never selected. Web-only registry returns nil -> caller's existing
  "unknown display driver" error (no silent network listener).
  Explicit GetDriver("web") lookup unchanged.
- pkg/display/driver_test.go (new): fakeDriver + installForTest
  (saves/restores InstalledDrivers). Tests tagged WEB-AUTO (9
  registration orders incl. web-first, single-desktop-driver cases),
  WEB-DEFAULT-OFF (web-only -> nil), WEB-EXPLICIT (web lookup in 3
  orders + unknown name -> nil).

## Why
Normal binary (main.go) blank-imports fyne, glfw, web; auto used to
follow import/init order. Priority keeps the normal binary's effective
default (fyne, first import) while making it order-independent.

## Verified
- go build ./... && go vet ./... && go test ./... — all green.
  (First full run: pkg/gomeboy failed on missing
  tests/roms/little-things-gb — pre-existing race: tests/rom_test.go
  extracts roms.zip during go test ./... while pkg/gomeboy runs in
  parallel. Passes once roms are extracted; unrelated to this change.)
- Entry points: gomeboy -h -> -driver default "auto" (-web-listen still
  registered); gomeboy-web -h -> default "web"; gomeboy-agent has no
  -driver flag and hardcodes GetDriver("web") (cmd/gomeboy-agent/main.go:144).
- No new flags added; -driver web is the single opt-in.

## Must know
- autoPriority hardcodes "fyne"/"glfw" in pkg/display: add new desktop
  driver names there (or rely on the non-web fallback) when adding
  drivers.
- gomeboy-web -driver auto now errors (web-only registry) — intended.

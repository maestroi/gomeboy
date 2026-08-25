# RUNNOTES — ERR-2: GLFW as a recoverable driver lifecycle

## What changed
- `pkg/display/glfw/glfw.go`: package `init` now ONLY does `runtime.LockOSThread()`
  + `display.Install`. Initialization moved to: `Start` -> `initSubsystems`
  (glfw.Init, primary monitor, sdl.Init, joystick open) -> `run` (createWindow,
  gl.Init AFTER the window — Windows needs a window first) -> `runRenderLoop`.
- Every failure returns `fmt.Errorf("<subsystem>: <op>: %w", err)` ("glfw: initialize",
  "no primary monitor found", "sdl: initialize joystick and video", "glfw: create
  window", "opengl: initialize"). No log.Fatal/panic anywhere.
- Driver tracks started/glfwInited/sdlInited/glInited/monitor/joystick/window
  (mutex-guarded). `Stop` is idempotent: window.Destroy, joystick close, sdl.Quit,
  glfw.Terminate — each only if initialized; clears all state. Start calls Stop on
  every exit path (no leaked subsystems).
- Seams (package func vars, defaults = real calls): glfwInit, glfwTerminate,
  getPrimaryMonitor, sdlInit, sdlQuit, sdlNumJoysticks, sdlJoystickOpen,
  enableJoystickEvents, glInit, createWindow, runRenderLoop.
- New `window` interface + `glfwWindow` adapter; new `joystick` interface +
  `sdlJoystick` adapter (sdl.Joystick is an incomplete C type — cannot be allocated
  from Go, so the adapter is the only way to fake it).
- `keyCallback` extracted as a package function (testable); `getBestMode` became
  `g.bestMode()`; `Rumble` now nil-guards the joystick (pre-existing nil-panic with
  no controller); pollTicker now Stop()ped.
- `pkg/display/glfw/glfw_test.go`: new — GLFW-IMPORT, GLFW-ERRORS, GLFW-CLEANUP, GLFW-BEHAVIOR.

## Gotchas for the next task
- `main.go` never calls driver.Stop(); Start self-cleans now, so behavior is
  unchanged (window close -> Start nil -> gb.Save -> exit).
- GLFW window destroy is `w.Destroy()`, not Close (no Close in this go-gl/glfw
  version). `&glfw.Monitor{}` zero literals are safe in tests as long as bestMode
  isn't called (fullscreen=false in tests).
- Pre-existing (NOT this task): `./tests` failures (Test_Acid2/cgb-acid-hell,
  TestAge panics on missing roms/age fixtures) and gofmt dirt in
  pkg/gomeboy/spectate.go + cmd/diag/main.go. `tests/roms/*.gb` auto-download on
  first run; parallel `go test ./...` can race that (spurious "no such file" on
  first pass — just re-run).

## Verification (all green)
- `go test ./pkg/display/glfw/ -v` 10/10 pass; `-race` clean; gofmt/vet/build clean; full `go test ./...` green except pre-existing ./tests failures.

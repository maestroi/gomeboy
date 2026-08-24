# RUNNOTES — GLFW on-screen display (OSD) for save/load/speed feedback

## What changed
- NEW `pkg/display/glfw/osd.go`: `osd` struct + `newOSD`/`Show`/`Draw`/`compileOSDProgram`.
  Rasterizes short messages with `golang.org/x/image/font/basicfont` (existing dep) into a
  CPU RGBA image, uploads as one texture, draws a single quad via a 460-core shader
  (dark box RGBA{0,0,0,200}, white text). `gl.VertexAttribPointerWithOffset` exists in the
  pinned go-gl (v0.0.0-20260331235117) — no fallback needed.
- `pkg/display/glfw/glfw.go`: `fmt` import; `hud := newOSD()` after `gl.Init()`; F5/F6 show
  "Saved"/"Save failed"/"Loaded"/"Load failed"; NEW +/-/KP+/KP- cases call
  `c.SetSpeed(c.Speed()±1)` + "Speed Nx"; `hud.Draw(w,h)` between blit and SwapBuffers.
- `pkg/emulator/controller.go`: added `Speed() int` + `SetSpeed(int)` to `Controller`.
- `internal/gameboy/gameboy.go`: `speed int` field (`NewGameBoy` sets 1), `Speed()`/`SetSpeed()`
  (clamped 1..8). Only `*gameboy.GameBoy` implements `Controller`.
- `pkg/audio/sdl.go`: `AudioData` steps `gb.Speed()-1` extra frames + drains their APU samples
  so the multiplier actually takes effect (no-op at speed 1).

## Discrepancy with task description (important)
Task assumed F5/F6 AND +/- handlers existed and that `Controller` had `Speed()`/`SetSpeed()`.
NONE of the speed parts existed (only F5/F6 cases were present). I added the interface methods,
the GameBoy speed field, and the audio stepping — same situation as the prior web task.

## Verification
- `go build ./pkg/display/glfw/...` and `go build ./...` succeed; `go vet` clean on touched pkgs.
- Manual (DISPLAY=:0, xdotool+scrot+ffmpeg): F5 -> dark box "Saved" top-left, fades ~1.5s;
  `=` -> "Speed 2x". Confirmed by pixel analysis (box min~55/white text px, ref below=255).
- Window position varies per run (WM placement) — locate the white region before sampling.

## Pre-existing issues (NOT caused by this change; out of scope)
- Intermittent dark/blank window: the UNMODIFIED baseline (clean 498cb92) also goes dark after
  ~15-50s (GL 0x501 after blit). Reproduced with the OSD fully disabled, so it is not the OSD.
- Flaky `Test_LittleThings/tellinglys` and missing `age` ROMs (pre-existing test failures).
- Test harness auto-regenerates README.md/tests/README.md on `go test ./tests/` — keep reverted.

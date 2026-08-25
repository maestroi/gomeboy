# gomeboy

![GitHub go.mod Go version (subdirectory of monorepo)](https://img.shields.io/github/go-mod/go-version/thelolagemann/gomeboy)

GomeBoy is my attempt at creating a fairly accurate and reasonably performant Game Boy emulator written with golang. It
is still currently in the very early stages of development, but it is already capable of running quite a few games with
varying degrees of success.

---

## Screenshots

### DMG Games

<img src="assets/images/tetris.png" width="250"> <img src="assets/images/super-mario-land2.png" width="250"> <img src="assets/images/pokemon-red.png" width="250">

### DMG Games running on CGB hardware

<img src="assets/images/tetris-cgb.png" width="250"> <img src="assets/images/super-mario-land2-cgb.png" width="250"> <img src="assets/images/pokemon-red-cgb.png" width="250">

### CGB Games

<img src="assets/images/tetris-dx.png" width="250"> <img src="assets/images/mario-tennis.png" width="250"> <img src="assets/images/pokemon-crystal.png" width="250">

### Peripherals (Printer)

![Printer](assets/images/printer.gif)

---

## Features


- GameBoy (DMG) and GameBoy Color (CGB) support
- SRAM and RTC support
- Run DMG games with CGB colorization palettes (without using a boot ROM)
- Automated testing against a large number of test ROMs
- Peripherals
	- Cartridge Mappers
      - MBC1
      - MBC2
      - MBC3
      - MBC5
      - ROM
  - Cheat Carts
    - Game Genie
    - GameShark
  - Serial
    - Printer
    - Link Cable
    - Local Multiplayer (needs reimplementation)
- Platform-agnostic (runs on Windows, Linux, and Mac)
- Desktop (Fyne/GLFW), web spectator, and a headless Go library
- Save/load and 1x–8x speed on desktop and web
- Optional agent overlay that publishes goal/action/observation to the web client

---

## Usage

Requires **Go 1.26+**. Desktop drivers need the usual Fyne/GLFW system libraries; the web binaries do not.

### Desktop

```sh
go run . -rom game.gb
go run . -rom game.gb -driver fyne   # or glfw, web
```

| Flag | Default | Description |
| --- | --- | --- |
| `-rom` | | Path to a `.gb` / `.gbc` ROM |
| `-boot` | | Optional boot ROM (`.gbr`) |
| `-model` | `auto` | `auto`, `dmg`, or `cgb` |
| `-printer` | `false` | Enable the Game Boy Printer |
| `-driver` | `auto` | `auto`, `glfw`, `fyne`, or `web` |

Battery saves (`.sav`) and quick-save states (`.state`) are written next to the process working directory, named after the ROM.

| Action | Fyne | GLFW | Web |
| --- | --- | --- | --- |
| Quick save | F5, or Emulation → Quick Save | F5 | Save |
| Quick load | F6, or Emulation → Quick Load | F6 | Load |
| Speed 1x–8x | F7 / F8, or Emulation → Speed | `+` / `-` | 1x / 2x / 4x |

Turbo speed mutes audio rather than pitching it up.

### Web player

Headless web binary (no Fyne/GLFW). It drives the emulator at 60 Hz and serves the WebSocket hub on **:8090**.

```sh
go run ./cmd/gomeboy-web -rom game.gb
```

The Svelte UI is served from `GOMEBOY_WEB_STATIC_DIR` at `/app/` (the Docker image sets this). The websocket URL is `ws://<host>:8090/`.

```sh
docker build -t gomeboy-web:latest .
```

Swarm deploy notes, ROM volume, and open questions live in [`deploy/README.md`](deploy/README.md).

### Agent spectator

`gomeboy-agent` runs `pkg/gomeboy` under a stub agent loop and publishes frames plus agent state (goal / last action / observation / status) to the same web client. The web UI never steps the emulator.

```sh
go run ./cmd/gomeboy-agent -rom game.gb
# optional: -fps 60
```

Open the web UI on :8090. The agent panel shows the stub loop; a real decision loop is not wired yet. Browser button presses also reach the emulator (no arbitration between human and agent).

---

## Library

GomeBoy can be used as a headless Go library for programmatic, in-process emulation. The core has no GUI, no audio device, and no realtime throttling — you advance it deterministically frame by frame. This makes it suitable for tooling, testing, and AI/agent workloads that need to drive a Game Boy and observe its state.

```go
import "github.com/thelolagemann/gomeboy/pkg/gomeboy"

// Create an emulator and load a ROM.
emu, err := gomeboy.New(
    gomeboy.WithROM("game.gb"),
    gomeboy.Headless(), // don't accumulate audio samples
)
if err != nil {
    log.Fatal(err)
}
defer emu.Close()

// Feed input and advance one frame at a time.
emu.Press(gomeboy.ButtonA)
emu.StepFrame()
emu.Release(gomeboy.ButtonA)

// Read the rendered frame (160x144, 24-bit RGB).
frame := emu.Frame()
// frame.RGB is 160*144*3 bytes, row-major.

// Read memory directly.
byte := emu.Read8(0xC140) // e.g. the LY register
```

### API

| Method | Description |
| --- | --- |
| `New(opts ...Option) (*Emulator, error)` | Create an emulator. `WithROM(path)` loads a ROM up front. |
| `WithROM(path) Option` | Load and initialize a ROM at construction. |
| `WithROMBytes(rom []byte) Option` | Load an in-memory ROM image. |
| `WithBootROM(path) Option` | Use a boot ROM (`.gbr`) instead of the HLE boot process. |
| `WithSaveDir(dir) Option` | Enable `.sav` / `.state` persistence in `dir`. Off by default (no disk I/O). |
| `Headless() Option` | Disable APU sample accumulation (prevents memory growth when running long). |
| `LoadROM(path) error` / `LoadROMBytes(rom, name) error` | Load a ROM and (re)initialize. |
| `Press(b Button)` / `Release(b Button)` | Press or release a joypad button. |
| `StepFrame()` | Advance by exactly one frame. |
| `StepFrames(n int)` | Advance by `n` frames. |
| `FrameCount() uint64` / `Cycle() uint64` | Frames advanced and master-clock cycle. |
| `Read8(addr uint16) byte` / `Read(addr uint16, n int) []byte` | CPU-accurate reads (DMA / PPU locks apply). |
| `Peek8` / `Peek16` / `PeekInto` | Side-effect-free observation of memory. |
| `Frame() Frame` | The most recently rendered frame (zero-copy view). |
| `Image() image.Image` / `PNG() ([]byte, error)` / `WritePNG(w)` | Copied frame as `image.Image` or PNG. Safe to keep after the next step. |
| `Reset() error` | Return to the boot state, reusing the loaded ROM. Battery RAM is preserved. |
| `SaveState() ([]byte, error)` / `LoadState([]byte) error` | Serialize / restore the full emulator state. |
| `QuickSave() error` / `QuickLoad() error` | Write / restore `<romname>.state`. |
| `Close() error` | Flush battery-backed save data and release resources. |

### Buttons

`ButtonA`, `ButtonB`, `ButtonStart`, `ButtonSelect`, `ButtonUp`, `ButtonDown`, `ButtonLeft`, `ButtonRight`.

### Framebuffer ownership

`Frame().RGB` is a **zero-copy view** into the emulator's internal frame buffer. It is valid only until the next call to `StepFrame` or `StepFrames`. Copy the bytes if you need to keep them. `Image()` / `PNG()` return copies.

### Save states

`SaveState`/`LoadState` capture and restore the *entire* deterministic execution state (CPU, scheduler, bus, cartridge, PPU, APU, timer, serial). Restoring a state on an emulator running the same ROM resumes from the exact same point. Save states are not compatible across different ROMs.

### Concurrency

A single `Emulator` is **not** safe for concurrent use by multiple goroutines — the caller must synchronize. Multiple `Emulator` instances are fully independent and may be created and run in the same process.

### HTTP spectator

`Spectator` serves a read-only PNG of the current frame. It does not accept input. Call `Capture` after each step you want viewers to see.

```go
spec := gomeboy.NewSpectator()
http.ListenAndServe(":8080", spec.Handler())
// after each StepFrame:
_ = spec.Capture(emu)
```

`GET /` is a small auto-refreshing page; `GET /frame.png` is the last captured frame.

---

# Automated Test Results


![progress](https://progress-bar.xyz/90/?scale=100&title=passing%20227,%20failing%2025&width=500)

| Test Suite | Pass Rate | Tests Passed | Tests Failed | Tests Total |
| --- | --- | --- | --- | --- |
| acid2 | 75% | 3 | 1 | 4 |
| bully | 50% | 1 | 1 | 2 |
| blarrg | 100% | 43 | 0 | 43 |
| little-things-gb | 75% | 3 | 1 | 4 |
| mooneye | 99% | 113 | 1 | 114 |
| samesuite | 75% | 59 | 19 | 78 |
| scribbltests | 100% | 5 | 0 | 5 |
| strikethrough | 0% | 0 | 2 | 2 |

<sup>Visit the [tests](tests/README.md) directory for more information.</sup>

---

# TODO

- [x] build instructions
- [x] github actions
- [ ] improve error handling and logging
- [ ] expose more emulator options to the user
- [ ] reimplement link cable & local multiplayer
- [ ] real agent decision loop (the current `gomeboy-agent` loop is a stub)

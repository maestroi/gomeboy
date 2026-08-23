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
| `WithBootROM(path) Option` | Use a boot ROM (`.gbr`) instead of the HLE boot process. |
| `Headless() Option` | Disable APU sample accumulation (prevents memory growth when running long). |
| `LoadROM(path) error` | Load a ROM and (re)initialize. |
| `Press(b Button)` / `Release(b Button)` | Press or release a joypad button. |
| `StepFrame()` | Advance by exactly one frame. |
| `StepFrames(n int)` | Advance by `n` frames. |
| `Read8(addr uint16) byte` / `Read(addr uint16, n int) []byte` | Read from the 16-bit address space. |
| `Frame() Frame` | The most recently rendered frame (zero-copy view). |
| `Reset() error` | Return to the boot state, reusing the loaded ROM. Battery RAM is preserved. |
| `SaveState() ([]byte, error)` / `LoadState([]byte) error` | Serialize / restore the full emulator state. |
| `Close() error` | Flush battery-backed save data and release resources. |

### Buttons

`ButtonA`, `ButtonB`, `ButtonStart`, `ButtonSelect`, `ButtonUp`, `ButtonDown`, `ButtonLeft`, `ButtonRight`.

### Framebuffer ownership

`Frame().RGB` is a **zero-copy view** into the emulator's internal frame buffer. It is valid only until the next call to `StepFrame` or `StepFrames`. Copy the bytes if you need to keep them.

### Save states

`SaveState`/`LoadState` capture and restore the *entire* deterministic execution state (CPU, scheduler, bus, cartridge, PPU, APU, timer, serial). Restoring a state on an emulator running the same ROM resumes from the exact same point. Save states are not compatible across different ROMs.

### Concurrency

A single `Emulator` is **not** safe for concurrent use by multiple goroutines — the caller must synchronize. Multiple `Emulator` instances are fully independent and may be created and run in the same process.

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

- [ ] build instructions
- [ ] github actions
- [ ] improve error handling and logging
- [ ] expose more emulator options to the user
- [ ] reimplement link cable & local multiplayerr
# GomeBoy

![Go version](https://img.shields.io/github/go-mod/go-version/maestroi/gomeboy)

GomeBoy is a Game Boy and Game Boy Color emulator written in Go. It can be used as a desktop emulator or an in-process library for deterministic automation, testing, search, and AI/agent workloads.

The core is designed to keep hardware-visible behavior accurate while also exposing fast paths for workloads that do not need realtime audio or RGB framebuffer output.

## Highlights

- Game Boy (DMG) and Game Boy Color (CGB) emulation
- GLFW desktop frontend
- Read-only HTTP spectator UI for library users
- Deterministic Go library with frame and instruction stepping
- Fast `Headless()` audio path and optional `WithoutVideo()` rendering path
- Allocation-free bulk memory observation with `PeekInto`
- Fast in-process checkpoints for search and branching
- Independent `Fork()` execution branches
- Frame- and instruction-granular `RunUntil` / watchpoint APIs
- CPU and PPU debugger state without copying a full save state
- Bounded flight recorder for inputs, frame samples, and selected RAM changes
- Deterministic input recording and replay
- Durable `.gbrun` session recordings with verified replay and later RGB regeneration
- `gomeboy-stream` CLI for FFmpeg MP4/RTMP encoding and `.gbrun` replay-to-video
- Causal `memprobe` experiments for agent reverse-engineering
- State hashing for replay and branch-equivalence checks
- Checked durable save states with ROM identity, model, version, frame/cycle metadata, and payload integrity
- SRAM, RTC, cartridge mappers, Game Genie / GameShark, printer, and serial support
- Automated regression testing against established Game Boy test ROM suites

---

## Screenshots

### DMG games

<img src="assets/images/tetris.png" width="250"> <img src="assets/images/super-mario-land2.png" width="250"> <img src="assets/images/pokemon-red.png" width="250">

### DMG games on CGB hardware

<img src="assets/images/tetris-cgb.png" width="250"> <img src="assets/images/super-mario-land2-cgb.png" width="250"> <img src="assets/images/pokemon-red-cgb.png" width="250">

### CGB games

<img src="assets/images/tetris-dx.png" width="250"> <img src="assets/images/mario-tennis.png" width="250"> <img src="assets/images/pokemon-crystal.png" width="250">

### Game Boy Printer

![Printer](assets/images/printer.gif)

---

## Emulator features

### Hardware and cartridge support

- DMG and CGB hardware models, plus model selection for `DMG0`, `DMG`, `CGB0`, `CGB`, `MGB`, `SGB`, `SGB2`, and `AGB`
- HLE boot process or optional boot ROM
- DMG games with CGB colorization palettes
- SRAM and RTC persistence
- Cartridge mapper support including ROM, MBC1, MBC2, MBC3, MBC5, MBC7, HuC1, M161, and Pocket Camera-related paths present in the core
- Game Genie and GameShark cheats
- Game Boy Printer
- Serial/link infrastructure

### Frontends

- GLFW desktop frontend
- Read-only HTTP spectator for library users

### Deterministic execution

- `StepFrame()` for one deterministic emulated frame
- `StepFrames(n)` for batched stepping with one core lock acquisition
- `StepInstruction()` for debugger-grade SM83 stepping
- `FrameCount()` and `Cycle()` counters
- Fixed WRAM boot randomization seed for reproducible headless runs
- Independent emulator instances with per-instance APU filter state

---

## Performance

GomeBoy includes fast paths specifically for headless simulation and agent/search workloads.

The measurements below were sampled five times with a one-second benchmark window on an AMD Ryzen 9 7950X (32 logical CPUs) with Go 1.27.0. These numbers describe **emulation throughput**, not the Game Boy display refresh rate; absolute numbers will vary by machine, but the relative video on/off and checkpoint/save-state gaps should hold across hardware.

### Video output on vs off

| Workload | Video on | `WithoutVideo()` | Improvement |
| --- | ---: | ---: | ---: |
| Single headless frame | 359.089 µs | 332.199 µs | 7.49% less time |
| Simulated throughput | ~2,785 frames/s | ~3,010 frames/s | **+8.10%** |
| 60-frame batch | 21.732 ms | 19.808 ms | 8.86% less time |
| 60-frame simulated throughput | ~2,761 frames/s | ~3,029 frames/s | **+9.71%** |

Both frame paths remained at **0 B/op and 0 allocs/op** in these benchmarks.

`WithoutVideo()` does not disable the PPU. Fetchers, FIFOs, scanline timing, interrupts, VRAM/OAM locking, and other hardware-visible behavior continue to run. It skips output-only RGB palette composition and framebuffer writes.

For an unthrottled controller or search worker, the measured configuration:

```go
emu, err := gomeboy.New(
    gomeboy.WithROM("game.gb"),
    gomeboy.Headless(),
    gomeboy.WithoutVideo(),
)
```

simulated roughly **3k Game Boy frames per wall-clock second** on that machine.

### Fast checkpoints

| Round trip | Median | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| `CheckpointInto` + `RestoreCheckpoint` | 16.052 µs | 0 | 0 |
| `SaveState` + `LoadState` | 4.720 ms | ~4,091,664 | 24,991 |

The process-local checkpoint path is approximately **294× faster** than serialized save/load, with about **99.66% lower latency**, and allocates nothing per round trip versus ~3.9 MiB for a serialized round trip.

Use checkpoints for search trees, rollback, planning, and repeated local branching. Use serialized or checked save states when the state needs to survive outside the current process.

### Memory observation

| Operation | 4 KiB median | Allocations | Intended use |
| --- | ---: | ---: | --- |
| `PeekInto` | ~28.15 ns | 0 | Fast side-effect-free observation |
| `ReadInto` | 6.350 µs | 0 | CPU-accurate reads with bus semantics |

`PeekInto` ignores DMA conflicts, PPU locks, and lazy IO side effects, so it runs roughly **225× faster** than `ReadInto` and is the preferred observation API for agents. `ReadInto` should be used when the caller specifically needs CPU-visible bus behavior.

### Headless execution improvements

The headless path also removes the 96 kHz audio-output sampling event while keeping hardware-visible APU state evolving. Batched `StepFrames(n)` avoids taking the emulator mutex once per frame. Both changes reduce overhead without changing the emulated program-visible state.

---

## Requirements

Requires **Go 1.26+**.

The GLFW driver needs the usual GLFW/OpenGL/SDL2 system libraries for your platform. The headless library path (`pkg/gomeboy`) does not require a desktop windowing stack.

---

## Desktop usage

```sh
go run . -rom game.gb
```

| Flag | Default | Description |
| --- | --- | --- |
| `-rom` | | Path to a `.gb` / `.gbc` ROM |
| `-boot` | | Optional boot ROM (`.gbr`) |
| `-model` | `auto` | `auto`, `DMG0`, `DMG`, `CGB0`, `CGB`, `MGB`, `SGB`, `SGB2`, or `AGB` |
| `-printer` | `false` | Attach the Game Boy Printer |
| `-cheats` | | Explicit GameShark / Game Genie cheat file |
| `-save-dir` | working directory | Directory for `.sav` / `.state` files |
| `-no-saves` | `false` | Disable save-file I/O |
| `-log-level` | `info` | `debug`, `info`, or `error` |
| `-pprof` | disabled | `host:port` for `net/http/pprof` |
| `-driver` | `auto` | `auto` or `glfw` (the only installed driver) |
| `-fullscreen` | `false` | Start in fullscreen |
| `-scale` | `1` | Window scale factor |

| Action | GLFW |
| --- | --- |
| Quick save | F5 |
| Quick load | F6 |
| Speed | `+` / `-` |
| Fullscreen | F11 |

Turbo speed mutes audio instead of pitching it up.

Battery saves and quick-save states are named after the ROM and written to the working directory or `-save-dir`. If `<romname>.cheats` exists in the working directory it is loaded automatically; `-cheats` loads an explicit file.

---

## Utilities

`gomeboy-stream` encodes live RGB24 frames or a `.gbrun` recording through FFmpeg (MP4 or RTMP). FFmpeg must be on `PATH`, or pass `-ffmpeg`.

```sh
go run ./cmd/gomeboy-stream -rom game.gb -recording run.gbrun -output run.mp4
```

`memprobe` runs checkpoint-and-compare memory experiments and prints JSON diffs:

```sh
go run ./cmd/memprobe -rom game.gb
```

See [docs/RECORDINGS.md](docs/RECORDINGS.md), [docs/STREAMING.md](docs/STREAMING.md), and [docs/MEMPROBE.md](docs/MEMPROBE.md).

---

## Go library

> This fork currently retains the upstream module path `github.com/maestroi/gomeboy`, so library imports use that path.

```go
import "github.com/maestroi/gomeboy/pkg/gomeboy"

emu, err := gomeboy.New(
    gomeboy.WithROM("game.gb"),
    gomeboy.Headless(),
    gomeboy.WithoutVideo(), // omit this when RGB frames are needed
)
if err != nil {
    log.Fatal(err)
}
defer emu.Close()

emu.Press(gomeboy.ButtonA)
emu.StepFrame()
emu.Release(gomeboy.ButtonA)

var observation [4096]byte
emu.PeekInto(0xC000, observation[:])
```

### Construction and execution

| API | Purpose |
| --- | --- |
| `New(opts ...Option)` | Create an emulator |
| `WithROM(path)` / `WithROMBytes(rom)` | Load ROM from disk or memory |
| `WithBootROM(path)` | Use a boot ROM instead of HLE boot |
| `WithSaveDir(dir)` | Enable disk-backed save/state persistence |
| `Headless()` | Disable output audio sampling while preserving APU-visible behavior |
| `WithoutVideo()` | Disable RGB generation while preserving PPU-visible behavior |
| `StepFrame()` | Step one emulated frame |
| `StepFrames(n)` | Batch multiple frames |
| `StepInstruction()` | Execute one SM83 instruction or interrupt-service step |
| `FrameCount()` / `Cycle()` | Read deterministic execution counters |
| `Reset()` | Return to boot state while preserving battery RAM |

### Memory and observation

| API | Purpose |
| --- | --- |
| `Peek8`, `Peek16`, `PeekInto` | Fast side-effect-free observation |
| `SnapshotMemory(dst)` | Copy the complete 64 KiB address space into caller-owned storage |
| `Read8`, `Read`, `ReadInto` | CPU-accurate reads with DMA/PPU/IO semantics |
| `CPUState()` | Compact CPU registers and execution state |
| `PPUState()` | Compact PPU timing/fetch state without framebuffer copying |
| `TrackMemory(range)` / `ChangesSince(tracker)` | Reusable selected-RAM diffing |
| `Frame()` | Zero-copy view of the latest RGB framebuffer |
| `Image`, `PNG`, `WritePNG` | Copied image output |

### Run-until conditions and watchpoints

Frame-granular execution:

```go
stop, err := emu.RunUntil(10_000,
    gomeboy.MemoryEquals(0xC123, 1),
    gomeboy.PCEquals(0x4000),
    gomeboy.InterruptPending(),
)
```

Instruction-granular debugging/watchpoints:

```go
stop, err := emu.RunCPUUntil(
    1_000_000,
    gomeboy.MemoryWatchpoint(0xC123),
)
```

Built-in conditions include:

- `MemoryEquals`
- `MemoryMaskedEquals`
- `MemoryChanged` / `MemoryWatchpoint`
- `PCEquals`
- `FrameAtLeast`
- `CycleAtLeast`
- `InterruptPending`
- `Any`
- `All`
- caller-defined `ConditionFunc`

`RunUntil` checks at frame boundaries. `RunCPUUntil` checks after debugger CPU steps and is intended for fine-grained diagnosis rather than maximum throughput. `MemoryWatchpoint` detects observed value changes; writing the same value does not count as a change.

Full semantics and examples are documented in [`docs/AGENT-TOOLING.md`](docs/AGENT-TOOLING.md).

### Checkpoints and branching

For low-overhead rollback on one emulator:

```go
var cp gomeboy.Checkpoint
emu.CheckpointInto(&cp)

// explore a branch
emu.Press(gomeboy.ButtonRight)
emu.StepFrames(30)

emu.RestoreCheckpoint(&cp)
```

For independent branches:

```go
left, err := emu.Fork()
if err != nil {
    log.Fatal(err)
}
defer left.Close()

right, err := emu.Fork()
if err != nil {
    log.Fatal(err)
}
defer right.Close()
```

A fork receives independent mutable CPU, bus/cartridge, PPU, APU, timer, scheduler, and serial state while reusing the immutable ROM image. Headless/no-video execution policies are preserved.

### Deterministic input recording and replay

```go
emu.StartInputRecording()
emu.Press(gomeboy.ButtonA)
emu.StepFrame()
emu.Release(gomeboy.ButtonA)
emu.StepFrame()
events := emu.StopInputRecording()

if err := replay.ReplayInputs(events, finalFrame); err != nil {
    log.Fatal(err)
}
```

Recorded events are replayed at frame boundaries. Cycle information is retained for diagnostics.

### Flight recorder

The bounded flight recorder is opt-in and can retain recent input transitions, frame samples, and selected memory changes.

```go
err := emu.EnableFlightRecorder(gomeboy.FlightRecorderOptions{
    Capacity:     4096,
    RecordFrames: true,
    Memory: []gomeboy.MemoryRange{
        {Start: 0xC000, Length: 0x2000},
    },
})
if err != nil {
    log.Fatal(err)
}

// ... execute ...
events := emu.DisableFlightRecorder()
```

Memory changes are sampled after frame or instruction boundaries. The recorder is intentionally not presented as a cycle-accurate trace of every internal bus write.

When a recorder requests per-frame or memory sampling, `StepFrames` takes the per-frame path so requested observations are not skipped. An input-only recorder leaves batched stepping intact.

### Save states

GomeBoy now has three state mechanisms for different jobs:

| Mechanism | Best for | Characteristics |
| --- | --- | --- |
| `CheckpointInto` / `RestoreCheckpoint` | Search and rollback | Fastest, opaque, process-local |
| `SaveState` / `LoadState` | Raw serialized state | Full emulator state, same-ROM responsibility is on caller |
| `SaveStateChecked` / `LoadStateChecked` | Durable checkpoints / bug reports | Adds format version, ROM SHA-256, model, frame/cycle, build metadata, and payload checksum |

Checked states can be inspected without restoring:

```go
data, err := emu.SaveStateChecked()
if err != nil {
    log.Fatal(err)
}

meta, err := gomeboy.InspectCheckedState(data)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("ROM=%s frame=%d cycle=%d\n", meta.ROMSHA256, meta.Frame, meta.Cycle)
```

`LoadStateChecked` rejects corrupted payloads, unsupported envelope versions, wrong-ROM states, and hardware-model mismatches before restoring.

### Determinism hashes

```go
hash, err := emu.StateHashHex()
if err != nil {
    log.Fatal(err)
}
fmt.Println(hash)
```

`StateHash` fingerprints deterministic execution state and intentionally excludes the RGB framebuffer because that buffer is output-only and may be disabled with `WithoutVideo()`.

This is useful for replay verification, worker-to-worker determinism checks, and confirming that two branches reached the same state.

### Buttons

`ButtonA`, `ButtonB`, `ButtonStart`, `ButtonSelect`, `ButtonUp`, `ButtonDown`, `ButtonLeft`, `ButtonRight`.

### Framebuffer ownership

`Frame().RGB` is a zero-copy view of the internal framebuffer and is overwritten by later rendering. Copy it if it must outlive the next frame. `Image()` and PNG helpers return copied output.

With `WithoutVideo()` enabled, the framebuffer is not regenerated and `Frame()` returns the last framebuffer contents.

### Concurrency

A single `Emulator` is not safe for unsynchronized concurrent use. Multiple emulator instances and `Fork()` branches are independent and can run concurrently when the caller manages their goroutines normally.

---

## HTTP spectator

The library spectator serves a read-only snapshot of the most recently captured frame.

```go
spec := gomeboy.NewSpectator()
http.ListenAndServe(":8080", spec.Handler())

// after a frame you want viewers to see:
_ = spec.Capture(emu)
```

- `GET /` serves a small auto-refreshing page
- `GET /frame.png` serves the most recently captured frame

---

## Benchmarking

The performance benchmarks use a self-contained synthetic ROM and do not depend on gitignored test ROM fixtures.

```sh
go test ./pkg/gomeboy \
  -run '^$' \
  -bench '^BenchmarkPerf' \
  -benchmem \
  -benchtime=1s \
  -count=5
```

Useful benchmark groups include headless frame stepping, no-video stepping, `StepFrames(60)`, `PeekInto`, `ReadInto`, checkpoint round trips, and serialized save-state round trips.

---

## Accuracy and regression testing

The repository runs automated regression tests against a broad set of Game Boy test ROM suites.

![progress](https://progress-bar.xyz/90/?scale=100&title=passing%20228,%20failing%2024&width=500)

| Test suite | Pass rate | Passed | Failed | Total |
| --- | ---: | ---: | ---: | ---: |
| acid2 | 75% | 3 | 1 | 4 |
| bully | 50% | 1 | 1 | 2 |
| blarrg | 100% | 43 | 0 | 43 |
| little-things-gb | 100% | 4 | 0 | 4 |
| mooneye | 99% | 113 | 1 | 114 |
| samesuite | 75% | 59 | 19 | 78 |
| scribbltests | 100% | 5 | 0 | 5 |
| strikethrough | 0% | 0 | 2 | 2 |

See [`tests/README.md`](tests/README.md) for suite details.

Agent/debug additions also have self-contained synthetic-ROM tests covering instruction/frame stepping equivalence, watchpoints, replay determinism, branching, checked-state validation, video/no-video execution equivalence, memory tracking, checkpoints, and allocation-conscious read APIs.

---

## Project status and known limitations

GomeBoy is actively evolving. The core already supports desktop play, headless/library use, and agent/debug tooling, but some areas are intentionally still incomplete:

- Link cable / local multiplayer support needs reimplementation before it should be considered production-ready.
- There is no bundled web player or agent-spectator binary; use the library APIs in `pkg/gomeboy` directly, including `NewSpectator()` for a read-only HTTP frame feed.
- Some external test suites still contain known failures; the current table above documents the repository's measured status rather than claiming complete hardware accuracy.

For automation and agent workloads, prefer the library APIs in `pkg/gomeboy` and the detailed guide in [`docs/AGENT-TOOLING.md`](docs/AGENT-TOOLING.md).

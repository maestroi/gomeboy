# Generic introspection API

GomeBoy exposes a small game-agnostic inspection surface for headless automation, debugging, and reverse engineering. The emulator provides hardware state and execution primitives; downstream tools are responsible for assigning game-specific meaning to addresses, values, maps, encounters, scripts, or other data.

The APIs in this document are usable from `pkg/gomeboy` without Fyne, GLFW, or the Web UI.

## Observation contract

`Peek8`, `Peek16`, and `PeekInto` are debugger-style observations of the currently mapped address space. They read raw bus state with `Bus.Get`/`Bus.CopyFrom` semantics and do not evaluate CPU-visible DMA conflicts, PPU region locks, or lazy IO readers.

`Read8`, `Read`, and `ReadInto` are different: they model what the emulated CPU can observe at that instant. Their results may therefore be affected by DMA conflicts, PPU locks, cartridge behavior, and lazy IO register evaluation.

Use `Peek*` for external automation and reverse engineering when the goal is to inspect emulator state without perturbing it. Use `Read*` only when CPU-visible bus behavior is itself the thing being studied.

`SnapshotMemory` copies the complete 64 KiB currently mapped address space into caller-owned storage. `TrackMemory` and `ChangesSince` provide reusable byte-diff tracking over a selected address range.

All of these APIs follow the `Emulator` concurrency contract: one emulator instance is not safe for concurrent use. A caller must not step an emulator concurrently with an inspection call.

## ROM and cartridge identity

`ROM()` returns a caller-owned copy of the exact ROM image loaded by the emulator. Mutating the returned slice cannot change the running emulator.

`ROMSHA256()` returns a stable SHA-256 fingerprint of that loaded image.

`Cartridge()` returns `CartInfo`, a value type containing cartridge metadata such as title, mapper/cartridge type, ROM/RAM sizes, CGB/SGB flags, checksums, and battery/RTC/rumble capabilities where available. It does not expose mutable mapper internals. Before a ROM is loaded, `Cartridge()` returns the zero value.

These APIs let a controller identify the exact game revision it is driving without reopening the ROM file.

## Included reverse-engineering primitives

The current public API intentionally favors small composable primitives over a debugger UI:

- `CPUState()` and `PPUState()` provide compact read-only execution snapshots.
- `StepInstruction()` provides debugger-granular SM83 stepping.
- `PCEquals` with `RunCPUUntil` provides execution/PC breakpoints at instruction boundaries.
- `MemoryWatchpoint` with `RunCPUUntil` detects observed byte-value changes at instruction boundaries.
- `RunUntil` provides lower-overhead frame-boundary conditions for automation workloads.
- `SnapshotMemory`, `TrackMemory`, and `ChangesSince` support full snapshots and selected-range byte diffs.
- `FlightRecorder` can retain bounded frame/instruction samples, inputs, and selected memory changes for diagnostics.

`MemoryWatchpoint` is intentionally an observed-state watchpoint, not an attempted-write trap. Writing the same value does not trigger it, and multiple writes inside one debugger step are not individually surfaced.

## Deferred primitives

The following are deliberately not part of the initial generic introspection surface:

### Attempted-write bus hooks

Cycle-accurate callbacks for every attempted memory write are deferred. Adding them in the core would put a callback on a very hot path and needs a design that keeps ordinary emulation allocation-free and effectively zero-cost when disabled.

The existing `MemoryWatchpoint` and memory trackers cover the common automation/debugging case without changing the bus write path.

### Bank-qualified inactive WRAM/VRAM inspection

`Peek*` and `SnapshotMemory` expose the address space exactly as it is currently mapped. On CGB hardware that means the currently selected WRAM/VRAM banks are visible through their normal CPU address windows.

A public API for directly reading an inactive CGB WRAM or VRAM bank is deferred. If added, it should expose copied/value data without leaking mutable `internal/io` storage or requiring callers to temporarily change bank-selection registers.

### Cycle-accurate trace callbacks

A general instruction/bus trace callback API is deferred. `StepInstruction`, `RunCPUUntil`, `CPUState`, `PPUState`, and the bounded `FlightRecorder` provide deterministic diagnostic building blocks without imposing tracing overhead on normal execution.

### Game-specific schemas

Game concepts such as Pokémon species, maps, badges, trainers, encounter tables, scripts, or save layouts do not belong in GomeBoy. They should remain in PokéPilot or other downstream game profiles built on top of these generic primitives.

## Why this boundary

This surface is intended to make support for additional GB/GBC games primarily a downstream profile problem while keeping GomeBoy a reusable emulator. Observation remains side-effect-free, headless use remains first-class, and the public package does not expose mutable `internal/io` implementation types.

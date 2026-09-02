# Agent and debugger tooling

GomeBoy exposes generic, opt-in primitives for automation, deterministic replay, branching, and failure diagnostics. These APIs deliberately know nothing about a particular game: callers decide which addresses, PCs, or state transitions mean “battle”, “dialogue”, “controllable”, and so on.

## Run until a condition

`RunUntil` advances frame by frame and checks one or more side-effect-free conditions before the first frame and after each stepped frame.

```go
stop, err := emu.RunUntil(10_000,
    gomeboy.MemoryChanged(0xC123),
    gomeboy.MemoryEquals(0xC456, 1),
)
if err != nil {
    log.Fatal(err)
}
if stop.Matched {
    log.Printf("stopped on %s at frame %d cycle %d", stop.Condition, stop.Frame, stop.Cycle)
}
```

Built-in conditions include `MemoryEquals`, `MemoryMaskedEquals`, `MemoryChanged`, `PCEquals`, `CycleAtLeast`, `Any`, and `All`. `ConditionFunc` lets a caller build a game-specific predicate without putting game semantics into GomeBoy.

`RunUntil` is frame-granular. Use `StepInstruction` when instruction-level diagnostics are required.

## Debugger state and instruction stepping

`CPUState` and `PPUState` return compact read-only state without copying the framebuffer or the full emulator snapshot.

```go
cpu := emu.CPUState()
ppu := emu.PPUState()
fmt.Printf("PC=%04X SP=%04X LY=%d mode=%d\n", cpu.PC, cpu.SP, ppu.LY, ppu.Mode)

step := emu.StepInstruction()
fmt.Printf("%04X -> %04X opcode=%02X cycles=%d\n",
    step.PCBefore, step.PCAfter, step.Opcode, step.Cycles)
```

A debugger step normally executes one SM83 opcode. If an interrupt is pending before the next fetch, servicing that interrupt is the step instead and `Interrupt` is true. HALT may advance scheduler time until the CPU can make progress; `Frames` reports display-frame boundaries crossed by the step.

## Memory change tracking

`TrackMemory` snapshots a selected, non-wrapping address range with side-effect-free reads. `ChangesSince` returns byte differences and then advances the baseline.

```go
tracker, err := emu.TrackMemory(gomeboy.MemoryRange{Start: 0xC000, Length: 0x2000})
if err != nil {
    log.Fatal(err)
}

emu.StepFrames(60)
for _, change := range emu.ChangesSince(tracker) {
    fmt.Printf("%04X: %02X -> %02X at frame %d\n",
        change.Address, change.Before, change.After, change.Frame)
}
```

The tracker reuses its snapshot buffers. The returned change slice is caller-owned.

## Bounded flight recorder

The flight recorder is disabled by default. When enabled it keeps only the newest configured number of events and can record input transitions, frame samples, and sampled memory changes.

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

// ... run the emulator ...

events := emu.DisableFlightRecorder()
```

Memory ranges are compared after `StepFrame` and `StepInstruction`. This records observed state changes at those sampling boundaries; it is not a cycle-accurate log of every internal bus write. If a recorder requests frame or memory sampling, `StepFrames` intentionally takes the per-frame path so no requested sample is skipped. An input-only recorder leaves batched stepping intact.

## Deterministic input recording and replay

Input recording stores frame-boundary button transitions.

```go
emu.StartInputRecording()
emu.Press(gomeboy.ButtonA)
emu.StepFrame()
emu.Release(gomeboy.ButtonA)
emu.StepFrame()
events := emu.StopInputRecording()
```

Replay the events from the same reset/checkpoint state:

```go
if err := replay.ReplayInputs(events, finalFrame); err != nil {
    log.Fatal(err)
}
```

An event at frame `F` is applied immediately before frame `F` is stepped. `Cycle` is retained in each `InputEvent` for diagnostics; replay is intentionally frame-granular.

## Forking execution

`Fork` creates an independent emulator at the exact current execution state.

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

left.Press(gomeboy.ButtonLeft)
right.Press(gomeboy.ButtonRight)
```

Mutable CPU, bus/cartridge, PPU, APU, timer, scheduler, and serial state is independent. The immutable ROM image is reused. Headless-audio and no-video output policies are preserved. Input logs and flight recorders are not inherited by a fork.

For repeated branch/restore on one instance, `CheckpointInto` / `RestoreCheckpoint` remains the lower-overhead primitive.

## Checked durable save states

Existing `SaveState` / `LoadState` remain unchanged. `SaveStateChecked` adds a self-describing envelope intended for durable checkpoints and bug reports. It records:

- checked-state format version
- GomeBoy build/module version when available
- ROM SHA-256
- hardware model
- frame and master cycle
- payload SHA-256

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

if err := emu.LoadStateChecked(data); err != nil {
    log.Fatal(err)
}
```

`LoadStateChecked` rejects corrupted payloads, the wrong ROM, unsupported envelope versions, and hardware-model mismatches before restoring state.

## State hashing

`StateHash` returns a SHA-256 fingerprint of deterministic execution state. The RGB framebuffer is intentionally excluded because it is output-only and may be disabled with `WithoutVideo`; the underlying PPU execution state is still included.

```go
hash, err := emu.StateHashHex()
if err != nil {
    log.Fatal(err)
}
fmt.Println(hash)
```

This is useful for replay assertions, cross-worker determinism checks, and verifying that two branches really reached the same emulator state.

## Performance model

All agent/debug features are opt-in. With no flight recorder attached, ordinary `StepFrame` and `StepFrames` retain their existing execution path except for a nil recorder check after a single-frame step. Batched `StepFrames` remains batched unless the active flight recorder requires per-frame sampling.

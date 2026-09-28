# External renderer primitives

GomeBoy exposes the emulator-side primitives needed by an external semantic
renderer without assigning game-specific meaning to memory, maps, entities, or
battle state. PokePilot and other consumers remain responsible for semantic
decoding and presentation.

## Authoritative sample coordinates

Use these values together for every semantic sample:

- `ExecutionEpoch()` identifies the current continuous execution timeline for
  one `Emulator` instance.
- `FrameCount()` is the deterministic frame coordinate within the restored
  execution state.
- `Cycle()` is the deterministic master-clock coordinate.

Ordinary stepping does not change the execution epoch. A successful ROM load,
`Reset`, `LoadState`, `LoadStateChecked`, `QuickLoad`, or
`RestoreCheckpoint` changes it. The epoch is process-local and intentionally
is not serialized into save states: its purpose is to tell a live consumer that
two observations which happen to have increasing frame numbers still belong to
different execution timelines.

This closes the forward-restore ambiguity that frame/cycle comparison alone
cannot detect. A consumer can therefore treat any epoch change as an immediate
presentation discontinuity and snap instead of interpolating.

## Observation and framebuffer APIs

- `Peek8`, `Peek16`, and `PeekInto` provide fast side-effect-free memory
  observation.
- `SnapshotMemory` copies the full mapped address space into caller-owned
  storage and returns the frame coordinate associated with that observation.
- `Read8`, `Read`, and `ReadInto` remain available when CPU-visible bus
  semantics are specifically required.
- `Frame()` exposes the latest RGB framebuffer as a zero-copy fallback/debug
  surface; `Image`, `PNG`, and `WritePNG` provide copied/encoded forms.
- `ReplayRecordingFrames` supplies deterministic per-frame replay callbacks
  for replay/video/spectator consumers.

No VRAM/OAM change-notification API is required by the current external
renderer. Semantic consumers already derive their state from normal memory
observation, and the existing PPU/debug interfaces remain available for
diagnostics.

## Pause, resume, stepping, and reconnects

The library has no autonomous realtime execution loop. `StepFrame`,
`StepFrames`, and `StepInstruction` are synchronous caller-driven actions,
so "paused" means simply that the caller is not stepping. A renderer may keep
serving the last sample while paused. When stepping resumes, frame/cycle
coordinates continue in the same epoch.

Reconnect is a transport concern outside the emulator. Consumers should compare
the first new sample with the last sample they retained:

- same epoch + nondecreasing coordinates: same execution timeline;
- changed epoch: reset/load/restore discontinuity, snap immediately;
- a transport gap with the same epoch: reconnect within the same authoritative
  timeline; presentation code may choose to snap rather than catch up visually.

## Ownership boundary

GomeBoy owns only generic emulation facts: execution coordinates, memory,
framebuffer output, state restore, and deterministic replay. Game-specific
addresses, tile meanings, entities, dialogue, battle state, themes, and
interpolation policy belong in downstream adapters/renderers.

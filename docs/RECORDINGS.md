# Durable session recordings

GomeBoy supports durable, deterministic emulator session recordings for automation, debugging, agent runs, and later offline rendering.

The format is intentionally game-agnostic. GomeBoy records emulator state and input timing; downstream tools such as PokéPilot can attach Pokémon-specific meaning, model decisions, milestones, battle annotations, or experiment metadata.

## What a `.gbrun` contains

A recording archive contains:

- a checked save state captured at the exact recording start point;
- ROM SHA-256 and hardware model identity;
- start/end frame and cycle coordinates;
- ordered joypad press/release transitions;
- caller-provided string metadata;
- the final deterministic state hash.

It does **not** store every RGB framebuffer. That keeps long recordings compact and lets the same run be replayed headlessly for verification or replayed with video enabled for screenshots, datasets, GIF/video encoding, or spectators.

## Recording a run

```go
recorder, err := emu.StartSessionRecording(gomeboy.RecordingOptions{
    Metadata: map[string]string{
        "run_id": "pokemon-red-0042",
        "agent":  "pokepilot",
        "model":  "local-model-name",
    },
})
if err != nil {
    log.Fatal(err)
}

// Drive the emulator normally.
emu.Press(gomeboy.ButtonA)
emu.StepFrame()
emu.Release(gomeboy.ButtonA)
emu.StepFrames(120)

recording, err := recorder.Stop()
if err != nil {
    log.Fatal(err)
}

if err := gomeboy.SaveRecording("pokemon-red-0042.gbrun", recording); err != nil {
    log.Fatal(err)
}
```

`StartSessionRecording` uses the existing deterministic input timeline internally. A low-level `StartInputRecording` session therefore cannot be active at the same time.

An input transition made on the final frame is preserved even if no additional frame is stepped before `Stop`.

## Loading and replaying

```go
recording, err := gomeboy.LoadRecording("pokemon-red-0042.gbrun")
if err != nil {
    log.Fatal(err)
}

replay, err := gomeboy.New(
    gomeboy.WithROM("pokemon-red.gb"),
    gomeboy.Headless(),
    gomeboy.WithoutVideo(),
)
if err != nil {
    log.Fatal(err)
}
defer replay.Close()

if err := replay.ReplayRecording(recording); err != nil {
    log.Fatal(err)
}
```

Replay restores the checked starting state, applies the recorded transitions at their original frame boundaries, steps to the recorded final frame, verifies the final cycle coordinate, and compares the final `StateHashHex` value with the recording.

A wrong ROM, wrong hardware model, corrupted checked state, malformed input timeline, or deterministic replay mismatch is rejected.

## Rendering frames later

Use `ReplayRecordingFrames` on a video-enabled emulator when a downstream tool wants RGB frames:

```go
replay, err := gomeboy.New(
    gomeboy.WithROM("pokemon-red.gb"),
    gomeboy.Headless(),
)
if err != nil {
    log.Fatal(err)
}
defer replay.Close()

err = replay.ReplayRecordingFrames(recording, func(frame uint64, image gomeboy.Frame) error {
    // image.RGB is a zero-copy view. Copy it here if an encoder keeps it.
    return encoder.AddFrame(frame, image.Width, image.Height, image.RGB)
})
if err != nil {
    log.Fatal(err)
}
```

The callback receives the restored initial frame and every subsequently stepped frame. GomeBoy deliberately does not choose an MP4/GIF/WebM encoder or external multimedia dependency in the core package; PokéPilot or a future CLI can build that policy on top of this callback.

## Why this lives in GomeBoy

The deterministic capture/replay mechanism belongs to the emulator because it is useful for any GB/GBC game. Pokémon-specific run naming, LLM decisions, battle milestones, map annotations, and replay browsing belong downstream.

A useful split is:

- **GomeBoy:** `.gbrun`, checked start state, input timeline, deterministic replay, frame callback, verification.
- **PokéPilot:** when to record, run/model metadata, agent decisions, Pokémon events, UI, and optional video export/publishing.

# Live video streaming

GomeBoy can feed its native RGB framebuffer directly to FFmpeg without adding a multimedia dependency to the emulator core.

The streaming layer is game-agnostic and video-only. GomeBoy owns frame production; FFmpeg owns H.264 encoding, file muxing, and protocols such as RTMP. PokéPilot or another controller decides when to publish frames and whether encoder backpressure is acceptable.

## Architecture

```text
PokéPilot / controller
        |
        v
same GomeBoy Emulator
   |             |
   |             +--> .gbrun session recording
   |
   +--> FrameSink / RGB24
              |
              v
           FFmpeg
         /        \
      MP4        RTMP
```

Streaming does **not** require a second emulator. `PublishFrame`, `StepFrameTo`, and `StepFramesTo` expose the framebuffer from the exact `Emulator` instance the controller is driving.

## Direct Go integration

```go
import (
    "os"

    "github.com/maestroi/gomeboy/pkg/gomeboy"
    gbstream "github.com/maestroi/gomeboy/pkg/gomeboy/stream"
)

emu, err := gomeboy.New(
    gomeboy.WithROM("game.gb"),
    gomeboy.Headless(),
)
if err != nil {
    log.Fatal(err)
}
defer emu.Close()

sink, err := gbstream.NewFFmpegSink(gbstream.FFmpegOptions{
    Output: "capture.mp4",
    Scale:  4,
    Stderr: os.Stderr,
})
if err != nil {
    log.Fatal(err)
}
defer sink.Close()

// Publish every stepped frame from this same emulator instance.
if err := emu.StepFramesTo(600, sink); err != nil {
    log.Fatal(err)
}
```

The default encoder is `libx264` with H.264, `veryfast`, zerolatency tuning, CRF 18, `yuv420p`, the native Game Boy frame rate (~59.7275 Hz), and 4x nearest-neighbor scaling (640x576).

`WithoutVideo()` cannot be used for a live encoder because it intentionally stops RGB framebuffer generation.

## RTMP

RTMP and RTMPS destinations automatically use the FLV muxer:

```go
sink, err := gbstream.NewFFmpegSink(gbstream.FFmpegOptions{
    Output:       "rtmps://stream-provider.example/live/STREAM_KEY",
    Realtime:     true,
    VideoBitrate: "2500k",
    Scale:        4,
    Stderr:       os.Stderr,
})
```

`Realtime` passes FFmpeg's realtime input pacing option. Because `FFmpegSink` is synchronous, a realtime or slow output can apply backpressure to emulator stepping. This is intentional in the generic layer: a downstream controller can choose to accept that pacing or introduce its own bounded queue/frame-dropping policy.

## `gomeboy-stream` utility

The command-line utility requires FFmpeg to be installed and available on `PATH` unless `-ffmpeg` points to another executable.

### Live raw-frame input

By default the utility reads headerless contiguous RGB24 frames from stdin. Native Game Boy frames are exactly `160 * 144 * 3 = 69120` bytes each.

```sh
producer-of-rgb24-frames | \
  go run ./cmd/gomeboy-stream \
    -output capture.mp4
```

For a realtime RTMP destination:

```sh
producer-of-rgb24-frames | \
  go run ./cmd/gomeboy-stream \
    -output 'rtmps://stream-provider.example/live/STREAM_KEY' \
    -realtime \
    -bitrate 2500k
```

This mode is intended for PokéPilot or another process that already receives raw GomeBoy frames. It avoids launching another emulator just to encode video.

### Render or stream a `.gbrun`

A deterministic session recording can be replayed through the same encoder:

```sh
go run ./cmd/gomeboy-stream \
  -rom pokemon-red.gb \
  -recording run.gbrun \
  -output run.mp4
```

Replay uses the checked starting state and input timeline from the `.gbrun`, verifies deterministic completion, and feeds each regenerated RGB frame to FFmpeg.

## Frame sink API

`pkg/gomeboy` exposes:

- `FrameSink` — generic synchronous consumer interface.
- `FrameSinkFunc` — function adapter.
- `PublishFrame(sink)` — publish the current framebuffer without stepping.
- `StepFrameTo(sink)` — step once, then publish that frame.
- `StepFramesTo(n, sink)` — step and publish every intermediate frame.

The `Frame` passed to a sink is zero-copy emulator memory. A sink that wants to retain a frame after `WriteFrame` returns must copy the RGB bytes. The FFmpeg sink consumes them synchronously and does not retain the buffer.

## Scope

The first streaming version deliberately keeps these concerns outside the core emulator:

- Twitch/YouTube account APIs and authentication;
- WebRTC signaling and browser peer management;
- overlays, chat, or Pokémon-specific metadata;
- audio capture/muxing;
- buffering/drop-frame policy for accelerated agent workloads.

Those can be layered on the generic frame sink later without changing emulator correctness or `.gbrun` recording semantics.

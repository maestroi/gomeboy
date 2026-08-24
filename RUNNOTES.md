# RUNNOTES — Task 3: cmd/gomeboy-agent
## Done
- `cmd/gomeboy-agent/main.go`: loads ROM via `gomeboy.New(WithROM, Headless)`,
  builds `webbridge.NewAdapter(emu, fb)`, `display.GetDriver("web")`
  type-asserted to `web.AgentPublisher` (installed driver is `*web.Player`),
  driver started in a goroutine, `runAgentLoop` under signal.NotifyContext
  (INT/TERM).
- `runAgentLoop(ctx, emu, adapter, publisher, frameInterval)` extracted,
  testable: ticker-owned pacing, checks `adapter.Paused()` before every step,
  then StepFrame → adapter.PublishFrame() →
  publisher.PublishAgentState(AgentState{Step, "stub: step the emulator", "StepFrame", AgentRunning}).
  Stub presses no buttons.
- Browser input forwarding goroutine maps the driver's io.Button channels to
  emu.Press/Release via the name-keyed `ioToGomeboyButton` map — never a
  bare gomeboy.Button(b) cast (internal/io.Button: A,B,Select,Start,Right,
  Left,Up,Down vs pkg/gomeboy.Button: A,B,Start,Select,Up,Down,Left,Right).
- `cmd/gomeboy-agent/main_test.go`: fakePublisher +
  TestRunAgentLoop_PublishesFramesAndState (3 frames on fb, steps 0..n,
  AgentRunning, 160*144*3 bytes) + TestRunAgentLoop_RespectsPause (paused:
  0 frames, FrameCount 0, 0 states). ROM fixture
  tests/roms/little-things-gb/firstwhite.gb (from package dir:
  ../../tests/roms/...; extract tests/roms.zip into tests/roms first).
- Verified: go build ok; go test -race ./cmd/gomeboy-agent/ both pass; full
  go test ./... ok except PRE-EXISTING tests pkg failures (TestAge,
  Test_Acid2). Changes left UNCOMMITTED (not requested).

## Next task must know
- THE PLAN FILE docs/superpowers/plans/2026-08-24-agent-web-overlay.md STILL
  DOES NOT EXIST (only the design spec). Task 3 reconstructed from the
  design spec + task instructions; both pre-fixed bugs respected (single
  named web import, no blank import; name-keyed button map).
- Tests pull in pkg/display/web whose init() binds :8090 and log.Fatals if
  the port is taken — keep :8090 free for go test ./cmd/gomeboy-agent.
- *web.Player.gb is nil here (no Attach): browser pause/play or PPU-debug
  messages would nil-deref in ReadPump. Stub stage: no human input expected.
- Svelte AgentPanel.svelte / game.ts EventType mirror still TODO (client
  must handle message type 0x10, JSON payload). Next likely task: real agent
  decision loop (LLM) replacing the stub, and/or the Svelte panel;
  human+agent input arbitration still unimplemented (accepted limitation).

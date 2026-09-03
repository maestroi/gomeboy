# Agent Web Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bridge the existing headless `pkg/gomeboy.Emulator` into the existing web spectator stack (`pkg/display/web` + Svelte client) so an agent-driven emulator can be watched — and its goal/action/observation state read — from a browser.

**Architecture:** A small `webbridge.Adapter` satisfies `pkg/emulator.Controller` for a `pkg/gomeboy.Emulator` and pushes frames onto the display layer's existing `fb` channel, but never advances the emulator itself. A separate `web.AgentPublisher`, implemented directly on the existing `web.Player`, broadcasts generic agent-state (goal/action/observation/status) over the existing WebSocket protocol as one new message type. A new `cmd/gomeboy-agent` binary wires these together with a stub loop that owns pacing and the pause check itself. The Svelte client gets one new store + panel component that renders the new message type.

**Tech Stack:** Go 1.26, `pkg/gomeboy` (existing headless emulator library), `pkg/display/web` (existing WebSocket hub), Svelte + TypeScript (`gomeboy-web/`), `encoding/json` for the new wire message (diagnostic-only, not performance-critical, unlike the existing binary frame protocol).

**Spec:** `docs/superpowers/specs/2026-08-24-agent-web-overlay-design.md`

## Global Constraints

- The web display layer is an observer/controller, never the source of emulator timing — nothing in `webbridge` or `pkg/display/web` may call `StepFrame`/`StepFrames`. Only the agent loop in `cmd/gomeboy-agent` steps the emulator.
- `Pause()` is advisory/cooperative only: it flips a flag and makes `PublishFrame` a no-op. It never blocks, sleeps, or itself prevents the emulator from being stepped.
- `pkg/gomeboy` is not modified by this plan.
- No Pokémon-specific fields anywhere in this plan's code (no map ID, no coordinates) — `AgentState.Observation` is a plain string.
- No Fyne/glfw changes — out of scope, nothing in this plan touches `pkg/display/fyne` or `pkg/display/glfw`.

---

### Task 1: `webbridge.Adapter`

**Files:**
- Create: `pkg/webbridge/adapter.go`
- Test: `pkg/webbridge/adapter_test.go`

**Interfaces:**
- Consumes: `pkg/gomeboy.Frame` (fields `Width int`, `Height int`, `RGB []byte`) and `pkg/gomeboy.Emulator` methods `Frame() gomeboy.Frame`, `LoadROM(string) error`, `QuickSave() error`, `QuickLoad() error` (all already exist, unchanged).
- Produces: `webbridge.Emulator` interface, `webbridge.NewAdapter(emu Emulator, fb chan<- []byte) *Adapter`, and `*Adapter` satisfying `github.com/maestroi/gomeboy/pkg/emulator.Controller` (`LoadROM(string) error`, `Pause()`, `Resume()`, `Paused() bool`, `Initialised() bool`, `QuickSave() error`, `QuickLoad() error`, `SetSpeed(int)`, `Speed() int`), plus `(*Adapter).PublishFrame()`. Task 3 constructs `webbridge.NewAdapter(emu, fb)` where `emu` is a `*gomeboy.Emulator` and `fb` is `chan []byte`.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/webbridge/adapter_test.go
package webbridge

import (
	"errors"
	"testing"

	"github.com/maestroi/gomeboy/pkg/emulator"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// fakeEmulator lets tests exercise Adapter without a real ROM.
type fakeEmulator struct {
	frame        gomeboy.Frame
	loadROMErr   error
	loadROMPath  string
	quickSaveErr error
	quickLoadErr error
	quickSaved   bool
	quickLoaded  bool
}

func (f *fakeEmulator) Frame() gomeboy.Frame { return f.frame }
func (f *fakeEmulator) LoadROM(path string) error {
	f.loadROMPath = path
	return f.loadROMErr
}
func (f *fakeEmulator) QuickSave() error { f.quickSaved = true; return f.quickSaveErr }
func (f *fakeEmulator) QuickLoad() error { f.quickLoaded = true; return f.quickLoadErr }

func TestAdapter_SatisfiesController(t *testing.T) {
	var _ emulator.Controller = (*Adapter)(nil)
}

func TestAdapter_InitiallyInitialisedAndNotPaused(t *testing.T) {
	a := NewAdapter(&fakeEmulator{}, make(chan []byte, 1))
	if !a.Initialised() {
		t.Error("expected Initialised() to be true after NewAdapter")
	}
	if a.Paused() {
		t.Error("expected Paused() to be false after NewAdapter")
	}
}

func TestAdapter_PauseResumeAreAdvisoryOnly(t *testing.T) {
	a := NewAdapter(&fakeEmulator{}, make(chan []byte, 1))
	a.Pause()
	if !a.Paused() {
		t.Error("expected Paused() true after Pause()")
	}
	a.Resume()
	if a.Paused() {
		t.Error("expected Paused() false after Resume()")
	}
}

func TestAdapter_PublishFrame_SendsCopyOfFrameData(t *testing.T) {
	rgb := []byte{1, 2, 3, 4, 5, 6}
	fake := &fakeEmulator{frame: gomeboy.Frame{Width: 1, Height: 2, RGB: rgb}}
	fb := make(chan []byte, 1)
	a := NewAdapter(fake, fb)

	a.PublishFrame()

	select {
	case got := <-fb:
		if len(got) != len(rgb) {
			t.Fatalf("expected %d bytes, got %d", len(rgb), len(got))
		}
		for i := range rgb {
			if got[i] != rgb[i] {
				t.Fatalf("byte %d: expected %d, got %d", i, rgb[i], got[i])
			}
		}
		// mutating the source slice must not affect what was already sent
		rgb[0] = 99
		if got[0] == 99 {
			t.Error("PublishFrame must copy frame data, not alias it")
		}
	default:
		t.Fatal("expected a frame on fb, got none")
	}
}

func TestAdapter_PublishFrame_NoopWhenPaused(t *testing.T) {
	fake := &fakeEmulator{frame: gomeboy.Frame{RGB: []byte{1, 2, 3}}}
	fb := make(chan []byte, 1)
	a := NewAdapter(fake, fb)
	a.Pause()

	a.PublishFrame()

	select {
	case <-fb:
		t.Fatal("expected no frame published while paused")
	default:
	}
}

func TestAdapter_PublishFrame_NeverBlocksOnFullChannel(t *testing.T) {
	fake := &fakeEmulator{frame: gomeboy.Frame{RGB: []byte{1}}}
	fb := make(chan []byte, 1)
	fb <- []byte{0} // fill the buffer
	a := NewAdapter(fake, fb)

	done := make(chan struct{})
	go func() {
		a.PublishFrame()
		close(done)
	}()
	select {
	case <-done:
	default:
		t.Fatal("PublishFrame must not block when fb is full")
	}
}

func TestAdapter_LoadROM_DelegatesAndTracksInitialised(t *testing.T) {
	fake := &fakeEmulator{loadROMErr: errors.New("boom")}
	a := NewAdapter(fake, make(chan []byte, 1))

	err := a.LoadROM("game.gb")
	if err == nil {
		t.Fatal("expected error from LoadROM to propagate")
	}
	if fake.loadROMPath != "game.gb" {
		t.Errorf("expected LoadROM to delegate path, got %q", fake.loadROMPath)
	}
	if a.Initialised() {
		t.Error("expected Initialised() false after a failed LoadROM")
	}

	fake.loadROMErr = nil
	if err := a.LoadROM("game.gb"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !a.Initialised() {
		t.Error("expected Initialised() true after a successful LoadROM")
	}
}

func TestAdapter_QuickSaveQuickLoad_Delegate(t *testing.T) {
	fake := &fakeEmulator{}
	a := NewAdapter(fake, make(chan []byte, 1))

	if err := a.QuickSave(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.quickSaved {
		t.Error("expected QuickSave to delegate to emulator")
	}

	if err := a.QuickLoad(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.quickLoaded {
		t.Error("expected QuickLoad to delegate to emulator")
	}
}

func TestAdapter_SetSpeedSpeed_RoundTrip(t *testing.T) {
	a := NewAdapter(&fakeEmulator{}, make(chan []byte, 1))
	if a.Speed() != 1 {
		t.Errorf("expected default speed 1, got %d", a.Speed())
	}
	a.SetSpeed(4)
	if a.Speed() != 4 {
		t.Errorf("expected speed 4, got %d", a.Speed())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/webbridge/... -v`
Expected: FAIL — `package webbridge` / `Adapter` / `NewAdapter` undefined (no `adapter.go` yet).

- [ ] **Step 3: Write the implementation**

```go
// pkg/webbridge/adapter.go

// Package webbridge bridges pkg/gomeboy's step-driven, headless Emulator
// into pkg/display's push/channel-based Driver model. It is deliberately
// narrow: emulator/display compatibility only. It never advances the
// emulator and it never publishes agent diagnostic state — see
// pkg/display/web.AgentPublisher for that.
package webbridge

import (
	"sync/atomic"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Emulator is the minimal surface Adapter needs. *gomeboy.Emulator
// satisfies it; tests use a fake instead of a real ROM.
type Emulator interface {
	Frame() gomeboy.Frame
	LoadROM(path string) error
	QuickSave() error
	QuickLoad() error
}

// Adapter satisfies pkg/emulator.Controller for an Emulator. It never
// calls StepFrame/StepFrames itself — the caller (the agent loop) owns
// all emulator advancement; PublishFrame only pushes whatever frame is
// already current.
//
// Pause is advisory/cooperative: Pause() only sets a flag and makes
// PublishFrame a no-op. It does not stop the emulator from being
// stepped — callers MUST check Paused() themselves before every
// StepFrame/Press/Release call.
type Adapter struct {
	emu Emulator
	fb  chan<- []byte

	paused atomic.Bool
	loaded atomic.Bool
	speed  atomic.Int64
}

// NewAdapter wraps emu, ready to publish frames onto fb. emu is assumed
// to already have a ROM loaded (pkg/gomeboy.New only succeeds that way
// when constructed with WithROM/WithROMBytes).
func NewAdapter(emu Emulator, fb chan<- []byte) *Adapter {
	a := &Adapter{emu: emu, fb: fb}
	a.loaded.Store(true)
	a.speed.Store(1)
	return a
}

// LoadROM delegates to the wrapped emulator and updates Initialised()
// accordingly.
func (a *Adapter) LoadROM(path string) error {
	err := a.emu.LoadROM(path)
	a.loaded.Store(err == nil)
	return err
}

// Pause sets the advisory paused flag. See the Adapter doc comment.
func (a *Adapter) Pause() { a.paused.Store(true) }

// Resume clears the advisory paused flag.
func (a *Adapter) Resume() { a.paused.Store(false) }

// Paused reports the advisory paused flag.
func (a *Adapter) Paused() bool { return a.paused.Load() }

// Initialised reports whether the wrapped emulator currently has a ROM
// loaded.
func (a *Adapter) Initialised() bool { return a.loaded.Load() }

// QuickSave delegates to the wrapped emulator.
func (a *Adapter) QuickSave() error { return a.emu.QuickSave() }

// QuickLoad delegates to the wrapped emulator.
func (a *Adapter) QuickLoad() error { return a.emu.QuickLoad() }

// SetSpeed records a speed multiplier for display-layer consumers.
// The step-driven agent loop paces itself and does not use this value.
func (a *Adapter) SetSpeed(s int) { a.speed.Store(int64(s)) }

// Speed returns the value last set by SetSpeed (default 1).
func (a *Adapter) Speed() int { return int(a.speed.Load()) }

// PublishFrame pushes a copy of the emulator's current frame onto the
// display hub's frame channel. Called by the agent loop after StepFrame;
// a no-op if the adapter is paused or the channel is full — this never
// blocks the caller.
//
// The copy is required: Emulator.Frame().RGB is documented as a
// zero-copy view valid only until the next StepFrame call, and fb is
// consumed asynchronously by the display hub.
func (a *Adapter) PublishFrame() {
	if a.paused.Load() {
		return
	}
	f := a.emu.Frame()
	buf := make([]byte, len(f.RGB))
	copy(buf, f.RGB)
	select {
	case a.fb <- buf:
	default:
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/webbridge/... -v`
Expected: PASS (all `TestAdapter_*` cases).

- [ ] **Step 5: Commit**

```bash
git add pkg/webbridge/adapter.go pkg/webbridge/adapter_test.go
git commit -m "Add webbridge.Adapter bridging pkg/gomeboy into the display Controller contract"
```

---

### Task 2: `AgentState` type and `AgentPublisher` on the web hub

**Files:**
- Modify: `pkg/display/web/events.go` (append one `Type` const)
- Create: `pkg/display/web/agentstate.go`
- Test: `pkg/display/web/agentstate_test.go`

**Interfaces:**
- Consumes: `pkg/display/web.hub.broadcast` (existing unexported field on `*hub`, already used by `Player.createMessage`/`p.hub.broadcast` elsewhere in `player.go`), `Player.hub *hub` (existing field).
- Produces: `web.AgentStatus` (string enum: `AgentIdle`, `AgentRunning`, `AgentPaused`, `AgentError`), `web.AgentState` struct (`Step uint64`, `Goal string`, `LastAction string`, `Observation string`, `Status AgentStatus`), `web.AgentPublisher` interface (`PublishAgentState(AgentState)`), and `(*web.Player).PublishAgentState(AgentState)` implementing it. Task 3 type-asserts the driver returned by `display.GetDriver("web")` to `web.AgentPublisher`. Task 4 mirrors the wire format (JSON body, message type `AgentUpdate`, no leading player-id byte) in `game.ts`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/display/web/agentstate_test.go
package web

import (
	"encoding/json"
	"testing"
)

func newTestPlayer() (*Player, *hub) {
	h := &hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 4),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
	p := &Player{hub: h}
	return p, h
}

func TestPlayer_PublishAgentState_BroadcastsJSON(t *testing.T) {
	p, h := newTestPlayer()

	p.PublishAgentState(AgentState{
		Step:        7,
		Goal:        "reach Pewter Gym",
		LastAction:  "UP",
		Observation: "collision north",
		Status:      AgentRunning,
	})

	select {
	case msg := <-h.broadcast:
		if len(msg) == 0 || msg[0] != AgentUpdate {
			t.Fatalf("expected message tagged with AgentUpdate, got %v", msg)
		}
		var got AgentState
		if err := json.Unmarshal(msg[1:], &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Step != 7 || got.Goal != "reach Pewter Gym" || got.Status != AgentRunning {
			t.Errorf("unexpected decoded state: %+v", got)
		}
	default:
		t.Fatal("expected a message on hub.broadcast")
	}
}

func TestPlayer_SatisfiesAgentPublisher(t *testing.T) {
	var _ AgentPublisher = (*Player)(nil)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/display/web/... -run TestPlayer_PublishAgentState -v`
Expected: FAIL — `AgentState`/`AgentUpdate`/`AgentPublisher`/`PublishAgentState` undefined.

- [ ] **Step 3: Add the new `Type` constant**

Modify `pkg/display/web/events.go` — append `AgentUpdate` as the last value in the existing `Type` block (do not renumber the existing entries):

```go
const (
	Frame Type = iota
	FramePatch
	FrameSkip
	ClientInfo
	PatchCache
	PatchCacheSync
	FrameCache
	FrameCacheSync
	FrameSync
	ClientListSync
	ClientClosing
	ClientListNew
	ClientListIdentify
	ServerInfo
	PlayerInfo
	PlayerIdentify
	AgentUpdate
)
```

- [ ] **Step 4: Write the implementation**

```go
// pkg/display/web/agentstate.go
package web

import "encoding/json"

// AgentStatus is the coarse state of the agent loop driving the
// emulator, for display purposes only.
type AgentStatus string

const (
	AgentIdle    AgentStatus = "idle"
	AgentRunning AgentStatus = "running"
	AgentPaused  AgentStatus = "paused"
	AgentError   AgentStatus = "error"
)

// AgentState is a generic, game-agnostic snapshot of what the agent loop
// is doing, broadcast to connected clients so a human can watch it work.
// It intentionally has no game-specific fields (no map ID, no
// coordinates) — semantic state belongs in Observation as a string, or
// in a later, separate protocol extension.
type AgentState struct {
	Step        uint64      `json:"step"`
	Goal        string      `json:"goal"`
	LastAction  string      `json:"lastAction"`
	Observation string      `json:"observation"`
	Status      AgentStatus `json:"status"`
}

// AgentPublisher is implemented by display drivers that can broadcast
// agent diagnostic state to connected clients. It is independent of
// pkg/emulator.Controller and of webbridge.Adapter: publishing agent
// state is not part of the emulator/display compatibility bridge.
type AgentPublisher interface {
	PublishAgentState(AgentState)
}

// PublishAgentState broadcasts s as JSON to every connected client,
// tagged with the AgentUpdate message type. Unlike frame data, this is
// low-frequency and diagnostic, so JSON (not the binary frame protocol)
// is used for simplicity.
func (p *Player) PublishAgentState(s AgentState) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	p.hub.broadcast <- append([]byte{AgentUpdate}, data...)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/display/web/... -v`
Expected: PASS, including pre-existing tests in the package (confirms the new `Type` const didn't break anything relying on enum values — nothing does, since `AgentUpdate` is appended last).

- [ ] **Step 6: Commit**

```bash
git add pkg/display/web/events.go pkg/display/web/agentstate.go pkg/display/web/agentstate_test.go
git commit -m "Add AgentState/AgentPublisher: generic agent diagnostics over the existing web hub"
```

---

### Task 3: `cmd/gomeboy-agent`

**Files:**
- Create: `cmd/gomeboy-agent/main.go`
- Test: `cmd/gomeboy-agent/main_test.go`

**Interfaces:**
- Consumes: `pkg/gomeboy.New(...Option) (*Emulator, error)`, `pkg/gomeboy.WithROM`, `pkg/gomeboy.Headless` (existing); `webbridge.NewAdapter(emu, fb) *Adapter` and `(*Adapter).Paused() bool` / `PublishFrame()` (Task 1); `web.AgentPublisher`, `web.AgentState`, `web.AgentRunning` (Task 2); `pkg/display.Init()`, `display.GetDriver(name string) Driver`, `Driver.Start(controller emulator.Controller, fb <-chan []byte, pressed, released chan<- io.Button) error` (existing, unchanged — same pattern as `cmd/gomeboy-web/main.go`).
- Produces: `runAgentLoop(ctx context.Context, emu *gomeboy.Emulator, adapter *webbridge.Adapter, publisher web.AgentPublisher, fb chan []byte, stepInterval, pauseSleepInterval time.Duration)` — extracted so the test can drive it directly without a real WebSocket server.

- [ ] **Step 1: Write the failing test**

```go
// cmd/gomeboy-agent/main_test.go
package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/gomeboy/pkg/display/web"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/gomeboy/pkg/webbridge"
)

var testROM = filepath.Join("..", "..", "tests", "roms", "little-things-gb", "firstwhite.gb")

type fakePublisher struct {
	mu     sync.Mutex
	states []web.AgentState
}

func (f *fakePublisher) PublishAgentState(s web.AgentState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = append(f.states, s)
}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.states)
}

func TestRunAgentLoop_PublishesFramesAndState(t *testing.T) {
	emu, err := gomeboy.New(gomeboy.WithROM(testROM), gomeboy.Headless())
	if err != nil {
		t.Fatalf("gomeboy.New: %v", err)
	}
	defer emu.Close()

	fb := make(chan []byte, 8)
	adapter := webbridge.NewAdapter(emu, fb)
	publisher := &fakePublisher{}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	runAgentLoop(ctx, emu, adapter, publisher, fb, time.Millisecond, 5*time.Millisecond)

	select {
	case frame := <-fb:
		if len(frame) == 0 {
			t.Error("expected a non-empty frame")
		}
	default:
		t.Error("expected at least one frame on fb")
	}

	if publisher.count() == 0 {
		t.Error("expected at least one published AgentState")
	}
}

func TestRunAgentLoop_RespectsPause(t *testing.T) {
	emu, err := gomeboy.New(gomeboy.WithROM(testROM), gomeboy.Headless())
	if err != nil {
		t.Fatalf("gomeboy.New: %v", err)
	}
	defer emu.Close()

	fb := make(chan []byte, 8)
	adapter := webbridge.NewAdapter(emu, fb)
	adapter.Pause()
	publisher := &fakePublisher{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	runAgentLoop(ctx, emu, adapter, publisher, fb, time.Millisecond, 5*time.Millisecond)

	if publisher.count() != 0 {
		t.Errorf("expected no published state while paused, got %d", publisher.count())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gomeboy-agent/... -v`
Expected: FAIL — `runAgentLoop` undefined (`main.go` doesn't exist yet in this form).

- [ ] **Step 3: Write the implementation**

```go
// cmd/gomeboy-agent/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/pkg/display"
	"github.com/maestroi/gomeboy/pkg/display/web"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/gomeboy/pkg/webbridge"
)

// runAgentLoop owns emulator advancement and pacing, per the governing
// invariant: the web display layer is never the source of emulator
// timing. It is a stub — it steps the emulator forward and presses no
// buttons — enough to prove the bridge end-to-end. Real agent decision
// logic (an LLM loop) is a separate, later piece of work.
func runAgentLoop(
	ctx context.Context,
	emu *gomeboy.Emulator,
	adapter *webbridge.Adapter,
	publisher web.AgentPublisher,
	fb chan []byte,
	stepInterval, pauseSleepInterval time.Duration,
) {
	step := uint64(0)
	for ctx.Err() == nil {
		if adapter.Paused() {
			time.Sleep(pauseSleepInterval)
			continue
		}

		emu.StepFrame()
		adapter.PublishFrame()
		publisher.PublishAgentState(web.AgentState{
			Step:   step,
			Status: web.AgentRunning,
		})

		step++
		time.Sleep(stepInterval)
	}
}

func main() {
	display.Init()

	romFile := flag.String("rom", "", "The rom file to load")
	flag.Parse()

	if *romFile == "" {
		fmt.Fprintln(os.Stderr, "gomeboy-agent: -rom is required")
		os.Exit(1)
	}

	emu, err := gomeboy.New(gomeboy.WithROM(*romFile), gomeboy.Headless())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gomeboy-agent: %v\n", err)
		os.Exit(1)
	}
	defer emu.Close()

	fb := make(chan []byte, 120)
	adapter := webbridge.NewAdapter(emu, fb)

	driver := display.GetDriver("web")
	if driver == nil {
		fmt.Fprintln(os.Stderr, "gomeboy-agent: web display driver not installed")
		os.Exit(1)
	}
	publisher, ok := driver.(web.AgentPublisher)
	if !ok {
		fmt.Fprintln(os.Stderr, "gomeboy-agent: web driver does not implement AgentPublisher")
		os.Exit(1)
	}

	// human/agent input arbitration is intentionally deferred (see spec);
	// this binary just forwards whatever io.Button events the display
	// driver reports (from a connected browser client, if any) into the
	// emulator alongside the agent's own Press/Release calls.
	//
	// internal/io.Button and pkg/gomeboy.Button are both uint8 enums but
	// declare their constants in different orders, so values must be
	// translated by name, never by numeric cast.
	pressed := make(chan io.Button, 1)
	released := make(chan io.Button, 1)
	go func() {
		for {
			select {
			case b := <-pressed:
				if gb, ok := ioToGomeboyButton[b]; ok {
					emu.Press(gb)
				}
			case b := <-released:
				if gb, ok := ioToGomeboyButton[b]; ok {
					emu.Release(gb)
				}
			}
		}
	}()

	go func() {
		if err := driver.Start(adapter, fb, pressed, released); err != nil {
			fmt.Fprintf(os.Stderr, "gomeboy-agent: display driver: %v\n", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runAgentLoop(ctx, emu, adapter, publisher, fb, time.Second/60, 100*time.Millisecond)
}

// ioToGomeboyButton translates internal/io.Button (used by the display
// layer) to pkg/gomeboy.Button (used by Emulator.Press/Release). The two
// enums share a uint8 underlying type but declare their constants in
// different orders, so this must be a name-keyed map, never a numeric
// cast.
var ioToGomeboyButton = map[io.Button]gomeboy.Button{
	io.ButtonA:      gomeboy.ButtonA,
	io.ButtonB:      gomeboy.ButtonB,
	io.ButtonStart:  gomeboy.ButtonStart,
	io.ButtonSelect: gomeboy.ButtonSelect,
	io.ButtonUp:     gomeboy.ButtonUp,
	io.ButtonDown:   gomeboy.ButtonDown,
	io.ButtonLeft:   gomeboy.ButtonLeft,
	io.ButtonRight:  gomeboy.ButtonRight,
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./cmd/gomeboy-agent/... && go test ./cmd/gomeboy-agent/... -v`
Expected: build succeeds; `TestRunAgentLoop_PublishesFramesAndState` and `TestRunAgentLoop_RespectsPause` PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/gomeboy-agent/main.go cmd/gomeboy-agent/main_test.go
git commit -m "Add cmd/gomeboy-agent: wires pkg/gomeboy + webbridge into the web display driver"
```

---

### Task 4: Svelte agent panel

**Files:**
- Modify: `pkg/display/web/gomeboy-web/src/lib/game.ts`
- Create: `pkg/display/web/gomeboy-web/src/components/Player/AgentPanel.svelte`
- Modify: `pkg/display/web/gomeboy-web/src/components/Player/Player.svelte`

**Interfaces:**
- Consumes: Go `web.AgentState` JSON shape from Task 2 (`step`, `goal`, `lastAction`, `observation`, `status`), wire message type `AgentUpdate` = index 16 in the `Type`/`EventType` enum (appended last on both sides, so all existing numeric values are unchanged).
- Produces: `AgentStateData` TS interface and `Game.agentState: Writable<AgentStateData | null>`, importable as `import Game from "$lib/game"`, consumed by `AgentPanel.svelte`.

- [ ] **Step 1: Mirror the new enum value**

Modify `EventType` in `game.ts` (append as the last member, matching the Go `Type` const order from Task 2 exactly):

```typescript
export enum EventType {
    Frame,
    FramePatch,
    FrameSkip,
    ClientInfo,
    PatchCache,
    PatchCacheSync,
    FrameCache,
    FrameCacheSync,
    FrameSync,
    ClientListSync,
    ClientClosing,
    ClientListNew,
    ClientListIdentify,
    ServerInfo,
    PlayerInfo,
    PlayerIdentify,
    AgentUpdate,
}
```

- [ ] **Step 2: Add the `AgentStateData` type and store**

Add near the other type/enum declarations in `game.ts`:

```typescript
export enum AgentStatus {
    Idle = "idle",
    Running = "running",
    Paused = "paused",
    Error = "error",
}

export interface AgentStateData {
    step: number;
    goal: string;
    lastAction: string;
    observation: string;
    status: AgentStatus;
}
```

In the `Game` class, add a field declaration alongside the other `Writable<...>` fields (near `client: Writable<UserClient>;`):

```typescript
    agentState: Writable<AgentStateData | null>;
```

Initialize it in the constructor, alongside `this.client = writable(new UserClient("", "", "", 0))`:

```typescript
        this.agentState = writable(null);
```

- [ ] **Step 3: Decode the message in the dispatch switch**

Add a case to the `switch (eventType)` block in `connect()`, alongside the existing `case EventType.ServerInfo:` (not added to the `playerEvents` array — this message has no leading player-id byte, same as `ServerInfo`):

```typescript
                case EventType.AgentUpdate:
                    try {
                        const decoded = JSON.parse(new TextDecoder().decode(eventData)) as AgentStateData;
                        this.agentState.set(decoded);
                    } catch (e) {
                        console.warn("failed to decode AgentUpdate", e);
                    }
                    break
```

- [ ] **Step 4: Create the panel component**

```svelte
<!-- pkg/display/web/gomeboy-web/src/components/Player/AgentPanel.svelte -->
<div class="agent-panel">
	{#if $agentState}
		<div class="row"><span class="label">Status</span><span class="value status-{$agentState.status}">{$agentState.status}</span></div>
		<div class="row"><span class="label">Step</span><span class="value">{$agentState.step}</span></div>
		<div class="row"><span class="label">Goal</span><span class="value">{$agentState.goal}</span></div>
		<div class="row"><span class="label">Last action</span><span class="value">{$agentState.lastAction}</span></div>
		<div class="row"><span class="label">Observation</span><span class="value">{$agentState.observation}</span></div>
	{:else}
		<p class="empty">No agent connected.</p>
	{/if}
</div>

<script>
	import Game from "$lib/game";
	let { agentState } = Game;
</script>

<style lang="scss">
	.agent-panel {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
	}

	.row {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}

	.label {
		opacity: 0.7;
	}

	.value {
		text-align: right;
		word-break: break-word;
	}

	.status-error {
		color: #d33;
	}

	.empty {
		opacity: 0.6;
		font-style: italic;
	}
</style>
```

- [ ] **Step 5: Mount the panel as a sibling of `Controls`**

Modify `Player.svelte`: add the import alongside the existing `Controls` import, and render it inside the same `{#if controls}` block, after `<Controls/>`:

```diff
 	import Controls from "./Controls.svelte";
+	import AgentPanel from "./AgentPanel.svelte";
```

```diff
 				{#if controls}
 					<Controls/>
+					<AgentPanel/>
 					<Scaler height="24" width="160">
```

- [ ] **Step 6: Manual verification**

Run: `cd pkg/display/web/gomeboy-web && npm run dev` (or the project's existing dev script — check `package.json` if `dev` isn't present), then in a separate terminal run `go run ./cmd/gomeboy-agent -rom tests/roms/little-things-gb/firstwhite.gb` and open the dev server URL in a browser.
Expected: the game view renders frames as before; the new panel shows `Status: running` and an incrementing `Step` count (no `Goal`/`Last action`/`Observation` yet, since the Task 3 stub loop doesn't set them — confirms the wiring works end-to-end).

- [ ] **Step 7: Commit**

```bash
git add pkg/display/web/gomeboy-web/src/lib/game.ts pkg/display/web/gomeboy-web/src/components/Player/AgentPanel.svelte pkg/display/web/gomeboy-web/src/components/Player/Player.svelte
git commit -m "Add AgentPanel.svelte: renders AgentState broadcast from the agent-driven emulator"
```

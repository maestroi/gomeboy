package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thelolagemann/gomeboy/pkg/display/web"
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
	"github.com/thelolagemann/gomeboy/pkg/webbridge"
)

// testROM is a simple, deterministic ROM that renders a known image.
var testROM = filepath.Join("..", "..", "tests", "roms", "little-things-gb", "firstwhite.gb")

// fakePublisher records published agent state for assertions. It stands in
// for the web hub so runAgentLoop can be tested without a real WebSocket
// server or connected clients.
type fakePublisher struct {
	mu     sync.Mutex
	states []web.AgentState
}

func (f *fakePublisher) PublishAgentState(s web.AgentState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = append(f.states, s)
}

func (f *fakePublisher) snapshot() []web.AgentState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]web.AgentState(nil), f.states...)
}

func newTestEmulator(t *testing.T) *gomeboy.Emulator {
	t.Helper()
	e, err := gomeboy.New(gomeboy.WithROM(testROM), gomeboy.Headless())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func TestRunAgentLoop_PublishesFramesAndState(t *testing.T) {
	emu := newTestEmulator(t)
	defer emu.Close()

	fb := make(chan []byte, 120)
	adapter := webbridge.NewAdapter(emu, fb)
	publisher := &fakePublisher{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAgentLoop(ctx, emu, adapter, publisher, time.Millisecond)
		close(done)
	}()

	// Collect a few frames off the display channel the agent loop publishes
	// onto.
	const wantFrames = 3
	var frames [][]byte
	deadline := time.After(10 * time.Second)
	for len(frames) < wantFrames {
		select {
		case f := <-fb:
			frames = append(frames, f)
		case <-deadline:
			t.Fatalf("received %d of %d frames before deadline", len(frames), wantFrames)
		}
	}

	cancel()
	<-done

	if n := emu.FrameCount(); n < uint64(wantFrames) {
		t.Errorf("emulator advanced %d frames, want >= %d", n, wantFrames)
	}

	for i, f := range frames {
		if len(f) != 160*144*3 {
			t.Errorf("frame %d is %d bytes, want %d (160x144x3 RGB)", i, len(f), 160*144*3)
		}
	}

	states := publisher.snapshot()
	if len(states) < wantFrames {
		t.Fatalf("publisher recorded %d states, want >= %d", len(states), wantFrames)
	}
	for i, s := range states {
		if s.Step != uint64(i) {
			t.Errorf("state[%d].Step = %d, want %d", i, s.Step, i)
		}
		if s.Status != web.AgentRunning {
			t.Errorf("state[%d].Status = %q, want %q", i, s.Status, web.AgentRunning)
		}
	}
}

func TestRunAgentLoop_RespectsPause(t *testing.T) {
	emu := newTestEmulator(t)
	defer emu.Close()

	fb := make(chan []byte, 120)
	adapter := webbridge.NewAdapter(emu, fb)
	adapter.Pause()
	publisher := &fakePublisher{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAgentLoop(ctx, emu, adapter, publisher, time.Millisecond)
		close(done)
	}()

	// Let the loop spin while paused; it must not advance the emulator.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if n := len(fb); n != 0 {
		t.Errorf("adapter published %d frames while paused, want 0", n)
	}
	if n := emu.FrameCount(); n != 0 {
		t.Errorf("emulator advanced %d frames while paused, want 0", n)
	}
	if s := publisher.snapshot(); len(s) != 0 {
		t.Errorf("publisher recorded %d states while paused, want 0", len(s))
	}
}

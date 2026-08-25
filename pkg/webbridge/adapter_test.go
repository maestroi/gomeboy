package webbridge

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/thelolagemann/gomeboy/pkg/emulator"
	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

// fakeEmulator is a trivial fake implementing the webbridge.Emulator
// interface. It records every call so tests can assert delegation, and its
// Frame.RGB references a mutable buffer so tests can simulate the next step
// invalidating the zero-copy view.
type fakeEmulator struct {
	frame      gomeboy.Frame
	loadROMs   []string
	loadROMErr error
	quickSaves int
	quickLoads int
	saveErr    error
	loadErr    error
	frameCalls int
}

func (f *fakeEmulator) Frame() gomeboy.Frame {
	f.frameCalls++
	return f.frame
}

func (f *fakeEmulator) LoadROM(path string) error {
	f.loadROMs = append(f.loadROMs, path)
	return f.loadROMErr
}

func (f *fakeEmulator) QuickSave() error {
	f.quickSaves++
	return f.saveErr
}

func (f *fakeEmulator) QuickLoad() error {
	f.quickLoads++
	return f.loadErr
}

var (
	_ emulator.Controller = (*Adapter)(nil)
	_ Emulator            = (*gomeboy.Emulator)(nil)
)

func TestAdapter_SatisfiesController(t *testing.T) {
	var c emulator.Controller = NewAdapter(&fakeEmulator{}, make(chan []byte, 1))
	if c.Initialised() {
		t.Error("expected a fresh adapter to report not initialised")
	}
	if c.Paused() {
		t.Error("expected a fresh adapter to report not paused")
	}
}

func TestAdapter_InitiallyInitialisedAndNotPaused(t *testing.T) {
	a := NewAdapter(&fakeEmulator{}, make(chan []byte, 1))
	if a.Initialised() {
		t.Error("expected Initialised() to be false before LoadROM")
	}
	if a.Paused() {
		t.Error("expected Paused() to be false after construction")
	}
}

func TestAdapter_PauseResumeAreAdvisoryOnly(t *testing.T) {
	f := &fakeEmulator{}
	a := NewAdapter(f, make(chan []byte, 1))

	a.Pause()
	if !a.Paused() {
		t.Fatal("expected Paused() to be true after Pause()")
	}
	a.Resume()
	if a.Paused() {
		t.Fatal("expected Paused() to be false after Resume()")
	}
	if f.frameCalls != 0 {
		t.Errorf("Pause/Resume must not touch the emulator: %d Frame() calls", f.frameCalls)
	}
}

func TestAdapter_PublishFrame_SendsCopyOfFrameData(t *testing.T) {
	rgb := []byte{1, 2, 3, 4, 5, 6}
	f := &fakeEmulator{frame: gomeboy.Frame{Width: 2, Height: 1, RGB: rgb}}
	fb := make(chan []byte, 1)
	a := NewAdapter(f, fb)

	a.PublishFrame()

	select {
	case got := <-fb:
		if !bytes.Equal(got, rgb) {
			t.Fatalf("published frame = %v, want %v", got, rgb)
		}
		// Simulate the next step invalidating the zero-copy view.
		for i := range rgb {
			rgb[i] = 0
		}
		if !bytes.Equal(got, []byte{1, 2, 3, 4, 5, 6}) {
			t.Errorf("published frame was mutated by the emulator: %v", got)
		}
	default:
		t.Fatal("expected a frame on the channel")
	}
}

func TestAdapter_PublishFrame_NoopWhenPaused(t *testing.T) {
	f := &fakeEmulator{frame: gomeboy.Frame{Width: 1, Height: 1, RGB: []byte{9, 9, 9}}}
	fb := make(chan []byte, 1)
	a := NewAdapter(f, fb)

	a.Pause()
	a.PublishFrame()

	select {
	case got := <-fb:
		t.Fatalf("expected no frame while paused, got %v", got)
	default:
	}
	if f.frameCalls != 0 {
		t.Errorf("PublishFrame must not read a frame while paused: %d calls", f.frameCalls)
	}
}

func TestAdapter_PublishFrame_NeverBlocksOnFullChannel(t *testing.T) {
	f := &fakeEmulator{frame: gomeboy.Frame{Width: 2, Height: 1, RGB: []byte{1, 2, 3, 4, 5, 6}}}
	fb := make(chan []byte, 1)
	fb <- []byte{0xde, 0xad}
	a := NewAdapter(f, fb)

	done := make(chan struct{})
	go func() {
		a.PublishFrame()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PublishFrame blocked on a full channel")
	}

	select {
	case got := <-fb:
		if !bytes.Equal(got, []byte{0xde, 0xad}) {
			t.Fatalf("existing frame replaced: got %v", got)
		}
	default:
		t.Fatal("existing frame lost")
	}
}

func TestAdapter_LoadROM_DelegatesAndTracksInitialised(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		f := &fakeEmulator{}
		a := NewAdapter(f, make(chan []byte, 1))

		if err := a.LoadROM("rom.gb"); err != nil {
			t.Fatalf("LoadROM: %v", err)
		}
		if len(f.loadROMs) != 1 || f.loadROMs[0] != "rom.gb" {
			t.Fatalf("LoadROM not delegated with the path: %v", f.loadROMs)
		}
		if !a.Initialised() {
			t.Error("expected Initialised() to be true after a successful LoadROM")
		}
	})

	t.Run("failure", func(t *testing.T) {
		boom := errors.New("no such rom")
		f := &fakeEmulator{loadROMErr: boom}
		a := NewAdapter(f, make(chan []byte, 1))

		err := a.LoadROM("missing.gb")
		if !errors.Is(err, boom) {
			t.Fatalf("LoadROM error = %v, want %v", err, boom)
		}
		if a.Initialised() {
			t.Error("expected Initialised() to stay false after a failed LoadROM")
		}
	})
}

func TestAdapter_QuickSaveQuickLoad_Delegate(t *testing.T) {
	saveErr := errors.New("save failed")
	loadErr := errors.New("load failed")
	f := &fakeEmulator{saveErr: saveErr, loadErr: loadErr}
	a := NewAdapter(f, make(chan []byte, 1))

	if err := a.QuickSave(); !errors.Is(err, saveErr) {
		t.Errorf("QuickSave error = %v, want %v", err, saveErr)
	}
	if err := a.QuickLoad(); !errors.Is(err, loadErr) {
		t.Errorf("QuickLoad error = %v, want %v", err, loadErr)
	}
	if f.quickSaves != 1 || f.quickLoads != 1 {
		t.Errorf("delegation counts: saves=%d loads=%d, want 1/1", f.quickSaves, f.quickLoads)
	}
}

func TestAdapter_SetSpeedSpeed_RoundTrip(t *testing.T) {
	a := NewAdapter(&fakeEmulator{}, make(chan []byte, 1))

	if got := a.Speed(); got != 1 {
		t.Fatalf("default Speed() = %d, want 1", got)
	}
	for _, want := range []int{2, 10} {
		a.SetSpeed(want)
		if got := a.Speed(); got != want {
			t.Errorf("Speed() = %d after SetSpeed(%d)", got, want)
		}
	}
}

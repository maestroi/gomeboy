//go:build !android

package glfw

import (
	"errors"
	"strings"
	"testing"
)

type fakeROMDropWindow struct {
	*fakeWindow
	drop  func([]string)
	title string
}

func (f *fakeROMDropWindow) setDropCallback(callback func([]string)) { f.drop = callback }
func (f *fakeROMDropWindow) setTitle(title string)                 { f.title = title }

type fakeROMController struct {
	initialised bool
	paused      bool
	loaded      string
	loadErr     error
	resumes     int
	speed       int
}

func (f *fakeROMController) LoadROM(path string) error {
	f.loaded = path
	if f.loadErr != nil {
		return f.loadErr
	}
	f.initialised = true
	return nil
}
func (f *fakeROMController) Pause()              { f.paused = true }
func (f *fakeROMController) Resume()             { f.paused = false; f.resumes++ }
func (f *fakeROMController) Paused() bool        { return f.paused }
func (f *fakeROMController) Initialised() bool   { return f.initialised }
func (f *fakeROMController) QuickSave() error    { return nil }
func (f *fakeROMController) QuickLoad() error    { return nil }
func (f *fakeROMController) SetSpeed(speed int)  { f.speed = speed }
func (f *fakeROMController) Speed() int          { return f.speed }

func TestSelectROMPath(t *testing.T) {
	path, err := selectROMPath([]string{"notes.txt", "/tmp/Pokemon.GBC"})
	if err != nil {
		t.Fatalf("selectROMPath: %v", err)
	}
	if path != "/tmp/Pokemon.GBC" {
		t.Fatalf("selected %q, want /tmp/Pokemon.GBC", path)
	}

	if _, err := selectROMPath([]string{"notes.txt"}); err == nil {
		t.Fatal("expected non-ROM drop to be rejected")
	}
}

func TestInstallROMDropLoadsAndResumes(t *testing.T) {
	w := &fakeROMDropWindow{fakeWindow: &fakeWindow{}}
	c := &fakeROMController{paused: true}

	installROMDrop(w, c)
	if w.title != romDropHintTitle {
		t.Fatalf("initial title = %q, want %q", w.title, romDropHintTitle)
	}
	if w.drop == nil {
		t.Fatal("drop callback was not installed")
	}

	w.drop([]string{"/roms/game.gb"})
	if c.loaded != "/roms/game.gb" {
		t.Fatalf("loaded %q", c.loaded)
	}
	if c.resumes != 1 || c.paused {
		t.Fatalf("resume state: resumes=%d paused=%v", c.resumes, c.paused)
	}
	if !strings.Contains(w.title, "game.gb") {
		t.Fatalf("success title %q does not identify loaded ROM", w.title)
	}
}

func TestInstallROMDropShowsFailureWithoutResume(t *testing.T) {
	w := &fakeROMDropWindow{fakeWindow: &fakeWindow{}}
	c := &fakeROMController{paused: true, loadErr: errors.New("bad cartridge")}

	installROMDrop(w, c)
	w.drop([]string{"broken.gbc"})
	if c.resumes != 0 {
		t.Fatalf("Resume called %d times after failed load", c.resumes)
	}
	if !strings.Contains(strings.ToLower(w.title), "failed") {
		t.Fatalf("failure was not surfaced in window title: %q", w.title)
	}
}

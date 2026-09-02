//go:build !android

package glfw

import (
	"strings"
	"testing"
)

type menuController struct {
	paused bool
	speed  int
	resets int
}

func (m *menuController) LoadROM(string) error { return nil }
func (m *menuController) Reset() error {
	m.resets++
	return nil
}
func (m *menuController) Pause()            { m.paused = true }
func (m *menuController) Resume()           { m.paused = false }
func (m *menuController) Paused() bool      { return m.paused }
func (m *menuController) Initialised() bool { return true }
func (m *menuController) QuickSave() error  { return nil }
func (m *menuController) QuickLoad() error  { return nil }
func (m *menuController) SetSpeed(speed int) {
	m.speed = speed
}
func (m *menuController) Speed() int { return m.speed }

func TestMenuNavigationWraps(t *testing.T) {
	m := &menu{}
	m.Toggle()
	m.Move(-1)
	if got, want := m.Action(), menuClose; got != want {
		t.Fatalf("Move(-1) action = %v, want %v", got, want)
	}
	m.Move(1)
	if got, want := m.Action(), menuReset; got != want {
		t.Fatalf("Move(1) action = %v, want %v", got, want)
	}
}

func TestMenuTextReflectsControllerState(t *testing.T) {
	c := &menuController{speed: 4}
	m := &menu{open: true, selected: int(menuSpeed)}
	got := m.Text(c, true)

	for _, want := range []string{"Reset", "> Speed: 4x", "Fullscreen: on"} {
		if !strings.Contains(got, want) {
			t.Fatalf("menu text %q does not contain %q", got, want)
		}
	}
}

//go:build !android

package glfw

import (
	"strings"
	"testing"

	"github.com/go-gl/glfw/v3.4/glfw"
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

func TestEscapeMakesMenuModal(t *testing.T) {
	c := &menuController{speed: 1}
	m := &menu{}
	cb := keyCallback(&glfwDriver{}, nil, c, nil, nil, nil, m)

	cb(nil, glfw.KeyEscape, 0, glfw.Press, 0)
	if !m.open {
		t.Fatal("Escape did not open menu")
	}
	if !c.Paused() {
		t.Fatal("opening menu did not pause emulation")
	}

	cb(nil, glfw.KeyEscape, 0, glfw.Press, 0)
	if m.open {
		t.Fatal("second Escape did not close menu")
	}
	if c.Paused() {
		t.Fatal("closing menu did not resume emulation")
	}
}

func TestResetActionKeepsMenuPaused(t *testing.T) {
	c := &menuController{paused: true, speed: 1}
	m := &menu{open: true, selected: int(menuReset)}

	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if c.resets != 1 {
		t.Fatalf("reset calls = %d, want 1", c.resets)
	}
	if !c.Paused() {
		t.Fatal("reset action resumed emulation behind menu")
	}
	if !m.open {
		t.Fatal("reset action closed menu")
	}
}

func TestMenuSpeedCyclesExpectedValues(t *testing.T) {
	c := &menuController{paused: true, speed: 1}
	m := &menu{open: true, selected: int(menuSpeed)}

	for _, want := range []int{2, 4, 8, 1} {
		executeMenuAction(&glfwDriver{}, nil, c, nil, m)
		if c.speed != want {
			t.Fatalf("speed = %d, want %d", c.speed, want)
		}
	}
}

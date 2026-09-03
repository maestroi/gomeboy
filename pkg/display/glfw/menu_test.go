//go:build !android

package glfw

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-gl/glfw/v3.4/glfw"
)

type menuController struct {
	paused  bool
	speed   int
	resets  int
	saveErr error
	loadErr error
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
func (m *menuController) QuickSave() error  { return m.saveErr }
func (m *menuController) QuickLoad() error  { return m.loadErr }
func (m *menuController) SetSpeed(speed int) {
	m.speed = speed
}
func (m *menuController) Speed() int { return m.speed }

func TestMenuNavigationWraps(t *testing.T) {
	m := &menu{}
	m.Toggle()
	m.Move(-1)
	if got, want := m.Action(), menuReset; got != want {
		t.Fatalf("Move(-1) action = %v, want %v", got, want)
	}
	m.Move(1)
	if got, want := m.Action(), menuResume; got != want {
		t.Fatalf("Move(1) action = %v, want %v", got, want)
	}
}

func TestMenuTextReflectsControllerState(t *testing.T) {
	c := &menuController{speed: 4}
	m := &menu{open: true, selected: int(menuSpeed)}
	got := m.Text(c, true)

	for _, want := range []string{"Resume game", "> Speed: 4x", "Fullscreen: on", "arrows D-pad", "Backspace Select"} {
		if !strings.Contains(got, want) {
			t.Fatalf("menu text %q does not contain %q", got, want)
		}
	}
}

func TestStartupHelpExplainsKeyboardMapping(t *testing.T) {
	got := startupHelpText()
	for _, want := range []string{"Arrows       D-Pad", "A / B        Game Boy A / B", "Enter        Start", "Backspace    Select", "Esc          Menu", "F5 / F6      Quick Save / Load", "F11          Fullscreen"} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup help %q does not contain %q", got, want)
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

func TestResumeActionClosesMenu(t *testing.T) {
	c := &menuController{paused: true, speed: 1}
	m := &menu{open: true, selected: int(menuResume)}

	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if m.open {
		t.Fatal("resume action left menu open")
	}
	if c.Paused() {
		t.Fatal("resume action left emulation paused")
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
	if got, want := m.status, "Game reset"; got != want {
		t.Fatalf("reset status = %q, want %q", got, want)
	}
}

func TestMenuQuickSaveAndLoadFeedback(t *testing.T) {
	c := &menuController{paused: true, speed: 1}
	m := &menu{open: true, selected: int(menuQuickSave)}

	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if got, want := m.status, "State saved"; got != want {
		t.Fatalf("quick save status = %q, want %q", got, want)
	}

	c.saveErr = errors.New("disk full")
	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if got, want := m.status, "Quick Save failed"; got != want {
		t.Fatalf("failed quick save status = %q, want %q", got, want)
	}

	m.selected = int(menuQuickLoad)
	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if got, want := m.status, "State loaded"; got != want {
		t.Fatalf("quick load status = %q, want %q", got, want)
	}

	c.loadErr = errors.New("missing state")
	executeMenuAction(&glfwDriver{}, nil, c, nil, m)
	if got, want := m.status, "Quick Load failed"; got != want {
		t.Fatalf("failed quick load status = %q, want %q", got, want)
	}
}

func TestMenuMoveClearsStatus(t *testing.T) {
	m := &menu{open: true, status: "State saved"}
	m.Move(1)
	if m.status != "" {
		t.Fatalf("menu status = %q after navigation, want empty", m.status)
	}
}

func TestMenuSpeedCyclesExpectedValues(t *testing.T) {
	c := &menuController{paused: true, speed: 1}
	m := &menu{open: true, selected: int(menuSpeed)}

	for _, want := range []int{2, 4, 8, 16, 1} {
		executeMenuAction(&glfwDriver{}, nil, c, nil, m)
		if c.speed != want {
			t.Fatalf("speed = %d, want %d", c.speed, want)
		}
	}
}

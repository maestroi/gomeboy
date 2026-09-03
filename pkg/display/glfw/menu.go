//go:build !android

package glfw

import (
	"fmt"
	"strings"

	"github.com/maestroi/gomeboy/pkg/emulator"
)

type menuAction int

const (
	menuResume menuAction = iota
	menuQuickSave
	menuQuickLoad
	menuSpeed
	menuFullscreen
	menuFPS
	menuReset
	menuNoop
)

const menuItemCount = int(menuReset) + 1

// fpsOverlayEnabled is intentionally off by default so the normal gameplay
// view stays clean. The pause menu toggles it for users who want performance
// diagnostics without conflating render FPS with emulation speed.
var fpsOverlayEnabled bool

// menu is the small state machine behind the GLFW in-window menu. It is kept
// independent from OpenGL so navigation and labels can be tested without a
// display server.
type menu struct {
	open     bool
	selected int
	status   string
}

func (m *menu) Toggle() {
	m.open = !m.open
	if m.open {
		m.selected = 0
		m.status = ""
	}
}

func (m *menu) Move(delta int) {
	if !m.open || delta == 0 {
		return
	}
	m.status = ""
	m.selected = (m.selected + delta) % menuItemCount
	if m.selected < 0 {
		m.selected += menuItemCount
	}
}

func (m *menu) Action() menuAction {
	if !m.open {
		return menuResume
	}

	action := menuAction(m.selected)
	if action == menuFPS {
		fpsOverlayEnabled = !fpsOverlayEnabled
		if fpsOverlayEnabled {
			m.SetStatus("FPS counter enabled")
		} else {
			m.SetStatus("FPS counter disabled")
		}
		// executeMenuAction deliberately has no work to do beyond refreshing
		// the menu for this display-only toggle.
		return menuNoop
	}
	return action
}

func (m *menu) Close() {
	m.open = false
	m.status = ""
}

func (m *menu) SetStatus(status string) {
	m.status = status
}

func (m *menu) Text(c emulator.Controller, fullscreen bool) string {
	speed := 1
	if c != nil && c.Speed() > 0 {
		speed = c.Speed()
	}

	fullscreenLabel := "off"
	if fullscreen {
		fullscreenLabel = "on"
	}

	fpsLabel := "off"
	if fpsOverlayEnabled {
		fpsLabel = "on"
	}

	items := []string{
		"Resume game",
		"Quick Save  [F5]",
		"Quick Load  [F6]",
		fmt.Sprintf("Speed: %dx", speed),
		"Fullscreen: " + fullscreenLabel + "  [F11]",
		"Show FPS: " + fpsLabel,
		"Reset game",
	}

	var b strings.Builder
	b.WriteString("GomeBoy                         PAUSED\n\n")
	for i, item := range items {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(item)
		b.WriteByte('\n')
	}
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(m.status)
		b.WriteByte('\n')
	}
	b.WriteString("\nControls: arrows D-pad | A/B buttons\n")
	b.WriteString("Enter Start | Backspace Select | Esc menu\n")
	b.WriteString("Up/Down select | Enter activate")
	return b.String()
}

func startupHelpText() string {
	return "GomeBoy\n\n" +
		"Controls\n" +
		"Arrows       D-Pad\n" +
		"A / B        Game Boy A / B\n" +
		"Enter        Start\n" +
		"Backspace    Select\n" +
		"Esc          Menu\n" +
		"F5 / F6      Quick Save / Load\n" +
		"F11          Fullscreen\n\n" +
		"Controls are always listed in the Esc menu."
}

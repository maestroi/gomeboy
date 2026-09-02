//go:build !android

package glfw

import (
	"fmt"
	"strings"

	"github.com/thelolagemann/gomeboy/pkg/emulator"
)

type menuAction int

const (
	menuPause menuAction = iota
	menuQuickSave
	menuQuickLoad
	menuSpeed
	menuFullscreen
	menuClose
)

const menuItemCount = int(menuClose) + 1

// menu is the small state machine behind the GLFW in-window menu. It is kept
// independent from OpenGL so navigation and labels can be tested without a
// display server.
type menu struct {
	open     bool
	selected int
}

func (m *menu) Toggle() {
	m.open = !m.open
	if m.open {
		m.selected = 0
	}
}

func (m *menu) Move(delta int) {
	if !m.open || delta == 0 {
		return
	}
	m.selected = (m.selected + delta) % menuItemCount
	if m.selected < 0 {
		m.selected += menuItemCount
	}
}

func (m *menu) Action() menuAction {
	if !m.open {
		return menuClose
	}
	return menuAction(m.selected)
}

func (m *menu) Close() {
	m.open = false
}

func (m *menu) Text(c emulator.Controller, fullscreen bool) string {
	pauseLabel := "Pause"
	if c != nil && c.Paused() {
		pauseLabel = "Resume"
	}

	speed := 1
	if c != nil && c.Speed() > 0 {
		speed = c.Speed()
	}

	fullscreenLabel := "off"
	if fullscreen {
		fullscreenLabel = "on"
	}

	items := []string{
		pauseLabel,
		"Quick Save",
		"Quick Load",
		fmt.Sprintf("Speed: %dx", speed),
		"Fullscreen: " + fullscreenLabel,
		"Close menu",
	}

	var b strings.Builder
	b.WriteString("GomeBoy\n\n")
	for i, item := range items {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(item)
		b.WriteByte('\n')
	}
	b.WriteString("\nUp/Down select | Enter activate | Esc close")
	return b.String()
}

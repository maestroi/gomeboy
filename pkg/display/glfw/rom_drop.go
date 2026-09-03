//go:build !android

package glfw

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/pkg/emulator"
	"github.com/maestroi/gomeboy/pkg/log"
)

const (
	defaultWindowTitle = "GomeBoy"
	romDropHintTitle   = "GomeBoy — drop a .gb/.gbc ROM here"
)

// init layers standalone ROM loading onto the existing render-loop seam. This
// keeps the GLFW window abstraction small while still making the no-ROM launch
// path usable without reintroducing a widget toolkit or native-dialog dependency.
func init() {
	previous := runRenderLoop
	runRenderLoop = func(g *glfwDriver, w window, c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
		installROMDrop(w, c)
		return previous(g, w, c, frames, pressed, released)
	}
}

type romDropWindow interface {
	setDropCallback(func(paths []string))
	setTitle(string)
}

func installROMDrop(w window, c emulator.Controller) {
	dropWindow, ok := w.(romDropWindow)
	if !ok || c == nil {
		return
	}

	if !c.Initialised() {
		dropWindow.setTitle(romDropHintTitle)
	}
	dropWindow.setDropCallback(func(paths []string) {
		path, err := selectROMPath(paths)
		if err != nil {
			dropWindow.setTitle("GomeBoy — " + err.Error())
			return
		}
		if err := c.LoadROM(path); err != nil {
			log.Errorf("load ROM %q: %v", path, err)
			dropWindow.setTitle("GomeBoy — ROM load failed")
			return
		}
		c.Resume()
		dropWindow.setTitle(defaultWindowTitle + " — " + filepath.Base(path))
	})
}

func selectROMPath(paths []string) (string, error) {
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".gb" || ext == ".gbc" {
			return path, nil
		}
	}
	return "", fmt.Errorf("drop a .gb or .gbc ROM")
}

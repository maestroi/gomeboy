//go:build !android

package glfw

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/pkg/emulator"
	"github.com/maestroi/gomeboy/pkg/log"
)

// init composes the everyday desktop controls around the existing render-loop
// seam. The wrapper only intercepts F8/F9 and delegates all normal GLFW input
// to the menu/key handler installed by the base driver.
func init() {
	previous := runRenderLoop
	runRenderLoop = func(g *glfwDriver, w window, c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
		return previous(g, &controlWindow{window: w, controller: c}, c, frames, pressed, released)
	}
}

type muteToggler interface {
	ToggleMute() bool
}

type controlWindow struct {
	window
	controller emulator.Controller
}

func (w *controlWindow) setKeyCallback(callback func(*glfw.Window, glfw.Key, int, glfw.Action, glfw.ModifierKey)) {
	w.window.setKeyCallback(func(native *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			switch key {
			case glfw.KeyF8:
				name, err := captureScreenshot()
				if err != nil {
					log.Errorf("screenshot: %v", err)
					w.setStatusTitle("Screenshot failed")
				} else {
					w.setStatusTitle("Saved " + name)
				}
			case glfw.KeyF9:
				if muted, ok := toggleMute(w.controller); ok {
					if muted {
						w.setStatusTitle("Muted")
					} else {
						w.setStatusTitle("Audio on")
					}
				}
			}
		}
		callback(native, key, scancode, action, mods)
	})
}

// setDropCallback is a compatibility passthrough for the standalone ROM-open
// flow. Keeping it here means this wrapper can be composed before or after the
// ROM-drop wrapper without depending on Go's file/init ordering.
func (w *controlWindow) setDropCallback(callback func([]string)) {
	if dropper, ok := w.window.(interface{ setDropCallback(func([]string)) }); ok {
		dropper.setDropCallback(callback)
		return
	}
	if native, ok := w.window.(*glfwWindow); ok {
		native.w.SetDropCallback(func(_ *glfw.Window, paths []string) { callback(paths) })
	}
}

func (w *controlWindow) setTitle(title string) { setNativeWindowTitle(w.window, title) }

func (w *controlWindow) setStatusTitle(status string) {
	setNativeWindowTitle(w.window, "GomeBoy — "+status)
}

func setNativeWindowTitle(w window, title string) {
	switch v := w.(type) {
	case *glfwWindow:
		v.w.SetTitle(title)
	case *controlWindow:
		setNativeWindowTitle(v.window, title)
	}
}

func toggleMute(c emulator.Controller) (muted bool, supported bool) {
	m, ok := c.(muteToggler)
	if !ok {
		return false, false
	}
	return m.ToggleMute(), true
}

func captureScreenshot() (string, error) {
	const width, height = 160, 144
	pixels := make([]byte, width*height*3)
	gl.ReadPixels(0, 0, width, height, gl.RGB, gl.UNSIGNED_BYTE, gl.Ptr(pixels))

	img, err := screenshotImage(pixels, width, height)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("screenshot-%s.png", time.Now().Format("2006-01-02-150405"))
	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return name, nil
}

// screenshotImage converts OpenGL bottom-up RGB pixels into a normal top-down
// RGBA image. It allocates only when a screenshot is requested; no per-frame
// framebuffer copy is retained by the driver.
func screenshotImage(rgb []byte, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 || len(rgb) != width*height*3 {
		return nil, fmt.Errorf("invalid screenshot buffer: got %d bytes for %dx%d", len(rgb), width, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for srcY := 0; srcY < height; srcY++ {
		dstY := height - 1 - srcY
		for x := 0; x < width; x++ {
			src := (srcY*width + x) * 3
			dst := dstY*img.Stride + x*4
			img.Pix[dst+0] = rgb[src+0]
			img.Pix[dst+1] = rgb[src+1]
			img.Pix[dst+2] = rgb[src+2]
			img.Pix[dst+3] = 0xff
		}
	}
	return img, nil
}

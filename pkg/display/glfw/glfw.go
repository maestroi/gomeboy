//go:build !android

package glfw

import (
	"errors"
	"fmt"
	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/pkg/display"
	"github.com/thelolagemann/gomeboy/pkg/emulator"
	"github.com/thelolagemann/gomeboy/pkg/log"
	"github.com/veandco/go-sdl2/sdl"
	"runtime"
	"sync"
	"time"
)

const (
	aspectRatio = float32(160) / float32(144)
)

// init registers the glfw display driver and locks the main goroutine to an
// OS thread so GLFW callbacks run on the main thread. It performs no GLFW,
// SDL, joystick, or OpenGL initialization: Start brings the subsystems up
// and Stop tears them down again.
func init() {
	// GLFW: this is needed to arrange for main to run on main thread
	runtime.LockOSThread()

	driver := &glfwDriver{}
	display.Install("glfw", driver, []display.DriverOption{
		{
			Name:        "fullscreen",
			Default:     false,
			Value:       &driver.fullscreen,
			Type:        "bool",
			Description: "Run in fullscreen mode",
		},
		{
			Name:        "scale",
			Default:     4.0,
			Value:       &driver.scale,
			Type:        "float",
			Description: "Scale the window by this factor",
		},
		{
			Name:        "maintain-aspect-ratio",
			Default:     false,
			Value:       &driver.maintainAspectRatio,
			Type:        "bool",
			Description: "Force the window to maintain the correct aspect ratio",
		},
	})
}

// Injectable initialization seams. The defaults call the real GLFW, SDL, and
// OpenGL entry points; tests substitute them so the driver lifecycle can be
// exercised without a display server.
var (
	glfwInit             = initGLFW
	glfwTerminate        = terminateGLFW
	getPrimaryMonitor    = primaryMonitor
	sdlInit              = initSDL
	sdlQuit              = quitSDL
	sdlNumJoysticks      = countJoysticks
	sdlJoystickOpen      = openJoystick
	enableJoystickEvents = enableJoystickEventState
	glInit               = initGL
	createWindow         = createGLFWWindow
	runRenderLoop        = defaultRenderLoop
)

func initGLFW() error { return glfw.Init() }

func terminateGLFW() { glfw.Terminate() }

func primaryMonitor() *glfw.Monitor { return glfw.GetPrimaryMonitor() }

func initSDL(flags uint32) error { return sdl.Init(flags) }

func quitSDL() { sdl.Quit() }

func countJoysticks() int { return sdl.NumJoysticks() }

func openJoystick(index int) joystick {
	j := sdl.JoystickOpen(index)
	if j == nil {
		return nil
	}
	return &sdlJoystick{j}
}

func enableJoystickEventState() { sdl.JoystickEventState(sdl.ENABLE) }

func initGL() error { return gl.Init() }

func createGLFWWindow(width, height int, title string) (window, error) {
	w, err := glfw.CreateWindow(width, height, title, nil, nil)
	if err != nil {
		return nil, err
	}
	return &glfwWindow{w}, nil
}

func defaultRenderLoop(g *glfwDriver, w window, c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
	return g.renderLoop(w, c, frames, pressed, released)
}

var (
	joypadKeys = map[glfw.Key]io.Button{
		glfw.KeyA:         io.ButtonA,
		glfw.KeyB:         io.ButtonB,
		glfw.KeyDown:      io.ButtonDown,
		glfw.KeyUp:        io.ButtonUp,
		glfw.KeyLeft:      io.ButtonLeft,
		glfw.KeyRight:     io.ButtonRight,
		glfw.KeyEnter:     io.ButtonStart,
		glfw.KeyBackspace: io.ButtonSelect,
	}
)

// window wraps the GLFW window so the driver can be exercised without a
// display server.
type window interface {
	setAspectRatio(x, y int)
	setMonitor(m *glfw.Monitor, x, y, width, height, refreshRate int)
	makeContextCurrent()
	size() (width, height int)
	pos() (x, y int)
	shouldClose() bool
	setKeyCallback(callback func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey))
	setSizeCallback(callback func(w *glfw.Window, width, height int))
	swapBuffers()
	close()
}

// glfwWindow adapts *glfw.Window to the window interface.
type glfwWindow struct {
	w *glfw.Window
}

func (g *glfwWindow) setAspectRatio(x, y int) { g.w.SetAspectRatio(x, y) }

func (g *glfwWindow) setMonitor(m *glfw.Monitor, x, y, width, height, refreshRate int) {
	g.w.SetMonitor(m, x, y, width, height, refreshRate)
}

func (g *glfwWindow) makeContextCurrent() { g.w.MakeContextCurrent() }

func (g *glfwWindow) size() (width, height int) { return g.w.GetSize() }

func (g *glfwWindow) pos() (x, y int) { return g.w.GetPos() }

func (g *glfwWindow) shouldClose() bool { return g.w.ShouldClose() }

func (g *glfwWindow) setKeyCallback(callback func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)) {
	g.w.SetKeyCallback(callback)
}

func (g *glfwWindow) setSizeCallback(callback func(w *glfw.Window, width, height int)) {
	g.w.SetSizeCallback(callback)
}

func (g *glfwWindow) swapBuffers() { g.w.SwapBuffers() }

func (g *glfwWindow) close() { g.w.Destroy() }

// joystick wraps the SDL joystick so the driver can be exercised without a
// physical controller (sdl.Joystick is an incomplete C type and cannot be
// allocated from Go).
type joystick interface {
	rumble(lowFreq, highFreq uint16, duration uint32)
	close()
}

// sdlJoystick adapts *sdl.Joystick to the joystick interface.
type sdlJoystick struct {
	j *sdl.Joystick
}

func (s *sdlJoystick) rumble(lowFreq, highFreq uint16, duration uint32) {
	s.j.Rumble(lowFreq, highFreq, duration)
}

func (s *sdlJoystick) close() { s.j.Close() }

// glfwDriver implements a barebones display driver using GLFW
// and the OpenGL API.
type glfwDriver struct {
	fullscreen          bool
	scale               float64
	maintainAspectRatio bool

	accelerometerX, accelerometerY float32
	windowSettings                 struct {
		width      int
		height     int
		xPos, yPos int
	}

	// lifecycle state, guarded by mu
	mu         sync.Mutex
	started    bool
	glfwInited bool
	sdlInited  bool
	glInited   bool
	monitor    *glfw.Monitor
	joystick   joystick
	window     window
}

// Start starts the display driver. It initializes the subsystems the driver
// needs (GLFW, monitor, SDL joysticks, window, OpenGL) and returns the first
// failure with subsystem context instead of exiting or panicking. Any
// subsystem that was initialized before the failure is released.
func (g *glfwDriver) Start(c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
	g.mu.Lock()
	if g.started {
		g.mu.Unlock()
		return errors.New("glfw: driver already started")
	}
	g.started = true
	g.mu.Unlock()

	if err := g.initSubsystems(); err != nil {
		g.Stop()
		return err
	}

	err := g.run(c, frames, pressed, released)
	g.Stop()
	return err
}

// initSubsystems brings up GLFW, the primary monitor, and SDL (joystick and
// video, plus any connected joysticks). OpenGL is initialized in run after
// the window exists, because some platforms (Windows) require a window
// before the GL library can load.
func (g *glfwDriver) initSubsystems() error {
	if err := glfwInit(); err != nil {
		return fmt.Errorf("glfw: initialize: %w", err)
	}
	g.glfwInited = true

	mon := getPrimaryMonitor()
	if mon == nil {
		return fmt.Errorf("glfw: no primary monitor found")
	}
	g.monitor = mon

	if err := sdlInit(sdl.INIT_JOYSTICK | sdl.INIT_VIDEO); err != nil {
		return fmt.Errorf("sdl: initialize joystick and video: %w", err)
	}
	g.sdlInited = true

	if n := sdlNumJoysticks(); n > 0 {
		enableJoystickEvents()
		for i := 0; i < n; i++ {
			if j := sdlJoystickOpen(i); j != nil {
				g.joystick = j
			}
		}
	}

	return nil
}

// run creates the window, initializes OpenGL, and hands off to the render
// loop. It returns the first failure with subsystem context.
func (g *glfwDriver) run(c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
	// create window
	w, err := createWindow(int(160*g.scale), int(144*g.scale), "GomeBoy")
	if err != nil {
		return fmt.Errorf("glfw: create window: %w", err)
	}
	g.window = w

	if g.maintainAspectRatio {
		w.setAspectRatio(10, 9)
	}
	// fullscreen
	if g.fullscreen {
		bestMode := g.bestMode()
		w.setMonitor(g.monitor, 0, 0, bestMode.Width, bestMode.Height, bestMode.RefreshRate)
	}

	w.makeContextCurrent()

	// Windows OS requires a window to be created before OpenGL can be initialized
	if err := glInit(); err != nil {
		return fmt.Errorf("opengl: initialize: %w", err)
	}
	g.glInited = true

	return runRenderLoop(g, w, c, frames, pressed, released)
}

// renderLoop sets up the OpenGL resources and input callbacks, then blocks in
// the draw loop until the window is closed.
func (g *glfwDriver) renderLoop(w window, c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
	hud := newOSD()

	// initialize window settings
	g.windowSettings.width, g.windowSettings.height = w.size()
	g.windowSettings.xPos, g.windowSettings.yPos = w.pos()

	var texture uint32
	{
		gl.GenTextures(1, &texture)

		gl.BindTexture(gl.TEXTURE_2D, texture)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)

		gl.BindImageTexture(0, texture, 0, false, 0, gl.WRITE_ONLY, gl.RGB8)
	}

	// setup event handling
	w.setKeyCallback(keyCallback(g, w, c, pressed, released, hud))

	var fb uint32
	{
		gl.GenFramebuffers(1, &fb)
		gl.BindFramebuffer(gl.FRAMEBUFFER, fb)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture, 0)

		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, fb)
		gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)
	}

	// handle resizing
	targetWidth := int32(160 * g.scale)
	targetHeight := int32(144 * g.scale)
	var offsetX, offsetY int32
	w.setSizeCallback(func(_ *glfw.Window, width, height int) {

		if float32(width)/float32(height) > aspectRatio {
			targetWidth = int32(float32(height) * aspectRatio)
			targetHeight = int32(height)
		} else {
			targetWidth = int32(width)
			targetHeight = int32(float32(width) / aspectRatio)
		}

		offsetX = (int32(width) - targetWidth) / 2
		offsetY = (int32(height) - targetHeight) / 2
	})

	pollTicker := time.NewTicker(time.Millisecond * 20) // to handle when paused
	defer pollTicker.Stop()
	var sdlEvent sdl.Event
	// draw loop
	for {
		select {
		case f := <-frames:
			glfw.PollEvents()
			if w.shouldClose() {
				return nil
			}
			gl.Clear(gl.COLOR_BUFFER_BIT)

			gl.BindTexture(gl.TEXTURE_2D, texture)
			gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGB8, 160, 144, 0, gl.RGB, gl.UNSIGNED_BYTE, gl.Ptr(f))

			gl.BlitFramebuffer(0, 0, 160, 144, offsetX, offsetY+targetHeight, offsetX+targetWidth, offsetY, gl.COLOR_BUFFER_BIT, gl.NEAREST)

			width, height := w.size()
			hud.Draw(int32(width), int32(height))

			w.swapBuffers()
		case <-pollTicker.C:

			for sdlEvent = sdl.PollEvent(); sdlEvent != nil; sdlEvent = sdl.PollEvent() {
				switch t := sdlEvent.(type) {
				case *sdl.JoyAxisEvent:
					switch t.Axis {
					case 3: // x-axis
						g.accelerometerX = -(float32(t.Value) / 32768.0)
					case 4: // y-axis
						g.accelerometerY = -(float32(t.Value) / 32768.0)
					}
				}
			}

			glfw.PollEvents()
		}
	}
}

// keyCallback builds the window key handler: mapped joypad keys are forwarded
// on the pressed/released channels and the function keys drive fullscreen,
// pause, save/load, and speed.
func keyCallback(g *glfwDriver, w window, c emulator.Controller, pressed, released chan<- io.Button, hud *osd) func(*glfw.Window, glfw.Key, int, glfw.Action, glfw.ModifierKey) {
	return func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		// check to see if the key is mapped to a joypad button
		if button, ok := joypadKeys[key]; ok {
			switch action {
			case glfw.Press:
				pressed <- button
			case glfw.Release:
				released <- button
			}
		}

		if action == glfw.Press {
			switch key {
			case glfw.KeyF11:
				// toggle fullscreen
				if g.fullscreen {
					w.setMonitor(nil, g.windowSettings.xPos, g.windowSettings.yPos, g.windowSettings.width, g.windowSettings.height, 60)
				} else {
					// store the current window settings
					g.windowSettings.width, g.windowSettings.height = w.size()
					g.windowSettings.xPos, g.windowSettings.yPos = w.pos()

					bestMode := g.bestMode()
					w.setMonitor(g.monitor, 0, 0, bestMode.Width, bestMode.Height, bestMode.RefreshRate)
				}

				g.fullscreen = !g.fullscreen
			case glfw.KeyEscape, glfw.KeyPause:
				if c.Paused() {
					c.Resume()
				} else {
					c.Pause()
				}
			case glfw.KeyF5:
				if err := c.QuickSave(); err != nil {
					log.Errorf("quick save: %v", err)
					hud.Show("Save failed", 2*time.Second)
				} else {
					hud.Show("Saved", 1500*time.Millisecond)
				}
			case glfw.KeyF6:
				if err := c.QuickLoad(); err != nil {
					log.Errorf("quick load: %v", err)
					hud.Show("Load failed", 2*time.Second)
				} else {
					hud.Show("Loaded", 1500*time.Millisecond)
				}
			case glfw.KeyEqual, glfw.KeyKPAdd:
				c.SetSpeed(c.Speed() + 1)
				hud.Show(fmt.Sprintf("Speed %dx", c.Speed()), 1500*time.Millisecond)
			case glfw.KeyMinus, glfw.KeyKPSubtract:
				c.SetSpeed(c.Speed() - 1)
				hud.Show(fmt.Sprintf("Speed %dx", c.Speed()), 1500*time.Millisecond)
			}
		}
	}
}

// Stop stops the display driver. It is idempotent: each resource is released
// only if it was initialized, so repeated calls and calls after a partially
// initialized Start are safe.
func (g *glfwDriver) Stop() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.window != nil {
		g.window.close()
		g.window = nil
	}
	if g.joystick != nil {
		g.joystick.close()
		g.joystick = nil
	}
	if g.sdlInited {
		sdlQuit()
		g.sdlInited = false
	}
	g.glInited = false
	if g.glfwInited {
		glfwTerminate()
		g.glfwInited = false
	}
	g.monitor = nil
	g.started = false

	return nil
}

func (g *glfwDriver) X() float32 {
	return g.accelerometerX
}

func (g *glfwDriver) Y() float32 {
	return g.accelerometerY
}

// Rumble triggers the connected joystick's rumble, if any.
func (g *glfwDriver) Rumble(lowFreq, highFreq uint16, duration uint32) {
	g.mu.Lock()
	joystick := g.joystick
	g.mu.Unlock()

	if joystick == nil {
		return
	}
	joystick.rumble(lowFreq, highFreq, duration)
}

// bestMode returns the best video mode for the driver's monitor by choosing
// the highest resolution that is the closest match to the native aspect
// ratio of the monitor. This should provide a reasonable default for most
// monitors.
func (g *glfwDriver) bestMode() *glfw.VidMode {
	mon := g.monitor
	sizeX, sizeY := mon.GetPhysicalSize()
	monAspectRatio := float32(sizeX) / float32(sizeY)
	closestMatch := float32(0)

	modes := mon.GetVideoModes()
	var best = modes[len(modes)-1]
	for _, vm := range modes {
		// skip modes that aren't at least 60FPS
		if vm.RefreshRate < 60 {
			continue
		}

		// skip modes that have a worse aspect ratio match
		vmAspectRatio := float32(vm.Width) / float32(vm.Height)
		if monAspectRatio-vmAspectRatio > closestMatch {
			continue
		}

		closestMatch = vmAspectRatio - monAspectRatio
		best = vm
	}

	return best
}

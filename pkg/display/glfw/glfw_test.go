//go:build !android

package glfw

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/pkg/display"
	"github.com/thelolagemann/gomeboy/pkg/emulator"
)

// resetSeams restores every initialization seam to its default at test end.
// Call it before a test substitutes any seam.
func resetSeams(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		glfwInit = initGLFW
		glfwTerminate = terminateGLFW
		getPrimaryMonitor = primaryMonitor
		sdlInit = initSDL
		sdlQuit = quitSDL
		sdlNumJoysticks = countJoysticks
		sdlJoystickOpen = openJoystick
		enableJoystickEvents = enableJoystickEventState
		glInit = initGL
		createWindow = createGLFWWindow
		runRenderLoop = defaultRenderLoop
	})
}

// fakeWindow is a display-server-free stand-in for the window interface.
type fakeWindow struct {
	mu             sync.Mutex
	aspect         [2]int
	monitorCalls   int
	contextCurrent bool
	closed         int
	keyCallback    func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)
	sizeCallback   func(w *glfw.Window, width, height int)
}

func (f *fakeWindow) setAspectRatio(x, y int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aspect = [2]int{x, y}
}

func (f *fakeWindow) setMonitor(m *glfw.Monitor, x, y, width, height, refreshRate int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.monitorCalls++
}

func (f *fakeWindow) makeContextCurrent() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contextCurrent = true
}

func (f *fakeWindow) size() (int, int) { return 640, 576 }

func (f *fakeWindow) pos() (int, int) { return 10, 20 }

func (f *fakeWindow) shouldClose() bool { return false }

func (f *fakeWindow) setKeyCallback(callback func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keyCallback = callback
}

func (f *fakeWindow) setSizeCallback(callback func(w *glfw.Window, width, height int)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizeCallback = callback
}

func (f *fakeWindow) swapBuffers() {}

func (f *fakeWindow) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

// fakeJoystick is a display-server-free stand-in for the joystick interface.
type fakeJoystick struct {
	mu      sync.Mutex
	rumbles int
	closed  int
}

func (f *fakeJoystick) rumble(lowFreq, highFreq uint16, duration uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rumbles++
}

func (f *fakeJoystick) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

// installedDriver returns the driver instance package init registered.
func installedDriver(t *testing.T) *glfwDriver {
	t.Helper()
	for _, d := range display.InstalledDrivers {
		if d.Name == "glfw" {
			driver, ok := d.Driver.(*glfwDriver)
			if !ok {
				t.Fatalf("glfw driver has type %T, want *glfwDriver", d.Driver)
			}
			return driver
		}
	}
	t.Fatal("glfw driver not installed by package init")
	return nil
}

// GLFW-IMPORT: importing the package registers the driver but performs no
// GLFW, SDL, joystick, or OpenGL initialization.
func TestImportDoesNotInitialize(t *testing.T) {
	driver := installedDriver(t)

	if driver.started {
		t.Error("package import marked the driver started")
	}
	if driver.glfwInited {
		t.Error("package import initialized GLFW")
	}
	if driver.sdlInited {
		t.Error("package import initialized SDL")
	}
	if driver.glInited {
		t.Error("package import initialized OpenGL")
	}
	if driver.monitor != nil {
		t.Error("package import discovered a monitor")
	}
	if driver.joystick != nil {
		t.Error("package import opened a joystick")
	}
	if driver.window != nil {
		t.Error("package import created a window")
	}
}

// GLFW-ERRORS: each injected initialization failure is returned from Start
// with subsystem context, without exiting or panicking the process.
func TestStartReturnsContextualErrors(t *testing.T) {
	tests := []struct {
		name      string
		fault     string
		wantSub   string // subsystem named in the error
		wantCause string // injected cause wrapped in the error
		wantTerm  int    // glfw.Terminate calls after the failed start
		wantQuit  int    // sdl.Quit calls after the failed start
		wantClose int    // window close calls after the failed start
	}{
		{name: "glfw init", fault: "glfwInit", wantSub: "glfw", wantCause: "no display server", wantTerm: 0, wantQuit: 0, wantClose: 0},
		{name: "monitor discovery", fault: "monitor", wantSub: "monitor", wantCause: "no primary monitor", wantTerm: 1, wantQuit: 0, wantClose: 0},
		{name: "sdl init", fault: "sdlInit", wantSub: "sdl", wantCause: "sdl unavailable", wantTerm: 1, wantQuit: 0, wantClose: 0},
		{name: "window creation", fault: "createWindow", wantSub: "window", wantCause: "cannot open display", wantTerm: 1, wantQuit: 1, wantClose: 0},
		{name: "opengl init", fault: "glInit", wantSub: "opengl", wantCause: "opengl unavailable", wantTerm: 1, wantQuit: 1, wantClose: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetSeams(t)
			win := &fakeWindow{}
			var terminated, quit int

			glfwInit = func() error {
				if tc.fault == "glfwInit" {
					return errors.New("no display server")
				}
				return nil
			}
			getPrimaryMonitor = func() *glfw.Monitor {
				if tc.fault == "monitor" {
					return nil
				}
				return &glfw.Monitor{}
			}
			sdlInit = func(flags uint32) error {
				if tc.fault == "sdlInit" {
					return errors.New("sdl unavailable")
				}
				return nil
			}
			sdlNumJoysticks = func() int { return 0 }
			createWindow = func(width, height int, title string) (window, error) {
				if tc.fault == "createWindow" {
					return nil, errors.New("cannot open display")
				}
				return win, nil
			}
			glInit = func() error {
				if tc.fault == "glInit" {
					return errors.New("opengl unavailable")
				}
				return nil
			}
			runRenderLoop = func(g *glfwDriver, w window, c emulator.Controller, frames <-chan []byte, pressed, released chan<- io.Button) error {
				return nil
			}
			glfwTerminate = func() { terminated++ }
			sdlQuit = func() { quit++ }

			g := &glfwDriver{scale: 1}
			err := g.Start(nil, nil, nil, nil)

			if err == nil {
				t.Fatal("expected Start to return the injected failure")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error does not name the %q subsystem: %v", tc.wantSub, err)
			}
			if !strings.Contains(err.Error(), tc.wantCause) {
				t.Errorf("error does not carry the injected cause: %v", err)
			}

			// a failed start must not leave initialized state behind
			if g.glfwInited || g.sdlInited || g.glInited || g.monitor != nil || g.joystick != nil || g.window != nil || g.started {
				t.Errorf("failed start left state behind: glfwInited=%v sdlInited=%v glInited=%v monitor=%v joystick=%v window=%v started=%v",
					g.glfwInited, g.sdlInited, g.glInited, g.monitor, g.joystick, g.window, g.started)
			}

			if terminated != tc.wantTerm {
				t.Errorf("glfw.Terminate calls = %d, want %d", terminated, tc.wantTerm)
			}
			if quit != tc.wantQuit {
				t.Errorf("sdl.Quit calls = %d, want %d", quit, tc.wantQuit)
			}
			if win.closed != tc.wantClose {
				t.Errorf("window close calls = %d, want %d", win.closed, tc.wantClose)
			}
		})
	}
}

// GLFW-CLEANUP: Stop releases only the resources that were initialized and
// is safe to call repeatedly, including on a driver that never started.
func TestStopReleasesOnlyInitializedResources(t *testing.T) {
	t.Run("never started", func(t *testing.T) {
		resetSeams(t)
		var terminated, quit int
		glfwTerminate = func() { terminated++ }
		sdlQuit = func() { quit++ }

		g := &glfwDriver{}
		if err := g.Stop(); err != nil {
			t.Fatalf("first Stop: %v", err)
		}
		if err := g.Stop(); err != nil {
			t.Fatalf("second Stop: %v", err)
		}

		if terminated != 0 || quit != 0 {
			t.Errorf("Stop on a fresh driver released resources: terminate=%d quit=%d", terminated, quit)
		}
	})

	t.Run("fully started", func(t *testing.T) {
		resetSeams(t)
		win := &fakeWindow{}
		joy := &fakeJoystick{}
		var terminated, quit int
		glfwTerminate = func() { terminated++ }
		sdlQuit = func() { quit++ }

		g := &glfwDriver{
			scale:      1,
			monitor:    &glfw.Monitor{},
			joystick:   joy,
			window:     win,
			glfwInited: true,
			sdlInited:  true,
			glInited:   true,
			started:    true,
		}

		if err := g.Stop(); err != nil {
			t.Fatalf("first Stop: %v", err)
		}
		if err := g.Stop(); err != nil {
			t.Fatalf("second Stop: %v", err)
		}
		if err := g.Stop(); err != nil {
			t.Fatalf("third Stop: %v", err)
		}

		if terminated != 1 {
			t.Errorf("glfw.Terminate calls = %d, want 1", terminated)
		}
		if quit != 1 {
			t.Errorf("sdl.Quit calls = %d, want 1", quit)
		}
		if joy.closed != 1 {
			t.Errorf("joystick close calls = %d, want 1", joy.closed)
		}
		if win.closed != 1 {
			t.Errorf("window close calls = %d, want 1", win.closed)
		}
		if g.glfwInited || g.sdlInited || g.glInited || g.monitor != nil || g.joystick != nil || g.window != nil || g.started {
			t.Error("Stop left initialized state behind")
		}
	})

	t.Run("partially started", func(t *testing.T) {
		resetSeams(t)
		var terminated, quit int
		glfwTerminate = func() { terminated++ }
		sdlQuit = func() { quit++ }

		// GLFW came up but SDL failed: only GLFW may be released
		g := &glfwDriver{
			monitor:    &glfw.Monitor{},
			glfwInited: true,
			sdlInited:  false,
		}

		if err := g.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}

		if terminated != 1 {
			t.Errorf("glfw.Terminate calls = %d, want 1", terminated)
		}
		if quit != 0 {
			t.Errorf("sdl.Quit calls = %d, want 0 (SDL was never initialized)", quit)
		}
	})
}

// GLFW-BEHAVIOR: the driver options registered by init are still wired to
// the driver instance's fields with their original names, types, and
// defaults.
func TestDriverOptionsWired(t *testing.T) {
	driver := installedDriver(t)

	var options []display.DriverOption
	for _, d := range display.InstalledDrivers {
		if d.Name == "glfw" {
			options = d.Options
		}
	}
	if len(options) != 3 {
		t.Fatalf("glfw driver registered %d options, want 3", len(options))
	}

	byName := map[string]display.DriverOption{}
	for _, o := range options {
		byName[o.Name] = o
	}

	check := func(name, wantType string, wantDefault any, field any) {
		t.Helper()
		o, ok := byName[name]
		if !ok {
			t.Fatalf("option %q not registered", name)
		}
		if o.Type != wantType {
			t.Errorf("option %q type = %q, want %q", name, o.Type, wantType)
		}
		if o.Default != wantDefault {
			t.Errorf("option %q default = %v, want %v", name, o.Default, wantDefault)
		}
		if !samePointer(o.Value, field) {
			t.Errorf("option %q value is not wired to the driver field", name)
		}
	}

	check("fullscreen", "bool", false, &driver.fullscreen)
	check("scale", "float", 4.0, &driver.scale)
	check("maintain-aspect-ratio", "bool", false, &driver.maintainAspectRatio)

	// the flag package writes through the option's Value pointer: flipping
	// it must flip the driver field
	*byName["fullscreen"].Value.(*bool) = true
	if !driver.fullscreen {
		t.Error("writing the fullscreen option did not update the driver")
	}
	*byName["fullscreen"].Value.(*bool) = false
}

func samePointer(a, b any) bool {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	return ra.Kind() == reflect.Ptr && rb.Kind() == reflect.Ptr && ra.Pointer() == rb.Pointer()
}

// GLFW-BEHAVIOR: the joypad key mapping is unchanged.
func TestJoypadKeyMapping(t *testing.T) {
	want := map[glfw.Key]io.Button{
		glfw.KeyA:         io.ButtonA,
		glfw.KeyB:         io.ButtonB,
		glfw.KeyDown:      io.ButtonDown,
		glfw.KeyUp:        io.ButtonUp,
		glfw.KeyLeft:      io.ButtonLeft,
		glfw.KeyRight:     io.ButtonRight,
		glfw.KeyEnter:     io.ButtonStart,
		glfw.KeyBackspace: io.ButtonSelect,
	}
	if len(joypadKeys) != len(want) {
		t.Fatalf("joypadKeys has %d entries, want %d", len(joypadKeys), len(want))
	}
	for key, button := range want {
		if got := joypadKeys[key]; got != button {
			t.Errorf("joypadKeys[%v] = %v, want %v", key, got, button)
		}
	}
}

// GLFW-BEHAVIOR: the key callback forwards mapped joypad keys to the
// pressed/released channels and ignores unmapped keys.
func TestKeyCallbackForwardsJoypadKeys(t *testing.T) {
	g := &glfwDriver{scale: 1}
	pressed := make(chan io.Button, 4)
	released := make(chan io.Button, 4)
	cb := keyCallback(g, nil, nil, pressed, released, nil)

	cb(nil, glfw.KeyA, 0, glfw.Press, 0)
	if b := <-pressed; b != io.ButtonA {
		t.Fatalf("press A = %v, want ButtonA", b)
	}

	cb(nil, glfw.KeyB, 0, glfw.Release, 0)
	if b := <-released; b != io.ButtonB {
		t.Fatalf("release B = %v, want ButtonB", b)
	}

	cb(nil, glfw.KeyUp, 0, glfw.Press, 0)
	if b := <-pressed; b != io.ButtonUp {
		t.Fatalf("press Up = %v, want ButtonUp", b)
	}

	// an unmapped key emits nothing
	cb(nil, glfw.KeyC, 0, glfw.Press, 0)
	select {
	case b := <-pressed:
		t.Fatalf("unmapped key emitted %v", b)
	default:
	}
}

// GLFW-CLEANUP/GLFW-BEHAVIOR: Rumble drives the attached joystick and is a
// no-op (not a nil panic) when none is attached.
func TestRumbleDrivesJoystick(t *testing.T) {
	joy := &fakeJoystick{}
	g := &glfwDriver{joystick: joy}

	g.Rumble(100, 200, 300)
	if joy.rumbles != 1 {
		t.Errorf("joystick rumble calls = %d, want 1", joy.rumbles)
	}
}

func TestRumbleWithoutJoystickIsNoop(t *testing.T) {
	g := &glfwDriver{}
	g.Rumble(100, 200, 300) // must not panic
}

// GLFW-BEHAVIOR: the screen aspect ratio constant is unchanged.
func TestAspectRatioConstant(t *testing.T) {
	if aspectRatio != float32(160)/float32(144) {
		t.Errorf("aspectRatio = %v, want %v", aspectRatio, float32(160)/float32(144))
	}
}

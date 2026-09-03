//go:build !android

package glfw

import "testing"

type controlTestController struct {
	muted bool
	speed int
}

func (c *controlTestController) LoadROM(string) error { return nil }
func (c *controlTestController) Pause()               {}
func (c *controlTestController) Resume()              {}
func (c *controlTestController) Paused() bool         { return false }
func (c *controlTestController) Initialised() bool    { return true }
func (c *controlTestController) QuickSave() error     { return nil }
func (c *controlTestController) QuickLoad() error     { return nil }
func (c *controlTestController) SetSpeed(speed int)   { c.speed = speed }
func (c *controlTestController) Speed() int           { return c.speed }
func (c *controlTestController) ToggleMute() bool {
	c.muted = !c.muted
	return c.muted
}

func TestToggleMuteOptionalCapability(t *testing.T) {
	c := &controlTestController{}
	muted, ok := toggleMute(c)
	if !ok || !muted {
		t.Fatalf("first toggle = muted %v supported %v, want true true", muted, ok)
	}
	muted, ok = toggleMute(c)
	if !ok || muted {
		t.Fatalf("second toggle = muted %v supported %v, want false true", muted, ok)
	}
}

func TestScreenshotImageFlipsOpenGLRows(t *testing.T) {
	// Two 1-pixel rows. OpenGL returns bottom row first: red, then green.
	rgb := []byte{255, 0, 0, 0, 255, 0}
	img, err := screenshotImage(rgb, 1, 2)
	if err != nil {
		t.Fatalf("screenshotImage: %v", err)
	}
	if got := img.RGBAAt(0, 0); got.G != 255 || got.R != 0 {
		t.Fatalf("top pixel = %#v, want green", got)
	}
	if got := img.RGBAAt(0, 1); got.R != 255 || got.G != 0 {
		t.Fatalf("bottom pixel = %#v, want red", got)
	}
}

func TestScreenshotImageRejectsWrongBufferSize(t *testing.T) {
	if _, err := screenshotImage([]byte{1, 2, 3}, 2, 2); err == nil {
		t.Fatal("expected invalid buffer size to fail")
	}
}

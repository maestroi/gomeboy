//go:build !test

package fyne

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/thelolagemann/gomeboy/internal/ppu"
)

func TestGameScreenFillsWindow(t *testing.T) {
	test.NewApp()

	src := image.NewRGBA(image.Rect(0, 0, ppu.ScreenWidth, ppu.ScreenHeight))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < ppu.ScreenHeight; y++ {
		for x := 0; x < ppu.ScreenWidth; x++ {
			src.Set(x, y, white)
		}
	}

	size := fyne.NewSize(float32(ppu.ScreenWidth*4), float32(ppu.ScreenHeight*4))
	screen := newGameScreen(src, size)

	w := test.NewWindow(screen)
	w.SetPadded(false)
	w.Resize(size)

	captured := w.Canvas().Capture()
	b := captured.Bounds()
	// Native GB resolution is 160x144. If the framebuffer is not scaled, the
	// window centre is empty theme background. After scaling it is game pixels.
	x, y := b.Dx()/2, b.Dy()/2
	r, g, bl, a := captured.At(x, y).RGBA()
	if r>>8 < 200 || g>>8 < 200 || bl>>8 < 200 {
		t.Fatalf("pixel at (%d,%d) = rgba(%d,%d,%d,%d), want near-white (screen did not scale to fill window)",
			x, y, r>>8, g>>8, bl>>8, a>>8)
	}

	// In-place framebuffer updates must show up after Refresh — this is the
	// live emulator path.
	red := color.RGBA{R: 255, A: 255}
	for py := 0; py < ppu.ScreenHeight; py++ {
		for px := 0; px < ppu.ScreenWidth; px++ {
			src.Set(px, py, red)
		}
	}
	screen.Refresh()
	captured = w.Canvas().Capture()
	r, g, bl, a = captured.At(x, y).RGBA()
	if r>>8 < 200 || g>>8 > 50 || bl>>8 > 50 {
		t.Fatalf("after refresh pixel at (%d,%d) = rgba(%d,%d,%d,%d), want near-red",
			x, y, r>>8, g>>8, bl>>8, a>>8)
	}
}

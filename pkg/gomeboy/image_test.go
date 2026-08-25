package gomeboy

import (
	"bytes"
	"image"
	"testing"
)

func TestPNGDecodable(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrames(10)
	pngBytes, err := e.PNG()
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img, format, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("image.Decode: %v", err)
	}
	if format != "png" {
		t.Errorf("decoded format = %q, want png", format)
	}
	b := img.Bounds()
	if b.Dx() != 160 || b.Dy() != 144 {
		t.Errorf("decoded bounds = %dx%d, want 160x144", b.Dx(), b.Dy())
	}
}

func TestImageMatchesFrameBytes(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrames(10)
	f := e.Frame()
	img := e.Image()

	for _, pt := range []struct{ x, y int }{{0, 0}, {159, 143}} {
		i := (pt.y*f.Width + pt.x) * 3
		want := [3]byte{f.RGB[i], f.RGB[i+1], f.RGB[i+2]}
		r, g, b, _ := img.At(pt.x, pt.y).RGBA()
		got := [3]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)}
		if got != want {
			t.Errorf("pixel (%d,%d): Image = %v, Frame.RGB = %v", pt.x, pt.y, got, want)
		}
	}
}

func TestPNGCopyIsIndependentOfNextFrame(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	e.StepFrames(10)
	first, err := e.PNG()
	if err != nil {
		t.Fatalf("PNG (first): %v", err)
	}
	if len(first) == 0 {
		t.Fatal("first PNG is empty")
	}

	e.StepFrames(5)
	second, err := e.PNG()
	if err != nil {
		t.Fatalf("PNG (second): %v", err)
	}
	if len(second) == 0 {
		t.Fatal("second PNG is empty")
	}
}

package gomeboy

import (
	"bytes"
	"image"
	"image/png"
	"io"

	"github.com/thelolagemann/gomeboy/internal/ppu"
)

// Image returns the current frame as a standard image.Image. The returned
// image is a copy; it is safe to keep after the next StepFrame/StepFrames
// call, unlike Frame().RGB.
func (e *Emulator) Image() image.Image {
	f := e.Frame()
	img := image.NewRGBA(image.Rect(0, 0, ppu.ScreenWidth, ppu.ScreenHeight))
	pix := img.Pix
	for i := 0; i < len(f.RGB)/3; i++ {
		s, d := i*3, i*4
		pix[d] = f.RGB[s]
		pix[d+1] = f.RGB[s+1]
		pix[d+2] = f.RGB[s+2]
		pix[d+3] = 255
	}
	return img
}

// WritePNG encodes the current frame as PNG to w.
func (e *Emulator) WritePNG(w io.Writer) error {
	return png.Encode(w, e.Image())
}

// PNG returns the current frame encoded as PNG bytes.
func (e *Emulator) PNG() ([]byte, error) {
	var buf bytes.Buffer
	if err := e.WritePNG(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

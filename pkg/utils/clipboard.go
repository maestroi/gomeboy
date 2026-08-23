//go:build !test

package utils

import (
	"bytes"
	"context"
	"golang.design/x/clipboard"
	"image"
	"image/png"
)

func CopyImage(img image.Image) error {
	err := clipboard.Init()
	if err != nil {
		return err
	}

	// encode image to byte slice
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}

	clipboard.Write(context.Background(), clipboard.FmtImage, b.Bytes())
	return nil
}

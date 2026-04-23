package graphics

import (
	"image"
	"io"

	"github.com/mattn/go-sixel"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
)

// SixelGraphicsEncoder encodes image data to a Writer using the sixel graphics protocol.
func SixelGraphicsEncoder() kit_renderer.ImageEncoder {
	return func(w io.Writer, img image.Image, r *kit_renderer.Renderer) (int, error) {
		err := sixel.NewEncoder(w).Encode(img)
		if err != nil {
			return 0, err
		}
		return 0, nil
	}
}

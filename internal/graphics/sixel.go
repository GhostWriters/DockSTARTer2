package graphics

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"

	"github.com/mattn/go-sixel"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
)

// SixelGraphicsEncoder encodes image data to a Writer using the sixel graphics protocol.
func SixelGraphicsEncoder() kit_renderer.ImageEncoder {
	return func(w io.Writer, img image.Image, r *kit_renderer.Renderer) (int, error) {
		dx, dy := img.Bounds().Dx(), img.Bounds().Dy()
		if dx <= 1 || dy <= 1 {
			return 0, fmt.Errorf("image too small or invalid")
		}

		// Flatten the image onto a solid background to handle transparency issues in terminal graphics.
		flattened := image.NewRGBA(img.Bounds())
		draw.Draw(flattened, flattened.Bounds(), image.Black, image.Point{}, draw.Src)
		draw.Draw(flattened, flattened.Bounds(), img, img.Bounds().Min, draw.Over)

		var sixelBuf bytes.Buffer
		err := sixel.NewEncoder(&sixelBuf).Encode(flattened)
		if err != nil {
			return 0, err
		}
		return w.Write(sixelBuf.Bytes())
	}
}

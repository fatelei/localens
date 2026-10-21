// Package thumbnail creates bounded, local JPEG previews without persistent storage.
package thumbnail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxSourceBytes = 32 * 1024 * 1024
const maxPixels = 32 * 1024 * 1024
const edge = 160

// Render returns a small image data URL. Source bytes/dimensions are checked
// before decoding the immutable buffer, avoiding oversized decoder allocations.
func Render(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourceBytes {
		return "", errors.New("thumbnail source exceeds limits")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxSourceBytes {
		return "", errors.New("thumbnail source exceeds limits")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 20000 || cfg.Height > 20000 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return "", errors.New("thumbnail dimensions exceed limits")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	width, height := cfg.Width, cfg.Height
	if max(width, height) > edge {
		if width >= height {
			height = max(1, height*edge/width)
			width = edge
		} else {
			width = max(1, width*edge/height)
			height = edge
		}
	}
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(destination, destination.Bounds(), image.NewUniform(color.RGBA{R: 242, G: 242, B: 246, A: 255}), image.Point{}, draw.Src)
	xdraw.ApproxBiLinear.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Over, nil)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var result bytes.Buffer
	if err = jpeg.Encode(&result, destination, &jpeg.Options{Quality: 75}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(result.Bytes()), nil
}

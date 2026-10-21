//go:build !darwin || !cgo

package catalog

import (
	"context"
	"errors"
)

func OCRAvailable() bool { return false }
func recognizeImage(context.Context, string) (string, error) {
	return "", errors.New("image OCR is unavailable on this platform")
}

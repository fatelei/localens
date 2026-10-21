//go:build darwin && cgo

package catalog

/*
#cgo LDFLAGS: -framework Vision -framework Foundation -framework ImageIO -framework CoreGraphics
#include <stdlib.h>
char *localens_recognize_text(const char *path, char **error);
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"
)

func OCRAvailable() bool { return true }
func recognizeImage(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var failure *C.char
	result := C.localens_recognize_text(p, &failure)
	if failure != nil {
		defer C.free(unsafe.Pointer(failure))
		return "", errors.New(C.GoString(failure))
	}
	if result == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(result))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return C.GoString(result), nil
}

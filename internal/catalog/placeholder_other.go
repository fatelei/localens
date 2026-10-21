//go:build !darwin

package catalog

import "os"

func isPlaceholder(os.FileInfo) bool { return false }

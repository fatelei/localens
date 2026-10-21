//go:build darwin

package catalog

import (
	"os"
	"syscall"
)

func isPlaceholder(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// UF_DATALESS is set on iCloud files whose content is not present on disk.
	return ok && stat.Flags&0x40000000 != 0
}

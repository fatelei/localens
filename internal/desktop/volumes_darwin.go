//go:build darwin

package desktop

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func LocalVolumeDirectories() []string {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil
	}
	var roots []string
	for _, entry := range entries {
		if entry.Name() == "Recovery" || entry.Name() == "Preboot" || entry.Name() == "VM" {
			continue
		}
		path, err := filepath.EvalSymlinks(filepath.Join("/Volumes", entry.Name()))
		if err != nil || path == "/" {
			continue
		}
		var stat unix.Statfs_t
		if err = unix.Statfs(path, &stat); err != nil || stat.Flags&unix.MNT_LOCAL == 0 {
			continue
		}
		if _, err = os.ReadDir(path); err == nil {
			roots = append(roots, path)
		}
	}
	return roots
}

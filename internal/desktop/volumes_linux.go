//go:build linux

package desktop

import (
	"os"
	"strings"
)

func LocalVolumeDirectories() []string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	var roots []string
	for line := range strings.SplitSeq(string(data), "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		mount := strings.Fields(left)
		fs := strings.Fields(right)
		if len(mount) < 5 || len(fs) == 0 {
			continue
		}
		switch fs[0] {
		case "ext2", "ext3", "ext4", "btrfs", "xfs", "vfat", "exfat", "ntfs", "ntfs3", "zfs", "fuseblk":
		default:
			continue
		}
		path := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(mount[4])
		if path == "/" || path == "/home" || path == "/boot" || strings.HasPrefix(path, "/boot/") {
			continue
		}
		if _, err = os.ReadDir(path); err == nil {
			roots = append(roots, path)
		}
	}
	return roots
}

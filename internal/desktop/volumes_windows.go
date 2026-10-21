//go:build windows

package desktop

import (
	"os"

	"golang.org/x/sys/windows"
)

func LocalVolumeDirectories() []string {
	var roots []string
	for letter := 'A'; letter <= 'Z'; letter++ {
		path := string(letter) + ":\\"
		native, err := windows.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		kind := windows.GetDriveType(native)
		if kind != windows.DRIVE_FIXED && kind != windows.DRIVE_REMOVABLE {
			continue
		}
		if _, err = os.ReadDir(path); err == nil {
			roots = append(roots, path)
		}
	}
	return roots
}

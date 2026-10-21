//go:build !darwin && !linux && !windows

package desktop

// Other systems default to the home tree; additional disks can be added in settings.
func LocalVolumeDirectories() []string { return nil }

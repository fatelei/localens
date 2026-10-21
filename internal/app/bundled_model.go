package app

import (
	"os"
	"path/filepath"
	"runtime"
)

func bundledResources() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return resourcesForExecutable(exe, runtime.GOOS)
}
func resourcesForExecutable(exe, system string) string {
	if system == "darwin" {
		return filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Resources"))
	}
	return filepath.Join(filepath.Dir(exe), "Resources")
}
func bundledPython() string {
	root := bundledResources()
	if root == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(root, "python", "python.exe")
	}
	return filepath.Join(root, "python", "bin", "python3")
}
func bundledModelAvailable() bool {
	root := bundledResources()
	for _, path := range []string{bundledPython(), filepath.Join(root, "model", "model.safetensors")} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

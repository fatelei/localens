package app

import (
	"path/filepath"
	"testing"
)

func TestBundledResourceLayout(t *testing.T) {
	for _, system := range []string{"darwin", "windows", "linux"} {
		t.Run(system, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "Shiwa")
			executable := filepath.Join(dir, "Shiwa")
			want := filepath.Join(dir, "Resources")
			if system == "darwin" {
				executable = filepath.Join(dir, "拾物.app", "Contents", "MacOS", "Shiwa")
				want = filepath.Join(dir, "拾物.app", "Contents", "Resources")
			}
			if got := resourcesForExecutable(executable, system); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

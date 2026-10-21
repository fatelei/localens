package app

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestEditorArguments(t *testing.T) {
	path := "/tmp/中文 code; $(echo unsafe).go"
	for _, tc := range []struct {
		id   string
		line int
		want []string
	}{
		{"code", 42, []string{"--goto", path + ":42:1"}},
		{"goland", 42, []string{"--line", "42", path}},
		{"zed", 42, []string{path + ":42"}},
		{"code", 0, []string{path}},
	} {
		if got := editorArguments(tc.id, path, tc.line); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v", tc.id, got)
		}
	}
	if _, err := editorExecutable("arbitrary-command"); err == nil {
		t.Fatal("accepted unknown editor")
	}
}

func TestOpenCodeRecalculatesCurrentLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test launcher")
	}
	a := appFixture(t)
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package demo\nfunc searchFiles() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	// Insert a line after indexing, and verify the actual editor receives the new line.
	if err := os.WriteFile(path, []byte("// inserted\npackage demo\nfunc searchFiles() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "args")
	t.Setenv("LOCALENS_EDITOR_TEST_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "code"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LOCALENS_EDITOR_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.SetEditor("code"); err != nil {
		t.Fatal(err)
	}
	saved, err := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || saved.Editor != "code" {
		t.Fatalf("preference not saved: %v %v", saved, err)
	}
	if err := a.OpenCode(path, "searchFiles", "func searchFiles() {}"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(log)
		if err == nil && string(b) == "--goto\n"+path+":3:1\n" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("editor did not receive current match line")
}

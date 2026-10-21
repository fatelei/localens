package app

import (
	"errors"
	"localens/internal/desktop"
	"path/filepath"
	"strings"
	"testing"

	"golang.design/x/hotkey"
)

type fakeShortcut struct {
	fail                     bool
	registered, unregistered int
	down, up                 chan hotkey.Event
}

func (f *fakeShortcut) Register() error {
	if f.fail {
		return errors.New("occupied")
	}
	f.registered++
	return nil
}
func (f *fakeShortcut) Unregister() error            { f.unregistered++; return nil }
func (f *fakeShortcut) Keydown() <-chan hotkey.Event { return f.down }
func (f *fakeShortcut) Keyup() <-chan hotkey.Event   { return f.up }
func TestShortcutReplacementPreservesOldOnFailure(t *testing.T) {
	a := appFixture(t)
	handles := []*fakeShortcut{}
	factory := func(spec desktop.ShortcutSpec) desktop.ShortcutHandle {
		f := &fakeShortcut{fail: spec.Canonical == "Ctrl+Alt+X", down: make(chan hotkey.Event), up: make(chan hotkey.Event)}
		handles = append(handles, f)
		return f
	}
	a.mu.Lock()
	a.desktopReady = true
	a.wg.Add(1)
	a.mu.Unlock()
	go a.runShortcutService(factory)
	waitApp(t, a, func(s Status, _ bool) bool { return s.Shortcut != "" })
	if err := a.SetShortcut("Ctrl+Alt+P"); err != nil {
		t.Fatal(err)
	}
	if handles[0].unregistered != 1 {
		t.Fatal("old shortcut not released")
	}
	saved, err := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || saved.Shortcut != "Ctrl+Alt+P" {
		t.Fatalf("not persisted: %+v %v", saved, err)
	}
	if err := a.SetShortcut("Ctrl+Alt+X"); err == nil {
		t.Fatal("occupied combination accepted")
	}
	if handles[1].unregistered != 0 {
		t.Fatal("registration failure removed active shortcut")
	}
	saved, _ = loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if saved.Shortcut != "Ctrl+Alt+P" {
		t.Fatal("failed shortcut persisted")
	}
	a.mu.Lock()
	original := a.dataDir
	a.dataDir = filepath.Join(original, "missing")
	a.mu.Unlock()
	err = a.SetShortcut("Ctrl+Alt+F")
	a.mu.Lock()
	a.dataDir = original
	label := a.status.Shortcut
	a.mu.Unlock()
	if err == nil || !strings.Contains(label, "P") {
		t.Fatal("save failure replaced shortcut")
	}
	if handles[1].unregistered != 0 || handles[len(handles)-1].unregistered != 1 {
		t.Fatal("save failure did not roll back native registration")
	}
}

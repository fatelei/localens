//go:build linux

package desktop

import (
	"testing"

	"golang.design/x/hotkey"
)

func TestLinuxShortcutModifierMapping(t *testing.T) {
	spec, err := ParseShortcut("Alt+Meta+Q")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Canonical != "Alt+Meta+Q" || spec.Label != "Alt + Super + Q" || spec.key != hotkey.KeyQ {
		t.Fatalf("unexpected shortcut: %+v", spec)
	}
	if len(spec.modifiers) != 2 || spec.modifiers[0] != hotkey.Mod1 || spec.modifiers[1] != hotkey.Mod4 {
		t.Fatalf("unexpected X11 modifiers: %v", spec.modifiers)
	}
}

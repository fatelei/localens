package desktop

import (
	"testing"

	"golang.design/x/hotkey"
)

func TestShortcutValidation(t *testing.T) {
	for _, value := range []string{"q", "Shift+Q", "Ctrl+Ctrl+Q", "Ctrl+Unknown", "Ctrl+K"} {
		if _, err := ParseShortcut(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	spec, err := ParseShortcut("alt + ctrl + p")
	if err != nil || spec.Canonical != "Ctrl+Alt+P" || spec.key != hotkey.KeyP {
		t.Fatalf("%+v %v", spec, err)
	}
}

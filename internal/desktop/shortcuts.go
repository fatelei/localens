package desktop

import (
	"errors"
	"runtime"
	"strings"

	"golang.design/x/hotkey"
)

type ShortcutSpec struct {
	Canonical, Label string
	modifiers        []hotkey.Modifier
	key              hotkey.Key
}
type ShortcutHandle interface {
	Register() error
	Unregister() error
	Keydown() <-chan hotkey.Event
	Keyup() <-chan hotkey.Event
}

type ShortcutFactory func(ShortcutSpec) ShortcutHandle

var shortcutKeys = map[string]hotkey.Key{
	"A": hotkey.KeyA, "B": hotkey.KeyB, "C": hotkey.KeyC, "D": hotkey.KeyD, "E": hotkey.KeyE, "F": hotkey.KeyF, "G": hotkey.KeyG, "H": hotkey.KeyH, "I": hotkey.KeyI, "J": hotkey.KeyJ, "K": hotkey.KeyK, "L": hotkey.KeyL, "M": hotkey.KeyM, "N": hotkey.KeyN, "O": hotkey.KeyO, "P": hotkey.KeyP, "Q": hotkey.KeyQ, "R": hotkey.KeyR, "S": hotkey.KeyS, "T": hotkey.KeyT, "U": hotkey.KeyU, "V": hotkey.KeyV, "W": hotkey.KeyW, "X": hotkey.KeyX, "Y": hotkey.KeyY, "Z": hotkey.KeyZ,
	"0": hotkey.Key0, "1": hotkey.Key1, "2": hotkey.Key2, "3": hotkey.Key3, "4": hotkey.Key4, "5": hotkey.Key5, "6": hotkey.Key6, "7": hotkey.Key7, "8": hotkey.Key8, "9": hotkey.Key9, "SPACE": hotkey.KeySpace,
	"F1": hotkey.KeyF1, "F2": hotkey.KeyF2, "F3": hotkey.KeyF3, "F4": hotkey.KeyF4, "F5": hotkey.KeyF5, "F6": hotkey.KeyF6, "F7": hotkey.KeyF7, "F8": hotkey.KeyF8, "F9": hotkey.KeyF9, "F10": hotkey.KeyF10, "F11": hotkey.KeyF11, "F12": hotkey.KeyF12,
}

func ParseShortcut(value string) (ShortcutSpec, error) {
	spec := ShortcutSpec{}
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(value, " ", "")), "+")
	if len(parts) < 2 || len(parts) > 5 {
		return spec, errors.New("use Ctrl, Alt or Meta with a letter, digit, Space or F1–F12")
	}
	keyName := parts[len(parts)-1]
	key, ok := shortcutKeys[keyName]
	if !ok {
		return spec, errors.New("unsupported shortcut key")
	}
	modifiers := map[string]bool{}
	for _, part := range parts[:len(parts)-1] {
		if part != "CTRL" && part != "ALT" && part != "SHIFT" && part != "META" {
			return spec, errors.New("unsupported shortcut modifier")
		}
		if modifiers[part] {
			return spec, errors.New("duplicate shortcut modifier")
		}
		modifiers[part] = true
	}
	if !modifiers["CTRL"] && !modifiers["ALT"] && !modifiers["META"] {
		return spec, errors.New("include Ctrl, Alt or Meta")
	}
	meta, metaName := shortcutMetaModifier()
	alt := shortcutAltModifier()
	names, labels := []string{}, []string{}
	for _, entry := range []struct {
		id, name, Label string
		mod             hotkey.Modifier
	}{{"CTRL", "Ctrl", "Ctrl", hotkey.ModCtrl}, {"ALT", "Alt", "Alt", alt}, {"SHIFT", "Shift", "Shift", hotkey.ModShift}, {"META", "Meta", metaName, meta}} {
		if modifiers[entry.id] {
			names = append(names, entry.name)
			labels = append(labels, entry.Label)
			spec.modifiers = append(spec.modifiers, entry.mod)
		}
	}
	if keyName == "SPACE" {
		keyName = "Space"
	}
	names = append(names, keyName)
	labels = append(labels, keyName)
	spec.Canonical = strings.Join(names, "+")
	spec.Label = strings.Join(labels, " + ")
	spec.key = key
	if spec.Canonical == "Ctrl+K" || spec.Canonical == "Ctrl+Y" || spec.Canonical == "Meta+K" || spec.Canonical == "Meta+Y" || (runtime.GOOS == "darwin" && spec.Canonical == "Meta+Q") || (runtime.GOOS != "darwin" && spec.Canonical == "Ctrl+Shift+Q") {
		return ShortcutSpec{}, errors.New("shortcut conflicts with a Shiwa command")
	}
	return spec, nil
}

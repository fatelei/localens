//go:build !darwin

package desktop

import "golang.design/x/hotkey"

func NativeShortcut(spec ShortcutSpec) ShortcutHandle { return hotkey.New(spec.modifiers, spec.key) }

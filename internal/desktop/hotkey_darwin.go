//go:build darwin

package desktop

import "golang.design/x/hotkey"

func shortcutMetaModifier() (hotkey.Modifier, string) { return hotkey.ModCmd, "Cmd" }

func shortcutAltModifier() hotkey.Modifier { return hotkey.ModOption }

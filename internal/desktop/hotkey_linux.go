//go:build linux

package desktop

import "golang.design/x/hotkey"

func shortcutMetaModifier() (hotkey.Modifier, string) { return hotkey.Mod4, "Super" }

func shortcutAltModifier() hotkey.Modifier { return hotkey.ModAlt }

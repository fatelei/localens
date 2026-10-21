//go:build windows

package desktop

import "golang.design/x/hotkey"

func shortcutMetaModifier() (hotkey.Modifier, string) {
	return hotkey.ModWin, "Win"
}

func shortcutAltModifier() hotkey.Modifier { return hotkey.ModAlt }

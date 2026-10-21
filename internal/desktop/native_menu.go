package desktop

import (
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

func ApplicationMenu(language string, actions Actions) *menu.Menu {
	result := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		result.Append(menu.AppMenu())
		result.Append(menu.EditMenu())
	}
	labels := []string{"显示搜索", "设置", "退出拾物"}
	if language == "en" {
		labels = []string{"Show Search", "Settings", "Quit Shiwa"}
	}
	name := "拾物"
	if language == "en" {
		name = "Shiwa"
	}
	commands := result.AddSubmenu(name)
	commands.AddText(labels[0], keys.CmdOrCtrl("k"), func(*menu.CallbackData) { actions.ShowLauncher() })
	commands.AddText(labels[1], keys.CmdOrCtrl(","), func(*menu.CallbackData) { actions.ShowSettings() })
	commands.AddSeparator()
	quitShortcut := keys.CmdOrCtrl("q")
	if runtime.GOOS != "darwin" {
		quitShortcut = keys.Combo("q", keys.ControlKey, keys.ShiftKey)
	}
	commands.AddText(labels[2], quitShortcut, func(*menu.CallbackData) { actions.QuitApp() })
	return result
}

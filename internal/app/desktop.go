package app

import (
	"localens/internal/desktop"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

func (a *App) desktopActions() desktop.Actions {
	return desktop.Actions{ShowLauncher: a.ShowLauncher, ShowSettings: a.ShowSettings, QuitApp: a.QuitApp}
}
func (a *App) applicationMenu(language string) *menu.Menu {
	return desktop.ApplicationMenu(language, a.desktopActions())
}
func (a *App) setupSystemTray() {
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return
	}
	a.wg.Add(1)
	a.mu.Unlock()
	desktop.InstallTray()
	go func() { defer a.wg.Done(); desktop.RunTray(a.ctx, a.desktopActions()) }()
}

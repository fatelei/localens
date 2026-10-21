package app

import (
	"context"
	"localens/internal/desktop"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) domReady(ctx context.Context) {
	a.mu.Lock()
	a.desktopReady = true
	a.mu.Unlock()
	desktop.ConfigureWindowAppearance()
	a.setupSystemTray()
	a.mu.Lock()
	language := a.status.Language
	a.mu.Unlock()
	wailsruntime.WindowSetTitle(ctx, windowTitle(language))
	wailsruntime.MenuSetApplicationMenu(ctx, a.applicationMenu(language))
	desktop.SetTrayLanguage(language == "en")
	wailsruntime.WindowCenter(ctx)
	x, y := wailsruntime.WindowGetPosition(ctx)
	wailsruntime.WindowSetPosition(ctx, x, max(40, y-120))
	wailsruntime.WindowExecJS(ctx, "window.localensFocus?.()")
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return
	}
	a.wg.Add(1)
	a.mu.Unlock()
	go a.runShortcutService(desktop.NativeShortcut)
}

// SetPanelHeight expands results beneath the search field while preserving its top edge.
func (a *App) SetPanelHeight(height int) {
	if a.ctx == nil {
		return
	}
	x, y := wailsruntime.WindowGetPosition(a.ctx)
	wailsruntime.WindowSetSize(a.ctx, 720, max(84, min(height, 600)))
	wailsruntime.WindowSetPosition(a.ctx, x, y)
	desktop.ConfigureWindowAppearance()
}

// ShowLauncher brings the floating search bar to the foreground.
func (a *App) ShowLauncher() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Show(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowExecJS(a.ctx, "window.localensFocus?.()")
}

// HideLauncher dismisses the search bar while indexing continues in the background.
func (a *App) HideLauncher() {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowHide(a.ctx)
}

// ShowSettings opens indexing and model preferences inside the compact panel.
func (a *App) ShowSettings() {
	a.ShowLauncher()
	if a.ctx != nil {
		wailsruntime.WindowExecJS(a.ctx, "showSettings()")
	}
}

// QuitApp terminates Shiwa and its background workers.
func (a *App) QuitApp() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func windowTitle(language string) string {
	if language == "en" {
		return "Shiwa Search"
	}
	return "拾物搜索"
}

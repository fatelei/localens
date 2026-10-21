package app

import (
	"localens/frontend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// Run starts the Wails desktop application.
func Run() error {
	app := NewApp()

	return wails.Run(&options.App{
		Title: "拾物搜索", Width: 720, Height: 84, MinWidth: 720, MinHeight: 84, MaxWidth: 720, MaxHeight: 600,
		Frameless: true, DisableResize: true, AlwaysOnTop: true, HideWindowOnClose: true,
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "app.localens.desktop", OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.ShowLauncher() }},
		BackgroundColour:   &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:        &assetserver.Options{Assets: frontend.Assets}, OnStartup: app.startup, OnDomReady: app.domReady, OnShutdown: app.shutdown, Bind: []any{app}, Menu: app.applicationMenu("zh-CN"),
		Mac: &mac.Options{TitleBar: mac.TitleBarHidden(), WebviewIsTransparent: true, WindowIsTranslucent: false, DisableZoom: true},
	})
}

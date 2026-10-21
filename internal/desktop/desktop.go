// Package desktop owns OS integration and never imports the application layer.
package desktop

// Actions connects native menu/tray events to application commands.
type Actions struct {
	ShowLauncher func()
	ShowSettings func()
	QuitApp      func()
}

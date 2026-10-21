//go:build !darwin

package desktop

import "context"

func InstallTray() {}

func RunTray(context.Context, Actions) {}

func SetTrayLanguage(bool) {}

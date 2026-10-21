package app

import (
	"context"
	"errors"
	"fmt"
	"localens/internal/desktop"

	"golang.design/x/hotkey"
)

type shortcutRequest struct {
	spec   desktop.ShortcutSpec
	result chan error
}

// SetShortcut replaces the global registration transactionally and persists it.
func (a *App) SetShortcut(value string) error {
	spec, err := desktop.ParseShortcut(value)
	if err != nil {
		return err
	}
	a.mu.Lock()
	available := a.desktopReady && !a.closing
	a.mu.Unlock()
	if !available {
		return errors.New("desktop shortcut service is not ready")
	}
	request := shortcutRequest{spec, make(chan error, 1)}
	select {
	case a.shortcutRequests <- request:
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
	select {
	case err := <-request.result:
		return err
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
}
func (a *App) runShortcutService(factory desktop.ShortcutFactory) {
	defer a.wg.Done()
	a.mu.Lock()
	value := a.preferences.Shortcut
	a.mu.Unlock()
	if value == "" {
		value = "Ctrl+Q"
	}
	spec, err := desktop.ParseShortcut(value)
	if err != nil {
		spec, _ = desktop.ParseShortcut("Ctrl+Q")
	}
	active := a.createShortcut(factory, spec)
	if err := active.Register(); err != nil {
		active = nil
		a.mu.Lock()
		a.status.ShortcutError = fmt.Sprintf("Shortcut unavailable: %v", err)
		a.mu.Unlock()
	}
	a.mu.Lock()
	a.status.Shortcut = spec.Label
	a.mu.Unlock()
	defer func() {
		if active != nil {
			_ = active.Unregister()
		}
	}()
	for {
		var down, up <-chan hotkey.Event
		if active != nil {
			down = active.Keydown()
			up = active.Keyup()
		}
		select {
		case <-a.ctx.Done():
			return
		case <-down:
			a.ShowLauncher()
		case <-up:
		case request := <-a.shortcutRequests:
			if request.spec.Canonical == spec.Canonical && active != nil {
				request.result <- nil
				continue
			}
			next := a.createShortcut(factory, request.spec)
			if err := next.Register(); err != nil {
				request.result <- fmt.Errorf("Shortcut unavailable; previous shortcut retained: %w", err)
				continue
			}
			// Keep the current binding active until the new one is registered and preferences are durable.
			a.mu.Lock()
			old := a.preferences.Shortcut
			a.preferences.Shortcut = request.spec.Canonical
			err := a.savePreferencesLocked()
			if err != nil {
				a.preferences.Shortcut = old
			}
			a.mu.Unlock()
			if err != nil {
				_ = next.Unregister()
				request.result <- err
				continue
			}
			if active != nil {
				if err := active.Unregister(); err != nil {
					_ = next.Unregister()
					a.mu.Lock()
					a.preferences.Shortcut = old
					saveErr := a.savePreferencesLocked()
					a.mu.Unlock()
					request.result <- errors.Join(err, saveErr)
					continue
				}
			}
			active = next
			spec = request.spec
			a.mu.Lock()
			a.status.Shortcut = spec.Label
			a.status.ShortcutError = ""
			a.mu.Unlock()
			request.result <- nil
		}
	}
}

func (a *App) createShortcut(factory desktop.ShortcutFactory, spec desktop.ShortcutSpec) desktop.ShortcutHandle {
	h := factory(spec)
	if contextual, ok := h.(interface{ SetContext(context.Context) }); ok {
		contextual.SetContext(a.ctx)
	}
	return h
}

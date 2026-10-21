package app

import (
	"context"
	"errors"
	"localens/internal/catalog"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ChooseDirectory opens the native picker and starts indexing only the selected directory.
func (a *App) ChooseDirectory() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择要索引的文件夹"})
	if err != nil || path == "" {
		return "", err
	}
	if err = a.store.AddRoot(a.ctx, path); err != nil {
		return "", err
	}
	a.beginScan(true)
	return path, nil
}

// RemoveDirectory clears a directory's index while preserving all source files.
func (a *App) RemoveDirectory(path string) error {
	if err := a.ready(); err != nil {
		return err
	}
	a.StopScan()
	return a.store.RemoveRoot(a.ctx, path)
}

// Rescan starts a user-requested incremental scan.
func (a *App) Rescan() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.beginScan(true)
	return nil
}
func (a *App) beginScan(user bool) {
	if a.store == nil {
		return
	}
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return
	}
	if a.status.Scanning || a.modelWorking {
		if user {
			a.scanRequested = true
		}
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.scanCancel = cancel
	a.scanRequested = false
	a.status.Scanning = true
	a.wg.Add(1)
	a.status.Progress = catalog.ScanProgress{Warnings: []string{}}
	if user {
		a.status.Error = ""
	}
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		defer cancel()
		err := a.store.Scan(ctx, func(p catalog.ScanProgress) { a.mu.Lock(); a.status.Progress = p; a.mu.Unlock() })
		a.mu.Lock()
		a.status.Scanning = false
		a.scanCancel = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			a.status.Error = err.Error()
		}
		if err == nil {
			a.status.LastScan = time.Now().Format("15:04:05")
		}
		modelReady := a.status.Model == "ready"
		requested := a.scanRequested
		a.mu.Unlock()
		if requested {
			a.beginScan(true)
		} else if err == nil && modelReady {
			a.beginEmbedding()
		}
	}()
}

// StopScan cancels the active scan; existing entries remain searchable.
func (a *App) StopScan() {
	a.mu.Lock()
	cancel := a.scanCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RetryImageOCR restarts offline recognition for indexed local images.
func (a *App) RetryImageOCR() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.mu.Lock()
	enabled := a.preferences.OCREnabled
	a.mu.Unlock()
	if !enabled {
		return errors.New("enable image OCR before retrying recognition")
	}
	return a.store.RetryImageOCR(a.ctx)
}

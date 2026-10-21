package app

import (
	"context"
	"time"
)

// GetThumbnail returns a bounded preview only for an unchanged, locally available
// image within the indexed scope. No file:// URLs or full-resolution bytes reach rows.
func (a *App) GetThumbnail(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	if _, err := a.store.VerifiedLocalImage(ctx, path); err != nil {
		return "", err
	}
	data, err := a.thumbnails.Get(ctx, path)
	if err != nil {
		return "", err
	}
	if _, err = a.store.VerifiedLocalImage(ctx, path); err != nil {
		return "", err
	}
	return data, nil
}

package catalog

import (
	"context"
	"errors"
	"os"
)

// VerifiedLocalImage rejects stale/unindexed paths and cloud-only placeholders
// before thumbnail generation can open and inadvertently download their content.
func (s *Store) VerifiedLocalImage(ctx context.Context, path string) (File, error) {
	f, err := s.VerifiedPath(ctx, path)
	if err != nil {
		return f, err
	}
	if f.Kind != "image" {
		return f, errors.New("not an indexed image")
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		return f, err
	}
	if isPlaceholder(info) {
		return f, errors.New("image is not available locally")
	}
	if info.Size() != f.Size || info.ModTime().UnixNano() != f.Modified {
		return f, errors.New("image changed; waiting for index refresh")
	}
	return f, nil
}

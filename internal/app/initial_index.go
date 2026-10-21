package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"localens/internal/desktop"
	"os"
	"path/filepath"
	"slices"
)

type defaultScope struct {
	SeenVolumes []string `json:"seenVolumes"`
}

// initializeIndexScope upgrades Desktop defaults and discovers new local disks.
// Seen volumes are never restored after the user removes them from settings.
func (a *App) initializeIndexScope() error {
	marker := filepath.Join(a.dataDir, "default-scope-v2")
	saved, err := os.ReadFile(marker)
	configured := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var scope defaultScope
	if configured && len(saved) > 0 && saved[0] == '{' {
		if err = json.Unmarshal(saved, &scope); err != nil {
			return err
		}
	}
	if !configured {
		home := os.Getenv("LOCALENS_INDEX_HOME")
		if home == "" {
			home, err = os.UserHomeDir()
			if err != nil {
				return err
			}
		}
		if err = a.store.ExpandRoot(a.ctx, home); err != nil {
			return fmt.Errorf("无法自动索引用户目录：%w", err)
		}
	}
	volumes := desktop.LocalVolumeDirectories()
	if override := os.Getenv("LOCALENS_INDEX_VOLUMES"); override != "" {
		volumes = filepath.SplitList(override)
	} else if os.Getenv("LOCALENS_INDEX_HOME") != "" {
		volumes = nil
	}
	changed := !configured
	for _, volume := range volumes {
		canonical, err := filepath.EvalSymlinks(volume)
		if err != nil {
			continue
		}
		if slices.Contains(scope.SeenVolumes, canonical) {
			continue
		}
		if err = a.store.ExpandRoot(a.ctx, canonical); err != nil {
			return fmt.Errorf("无法索引本地磁盘 %s：%w", canonical, err)
		}
		scope.SeenVolumes = append(scope.SeenVolumes, canonical)
		changed = true
	}
	if !changed {
		return nil
	}
	data, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	temporary := marker + ".tmp"
	if err = os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, marker)
}

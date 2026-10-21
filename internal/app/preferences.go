package app

import (
	"encoding/json"
	"errors"
	"localens/internal/catalog"
	"localens/internal/desktop"
	"os"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type preferences struct {
	Shortcut        string `json:"shortcut,omitempty"`
	Editor          string `json:"editor,omitempty"`
	Language        string `json:"language"`
	SemanticEnabled bool   `json:"semanticEnabled"`
	OCREnabled      bool   `json:"ocrEnabled"`
	Python          string `json:"python,omitempty"`
}

func loadPreferences(path string) (preferences, error) {
	p := preferences{Language: "zh-CN", SemanticEnabled: true, Shortcut: "Ctrl+Q"}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	var saved struct {
		Shortcut        string `json:"shortcut"`
		Editor          string `json:"editor"`
		Language        string `json:"language"`
		SemanticEnabled *bool  `json:"semanticEnabled"`
		OCREnabled      bool   `json:"ocrEnabled"`
		Python          string `json:"python"`
	}
	if err = json.Unmarshal(b, &saved); err != nil {
		return p, err
	}
	if saved.Language == "en" || saved.Language == "zh-CN" {
		p.Language = saved.Language
	}
	if saved.SemanticEnabled != nil {
		p.SemanticEnabled = *saved.SemanticEnabled
	}
	if spec, err := desktop.ParseShortcut(saved.Shortcut); err == nil {
		p.Shortcut = spec.Canonical
	}
	p.OCREnabled = saved.OCREnabled
	p.Editor = saved.Editor
	p.Python = saved.Python
	return p, nil
}
func (a *App) savePreferencesLocked() error {
	b, err := json.Marshal(a.preferences)
	if err != nil {
		return err
	}
	path := filepath.Join(a.dataDir, "settings.json")
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
	}
	return err
}

// SetLanguage persists and immediately applies an interface language.
func (a *App) SetLanguage(language string) error {
	if language != "zh-CN" && language != "en" {
		return errors.New("unsupported language")
	}
	a.mu.Lock()
	old := a.preferences.Language
	a.preferences.Language = language
	if err := a.savePreferencesLocked(); err != nil {
		a.preferences.Language = old
		a.mu.Unlock()
		return err
	}
	a.status.Language = language
	desktopReady := a.desktopReady
	a.mu.Unlock()
	if a.ctx != nil && desktopReady {
		wailsruntime.WindowSetTitle(a.ctx, windowTitle(language))
		wailsruntime.MenuSetApplicationMenu(a.ctx, a.applicationMenu(language))
	}
	if desktopReady {
		desktop.SetTrayLanguage(language == "en")
	}
	return nil
}

// SetSemanticEnabled persists semantic search and manages the bundled local engine.
func (a *App) SetSemanticEnabled(enabled bool) error {
	a.mu.Lock()
	old := a.preferences.SemanticEnabled
	a.preferences.SemanticEnabled = enabled
	if err := a.savePreferencesLocked(); err != nil {
		a.preferences.SemanticEnabled = old
		a.mu.Unlock()
		return err
	}
	a.status.SemanticEnabled = enabled
	model := a.status.Model
	working := a.modelWorking
	if !enabled {
		a.restartModelRequested = false
	} else if working && model != "ready" {
		a.restartModelRequested = true
	}
	a.mu.Unlock()
	if !enabled {
		a.StopModel()
		return nil
	}
	if working && model != "ready" {
		return nil
	}
	if model == "off" || model == "error" {
		return a.StartModel("")
	}
	return nil
}

// SetOCREnabled persists the independent, opt-in image recognition preference.
func (a *App) SetOCREnabled(enabled bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	if enabled && !catalog.OCRAvailable() {
		return errors.New("image OCR is unavailable on this platform")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.preferences.OCREnabled
	a.preferences.OCREnabled = enabled
	if err := a.savePreferencesLocked(); err != nil {
		a.preferences.OCREnabled = old
		return err
	}
	a.store.SetOCREnabled(enabled)
	a.status.OCREnabled = enabled
	return nil
}

package app

import (
	"context"
	"errors"
	"localens/internal/catalog"
	"localens/internal/embedding"
	"localens/internal/thumbnail"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// App exposes local desktop operations to the Wails frontend.
type App struct {
	thumbnails            *thumbnail.Cache
	shortcutRequests      chan shortcutRequest
	searchKey             string
	searchEpoch           uint64
	contentSearchCancel   context.CancelFunc
	restartModelRequested bool
	desktopReady          bool
	preferences           preferences
	wg                    sync.WaitGroup
	closing               bool
	scanRequested         bool
	modelGeneration       uint64
	ctx                   context.Context
	cancel                context.CancelFunc
	store                 *catalog.Store
	worker                *embedding.Worker
	mu                    sync.Mutex
	status                Status
	scanCancel            context.CancelFunc
	modelCancel           context.CancelFunc
	modelWorking          bool
	dataDir               string
}

// Status describes persistent services and background activity.
type Status struct {
	Editor          string               `json:"editor"`
	Language        string               `json:"language"`
	SemanticEnabled bool                 `json:"semanticEnabled"`
	OCREnabled      bool                 `json:"ocrEnabled"`
	BundledModel    bool                 `json:"bundledModel"`
	Shortcut        string               `json:"shortcut"`
	ShortcutError   string               `json:"shortcutError"`
	Roots           []catalog.Root       `json:"roots"`
	Stats           catalog.Stats        `json:"stats"`
	Scanning        bool                 `json:"scanning"`
	Progress        catalog.ScanProgress `json:"progress"`
	Model           string               `json:"model"`
	ModelError      string               `json:"modelError"`
	Embedding       bool                 `json:"embedding"`
	Embedded        int                  `json:"embedded"`
	Python          string               `json:"python"`
	Error           string               `json:"error"`
	LastScan        string               `json:"lastScan"`
}

// Preview contains a bounded local preview, with no remote resources.
type Preview struct {
	File  catalog.File `json:"file"`
	Text  string       `json:"text"`
	Image string       `json:"image"`
}

// NewApp initializes a stopped desktop application.
func NewApp() *App {
	return &App{thumbnails: thumbnail.NewCache(), shortcutRequests: make(chan shortcutRequest), worker: embedding.New(), preferences: preferences{Language: "zh-CN", SemanticEnabled: true}, status: Status{Model: "off", Roots: []catalog.Root{}, Language: "zh-CN", SemanticEnabled: true}}
}
func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	dir, err := os.UserConfigDir()
	if err == nil {
		a.dataDir = filepath.Join(dir, "Localens")
		if override := os.Getenv("LOCALENS_DATA_DIR"); override != "" {
			a.dataDir = override
		}
		a.store, err = catalog.Open(filepath.Join(a.dataDir, "index.db"))
	}
	if err != nil {
		a.mu.Lock()
		a.status.Error = err.Error()
		a.mu.Unlock()
		return
	}
	if err := a.initializeIndexScope(); err != nil {
		a.status.Error = err.Error()
	}
	p, prefErr := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if prefErr != nil {
		a.status.Error = prefErr.Error()
	}
	a.preferences = p
	a.status.Language = p.Language
	a.status.SemanticEnabled = p.SemanticEnabled
	a.status.OCREnabled = p.OCREnabled
	a.store.SetOCREnabled(p.OCREnabled)
	a.status.BundledModel = bundledModelAvailable()
	a.status.Python = a.defaultPython()
	if !a.status.BundledModel && p.Python != "" {
		a.status.Python = p.Python
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				if err := a.initializeIndexScope(); err != nil {
					a.mu.Lock()
					a.status.Error = err.Error()
					a.mu.Unlock()
				}
				a.beginScan(false)
			}
		}
	}()
	a.beginScan(false)
	if a.status.BundledModel && a.preferences.SemanticEnabled {
		_ = a.StartModel("")
	}
}
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
	a.worker.Close()
	a.mu.Lock()
	cancel := a.scanCancel
	modelCancel := a.modelCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if modelCancel != nil {
		modelCancel()
	}
	a.wg.Wait()
	if a.store != nil {
		_ = a.store.Close()
	}
}
func (a *App) defaultPython() string {
	paths := []string{bundledPython()}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(dir, "..", "Resources", "python", "bin", "python3"), filepath.Join(dir, ".venv", "bin", "python"), filepath.Join(dir, "..", "Resources", ".venv", "bin", "python"), filepath.Join(dir, "..", "..", "..", "..", "..", ".venv", "bin", "python"), filepath.Join(dir, "..", "..", ".venv", "Scripts", "python.exe"))
	}
	cwd, _ := os.Getwd()
	paths = append(paths, filepath.Join(cwd, ".venv", "bin", "python"), filepath.Join(cwd, ".venv", "Scripts", "python.exe"))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python3"
}
func (a *App) ready() error {
	if a.store == nil {
		return errors.New("索引数据库未就绪")
	}
	return nil
}

// GetStatus returns an immutable snapshot for UI polling.
func (a *App) GetStatus() (Status, error) {
	a.mu.Lock()
	v := a.status
	v.Editor = a.preferences.Editor
	v.Progress.Warnings = append([]string{}, a.status.Progress.Warnings...)
	a.mu.Unlock()
	if err := a.ready(); err != nil {
		return v, err
	}
	var err error
	v.Roots, err = a.store.Roots(a.ctx)
	if err != nil {
		return v, err
	}
	v.Stats, err = a.store.Statistics(a.ctx)
	return v, err
}

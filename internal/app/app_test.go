package app

import (
	"context"
	"localens/internal/catalog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func appFixture(t *testing.T) *App {
	t.Helper()
	a := NewApp()
	a.ctx, a.cancel = context.WithCancel(t.Context())
	a.dataDir = t.TempDir()
	var err error
	a.store, err = catalog.Open(filepath.Join(a.dataDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a
}
func waitApp(t *testing.T, a *App, condition func(Status, bool) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		s, working := a.status, a.modelWorking
		a.mu.Unlock()
		if condition(s, working) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background state did not settle")
}
func TestStopModelLoadingKeepsOffState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake interpreter")
	}
	a := appFixture(t)
	fake := filepath.Join(t.TempDir(), "fake-python")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 2\nprintf '{\"ready\":true}\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.StartModel(fake); err != nil {
		t.Fatal(err)
	}
	a.StopModel()
	waitApp(t, a, func(s Status, w bool) bool { return !w })
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status.Model != "off" || a.status.ModelError != "" {
		t.Fatalf("canceled loading overwritten: %#v", a.status)
	}
}
func TestDeadWorkerMarksModelErrorAndAllowsRestart(t *testing.T) {
	a := appFixture(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip(err)
	}
	script := filepath.Join(t.TempDir(), "dies.py")
	if err = os.WriteFile(script, []byte("import sys\nprint('{\"ready\":true}',flush=True)\nsys.stdin.readline()\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err = a.worker.Start(a.ctx, ctx, python, script); err != nil {
		t.Fatal(err)
	}
	a.status.Model = "ready"
	result, err := a.Search("something", "all", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning == "" {
		t.Fatal("inference failure was hidden")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status.Model != "error" {
		t.Fatalf("dead worker remained %s", a.status.Model)
	}
}
func TestQueuedScanAfterModelWork(t *testing.T) {
	a := appFixture(t)
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.AddRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "queued.txt"), []byte("queued"), 0600); err != nil {
		t.Fatal(err)
	}
	a.modelWorking = true
	if err = a.Rescan(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	if !a.scanRequested {
		t.Fatal("scan request lost")
	}
	a.modelWorking = false
	a.mu.Unlock()
	a.beginScan(true)
	waitApp(t, a, func(s Status, w bool) bool { return !s.Scanning })
	stats, err := a.store.Statistics(t.Context())
	if err != nil || stats.Files != 1 {
		t.Fatalf("queued scan %v %v", stats, err)
	}
}

// TestRealLocalModel exercises persistence and retrieval with real weights when explicitly enabled.
func TestRealLocalModel(t *testing.T) {
	python := os.Getenv("LOCALENS_TEST_PYTHON")
	if python == "" {
		t.Skip("set LOCALENS_TEST_PYTHON to run the real model integration test")
	}
	t.Setenv("HF_HUB_OFFLINE", "1")
	t.Setenv("TRANSFORMERS_OFFLINE", "1")
	a := appFixture(t)
	root := t.TempDir()
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.AddRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"租赁合同.md": "租期结束完成退房验收后，七日内退还押金。", "数据库笔记.txt": "为慢查询增加索引，减少扫描数据量。"} {
		if err = os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if image := os.Getenv("LOCALENS_TEST_IMAGE"); image != "" {
		data, err := os.ReadFile(image)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "capture.png"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.store.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err = a.StartModel(python); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		s, working := a.status, a.modelWorking
		a.mu.Unlock()
		if s.Model == "error" {
			t.Fatal(s.ModelError)
		}
		stats, err := a.store.Statistics(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if s.Model == "ready" && !working && stats.Vectors == stats.Chunks {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	status, err := a.GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Model != "ready" || status.Embedding || status.Stats.Vectors != status.Stats.Chunks || status.Stats.Vectors < 2 {
		t.Fatalf("real index not ready: %#v", status)
	}
	result, err := a.Search("租房押金什么时候退还", "all", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Semantic || result.Warning != "" || len(result.Files) == 0 || result.Files[0].Name != "租赁合同.md" {
		t.Fatalf("real semantic retrieval: %#v", result)
	}
	if os.Getenv("LOCALENS_TEST_IMAGE") != "" {
		result, err = a.Search("一个搜索框的截图", "image", true)
		if err != nil || !result.Semantic || len(result.Files) != 1 || result.Files[0].Name != "capture.png" {
			t.Fatalf("real image retrieval: %#v %v", result, err)
		}
	}
	t.Logf("real index: %d files, %d normalized vectors; text and image retrieval passed", status.Stats.Files, status.Stats.Vectors)
}

func TestFirstLaunchIndexesHomeFilename(t *testing.T) {
	desktop := t.TempDir()
	path := filepath.Join(desktop, "The_Telegraph_SLI_ebook.pdf")
	if err := os.WriteFile(path, []byte("fixture metadata only"), 0600); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(desktop, "Downloads")
	if err := os.Mkdir(downloads, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(downloads, "Out Of Context.mp4"), []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALENS_DATA_DIR", t.TempDir())
	t.Setenv("LOCALENS_INDEX_HOME", desktop)
	a := NewApp()
	a.startup(t.Context())
	t.Cleanup(func() { a.shutdown(context.Background()) })
	waitApp(t, a, func(s Status, _ bool) bool { return !s.Scanning })
	result, err := a.Search("the_telegraph", "all", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Name != "The_Telegraph_SLI_ebook.pdf" {
		t.Fatalf("first launch should find an existing home file, got %#v", result.Files)
	}
	result, err = a.Search("Out", "video", false)
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("nested download video not found: %#v %v", result, err)
	}
}

func TestDefaultHomeNotRestoredAfterRemoval(t *testing.T) {
	a := appFixture(t)
	desktop := t.TempDir()
	t.Setenv("LOCALENS_INDEX_HOME", desktop)
	if err := a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	roots, err := a.store.Roots(t.Context())
	if err != nil || len(roots) != 1 {
		t.Fatalf("initial scope: %#v %v", roots, err)
	}
	if err = a.store.RemoveRoot(t.Context(), roots[0].Path); err != nil {
		t.Fatal(err)
	}
	if err = a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	roots, err = a.store.Roots(t.Context())
	if err != nil || len(roots) != 0 {
		t.Fatalf("removed Desktop was restored: %#v %v", roots, err)
	}
}

func TestDefaultScopeMigratesDesktopWithoutLosingFiles(t *testing.T) {
	a := appFixture(t)
	home := t.TempDir()
	desktop := filepath.Join(home, "Desktop")
	if err := os.Mkdir(desktop, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "existing.txt"), []byte("retained contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddRoot(t.Context(), desktop); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.dataDir, "initial-index-configured"), []byte("configured"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALENS_INDEX_HOME", home)
	if err := a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	roots, err := a.store.Roots(t.Context())
	if err != nil || len(roots) != 1 || roots[0].Count != 1 {
		t.Fatalf("migration lost scope/files: %#v %v", roots, err)
	}
	result, err := a.Search("retained", "all", true)
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("migration lost body: %#v %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(home, "Out Of Context Football.mp4"), []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	result, err = a.Search("Out", "video", false)
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("home video not found: %#v %v", result, err)
	}
}

func TestLanguageAndSemanticPreferencesPersistTogether(t *testing.T) {
	a := appFixture(t)
	a.status.Model = "ready"
	if err := a.SetLanguage("en"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSemanticEnabled(false); err != nil {
		t.Fatal(err)
	}
	saved, err := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Language != "en" || saved.SemanticEnabled {
		t.Fatalf("preferences not preserved together: %#v", saved)
	}
	if err = a.SetLanguage("zh-CN"); err != nil {
		t.Fatal(err)
	}
	saved, err = loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || saved.Language != "zh-CN" || saved.SemanticEnabled {
		t.Fatalf("language switch reset model preference: %#v %v", saved, err)
	}
	if err = a.SetLanguage("invalid"); err == nil {
		t.Fatal("invalid locale accepted")
	}
}

func TestLegacyPreferencesRetainDefaultModelPreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"python":"/old/developer/python"}`), 0600); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Language != "zh-CN" || !prefs.SemanticEnabled {
		t.Fatalf("legacy preference migration: %#v", prefs)
	}
}

func TestSemanticReenableWhileLoadingCancellationRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake interpreter")
	}
	a := appFixture(t)
	fake := filepath.Join(t.TempDir(), "fake-python")
	body := "#!/bin/sh\nsleep 0.2\nprintf '{\"ready\":true}\\n'\nwhile read line; do printf '{\"vector\":[1]}\\n'; done\n"
	if err := os.WriteFile(fake, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.StartModel(fake); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSemanticEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSemanticEnabled(true); err != nil {
		t.Fatal(err)
	}
	waitApp(t, a, func(s Status, w bool) bool { return s.Model == "ready" && !w })
	saved, err := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || !saved.SemanticEnabled {
		t.Fatalf("enabled preference lost: %#v %v", saved, err)
	}
}

func TestNewVolumeDiscoveredAndRemovedVolumeNotRestored(t *testing.T) {
	a := appFixture(t)
	t.Setenv("LOCALENS_INDEX_HOME", t.TempDir())
	if err := a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	volume := t.TempDir()
	t.Setenv("LOCALENS_INDEX_VOLUMES", volume)
	if err := a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	roots, err := a.store.Roots(t.Context())
	if err != nil || len(roots) != 2 {
		t.Fatalf("new volume not discovered: %#v %v", roots, err)
	}
	canonical, err := filepath.EvalSymlinks(volume)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.RemoveRoot(t.Context(), canonical); err != nil {
		t.Fatal(err)
	}
	if err := a.initializeIndexScope(); err != nil {
		t.Fatal(err)
	}
	roots, err = a.store.Roots(t.Context())
	if err != nil || len(roots) != 1 {
		t.Fatalf("removed volume restored: %#v %v", roots, err)
	}
}

func TestKeywordContentSearchDoesNotRequireModel(t *testing.T) {
	a := appFixture(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("多词查询优先匹配连续短语"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.AddRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	result, err := a.SearchKeywords("多词查询优先", "all")
	if err != nil || len(result.Files) != 1 || result.Semantic || result.Warning != "" {
		t.Fatalf("keywords: %#v %v", result, err)
	}
	if result.Files[0].Match != "正文" {
		t.Fatalf("missing literal evidence: %#v", result.Files[0])
	}
}

func TestOCRPreferenceDefaultsOffAndPersistsIndependently(t *testing.T) {
	a := appFixture(t)
	if a.preferences.OCREnabled {
		t.Fatal("OCR must default off")
	}
	if err := a.RetryImageOCR(); err == nil {
		t.Fatal("disabled retry allowed")
	}
	if !catalog.OCRAvailable() {
		if err := a.SetOCREnabled(true); err == nil {
			t.Fatal("unsupported OCR enabled")
		}
		return
	}
	if err := a.SetOCREnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLanguage("en"); err != nil {
		t.Fatal(err)
	}
	saved, err := loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || !saved.OCREnabled || !saved.SemanticEnabled || saved.Language != "en" {
		t.Fatalf("save: %#v %v", saved, err)
	}
	if err = a.SetOCREnabled(false); err != nil {
		t.Fatal(err)
	}
	saved, err = loadPreferences(filepath.Join(a.dataDir, "settings.json"))
	if err != nil || saved.OCREnabled || a.status.OCREnabled {
		t.Fatalf("pause: %#v %v", saved, err)
	}
}

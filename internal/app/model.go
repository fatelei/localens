package app

import (
	"context"
	"errors"
	"fmt"
	"localens/engine"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StartModel explicitly loads the local model using the selected Python interpreter.
func (a *App) StartModel(python string) error {
	if err := a.ready(); err != nil {
		return err
	}
	python = strings.TrimSpace(python)
	if python == "" {
		a.mu.Lock()
		python = a.status.Python
		a.mu.Unlock()
		if python == "" || bundledModelAvailable() {
			python = a.defaultPython()
		}
	}
	resolved, err := exec.LookPath(python)
	if err != nil {
		return fmt.Errorf("找不到 Python：%w", err)
	}
	a.mu.Lock()
	if a.closing || a.modelWorking || a.status.Model == "loading" || a.status.Model == "ready" {
		a.mu.Unlock()
		return errors.New("模型已经启动或正在工作")
	}
	oldPython := a.preferences.Python
	a.preferences.Python = resolved
	if a.status.BundledModel {
		a.preferences.Python = ""
	}
	if err := a.savePreferencesLocked(); err != nil {
		a.preferences.Python = oldPython
		a.mu.Unlock()
		return err
	}
	a.modelGeneration++
	generation := a.modelGeneration
	a.wg.Add(1)
	a.status.Model = "loading"
	a.status.ModelError = ""
	a.status.Python = resolved
	a.modelWorking = true
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Minute)
	a.modelCancel = cancel
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		defer cancel()
		script := filepath.Join(a.dataDir, "worker.py")
		err := os.WriteFile(script, engine.WorkerScript, 0600)
		if err == nil {
			err = a.worker.Start(a.ctx, ctx, resolved, script)
		}
		a.mu.Lock()
		a.modelWorking = false
		a.modelCancel = nil
		if a.modelGeneration == generation && a.status.Model == "loading" {
			if err != nil {
				a.status.Model = "error"
				a.status.ModelError = err.Error()
			} else {
				a.status.Model = "ready"
			}
		}
		requested := a.scanRequested
		restart := a.restartModelRequested && a.preferences.SemanticEnabled && !a.closing
		a.restartModelRequested = false
		a.mu.Unlock()
		if restart {
			_ = a.StartModel("")
			return
		}
		if requested {
			a.beginScan(true)
		} else if err == nil {
			a.beginEmbedding()
		}
	}()
	return nil
}

// StopModel cancels model loading or embedding and unloads the local process.
func (a *App) StopModel() {
	a.mu.Lock()
	cancel := a.modelCancel
	a.modelGeneration++
	a.status.Model = "off"
	a.status.ModelError = ""
	if cancel != nil {
		cancel()
	}
	a.worker.Close()
	a.mu.Unlock()
}

// RebuildVectors retries pending semantic chunks without rescanning source files.
func (a *App) RebuildVectors() error {
	a.mu.Lock()
	ready := a.status.Model == "ready"
	a.mu.Unlock()
	if !ready {
		return errors.New("请先启用本地模型")
	}
	a.beginEmbedding()
	return nil
}
func (a *App) beginEmbedding() {
	a.mu.Lock()
	if a.closing || a.modelWorking || a.status.Scanning || a.status.Model != "ready" {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.modelCancel = cancel
	a.modelWorking = true
	a.status.Embedding = true
	a.status.Embedded = 0
	generation := a.modelGeneration
	a.wg.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		defer cancel()
		err := a.store.EmbedPending(ctx, a.worker, func(n int) { a.mu.Lock(); a.status.Embedded = n; a.mu.Unlock() })
		a.mu.Lock()
		a.modelWorking = false
		a.status.Embedding = false
		a.modelCancel = nil
		if err != nil && !errors.Is(err, context.Canceled) && a.modelGeneration == generation && a.status.Model == "ready" {
			a.status.ModelError = err.Error()
			if !a.worker.Running() {
				a.status.Model = "error"
			}
		}
		requested := a.scanRequested
		restart := a.restartModelRequested && a.preferences.SemanticEnabled && !a.closing
		a.restartModelRequested = false
		a.mu.Unlock()
		if restart {
			_ = a.StartModel("")
			return
		}
		if requested {
			a.beginScan(true)
		}
	}()
}

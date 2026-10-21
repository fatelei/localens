// Package embedding controls a local JSONL inference subprocess.
package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Worker serializes requests to a persistent model process.
type Worker struct {
	gate    chan struct{}
	mu      sync.Mutex
	cmd     *exec.Cmd
	input   io.WriteCloser
	replies chan response
	done    chan struct{}
	cancel  context.CancelFunc
}
type response struct {
	Ready  bool      `json:"ready"`
	Error  string    `json:"error"`
	Vector []float32 `json:"vector"`
}

// New returns a stopped worker.
func New() *Worker { return &Worker{gate: make(chan struct{}, 1)} }
func (w *Worker) acquire(ctx context.Context) error {
	select {
	case w.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *Worker) release() { <-w.gate }

// Bundled Windows Python lives directly in python/, Unix Python in python/bin/.
func bundledRuntimeRoot(python string) string {
	dir := filepath.Dir(python)
	if filepath.Base(dir) == "python" {
		return dir
	}
	return filepath.Dir(dir)
}

// Start loads the model. The process dies when the application lifetime ends.
func (w *Worker) Start(lifetime, ctx context.Context, python, script string) error {
	if err := w.acquire(ctx); err != nil {
		return err
	}
	defer w.release()
	w.stop()
	processCtx, cancel := context.WithCancel(lifetime)
	cmd := exec.CommandContext(processCtx, python, "-u", script)
	runtimeRoot := bundledRuntimeRoot(python)
	resources := filepath.Dir(runtimeRoot)
	modelRoot := filepath.Join(resources, "model")
	if _, err := os.Stat(filepath.Join(modelRoot, "model.safetensors")); err == nil && filepath.Base(runtimeRoot) == "python" {
		env := []string{}
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			if key == "PYTHONHOME" || key == "PYTHONPATH" || key == "LOCALENS_MODEL_PATH" || key == "HF_HUB_OFFLINE" || key == "TRANSFORMERS_OFFLINE" {
				continue
			}
			env = append(env, value)
		}
		cmd.Env = append(env, "PYTHONHOME="+runtimeRoot, "LOCALENS_MODEL_PATH="+modelRoot, "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "PYTHONNOUSERSITE=1", "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1", "HF_HUB_DISABLE_TELEMETRY=1")
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		input.Close()
		return err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		cancel()
		input.Close()
		return err
	}
	replies := make(chan response, 1)
	done := make(chan struct{})
	w.mu.Lock()
	w.cmd = cmd
	w.input = input
	w.replies = replies
	w.done = done
	w.cancel = cancel
	w.mu.Unlock()
	go func() {
		defer close(done)
		defer func() { _ = cmd.Wait() }()
		defer close(replies)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			var r response
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				r.Error = "推理协议错误: " + err.Error()
			}
			select {
			case replies <- r:
			case <-processCtx.Done():
				return
			}
		}
	}()

	select {
	case r, ok := <-replies:
		if !ok {
			w.stop()
			return errors.New("推理进程提前退出，请检查 Python 依赖")
		}
		if r.Error != "" {
			w.stop()
			return errors.New(r.Error)
		}
		if !r.Ready {
			w.stop()
			return errors.New("推理进程未就绪")
		}
		return nil
	case <-ctx.Done():
		w.stop()
		return ctx.Err()
	}
}
func (w *Worker) stop() {
	w.mu.Lock()
	cancel := w.cancel
	input := w.input
	w.cancel = nil
	w.cmd = nil
	w.input = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if input != nil {
		_ = input.Close()
	}
}

// Close terminates model inference without waiting on the request lock.
func (w *Worker) Close() { w.stop() }

// Running reports whether the worker is still available for requests.
func (w *Worker) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.input == nil {
		return false
	}
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// Encode runs one bounded request; a timeout kills the process to avoid protocol desynchronization.
func (w *Worker) Encode(ctx context.Context, kind, path, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := w.acquire(ctx); err != nil {
		return nil, err
	}
	defer w.release()
	w.mu.Lock()
	input, replies := w.input, w.replies
	w.mu.Unlock()
	if input == nil {
		return nil, errors.New("本地模型未启动")
	}
	request := map[string]string{"kind": kind, "path": path, "text": text}
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if _, err = input.Write(append(b, '\n')); err != nil {
		w.stop()
		return nil, err
	}
	select {
	case r, ok := <-replies:
		if !ok {
			w.stop()
			return nil, errors.New("推理进程已退出")
		}
		if r.Error != "" {
			return nil, fmt.Errorf("本地推理: %s", strings.TrimSpace(r.Error))
		}
		return r.Vector, nil
	case <-ctx.Done():
		w.stop()
		return nil, ctx.Err()
	}
}

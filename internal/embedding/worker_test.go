package embedding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func python(t *testing.T) string {
	t.Helper()
	name := "python3"
	if runtime.GOOS == "windows" {
		name = "python"
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s missing", name)
	}
	return p
}
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker.py")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestWorkerProtocolAndShutdown(t *testing.T) {
	w := New()
	t.Cleanup(w.Close)
	s := script(t, "import json,sys\nprint(json.dumps({'ready':True}),flush=True)\nfor line in sys.stdin:\n req=json.loads(line)\n print(json.dumps({'vector':[1.0]+[0.0]*255}),flush=True)\n")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := w.Start(t.Context(), ctx, python(t), s); err != nil {
		t.Fatal(err)
	}
	v, err := w.Encode(ctx, "query", "", "test")
	if err != nil || len(v) != 256 {
		t.Fatalf("response %d %v", len(v), err)
	}
	w.Close()
	if _, err = w.Encode(ctx, "query", "", "test"); err == nil {
		t.Fatal("stopped worker accepted request")
	}
}
func TestWorkerLoadFailure(t *testing.T) {
	w := New()
	defer w.Close()
	s := script(t, "print('{\"error\":\"dependency missing\"}',flush=True)\n")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := w.Start(t.Context(), ctx, python(t), s); err == nil || !strings.Contains(err.Error(), "dependency missing") {
		t.Fatalf("load error: %v", err)
	}
}
func TestWorkerCancellationKillsBlockedProcess(t *testing.T) {
	w := New()
	defer w.Close()
	s := script(t, "import time,sys\nprint('{\"ready\":true}',flush=True)\nfor line in sys.stdin: time.sleep(10)\n")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := w.Start(t.Context(), ctx, python(t), s); err != nil {
		t.Fatal(err)
	}
	requestCtx, requestCancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer requestCancel()
	start := time.Now()
	if _, err := w.Encode(requestCtx, "query", "", "x"); err == nil {
		t.Fatal("canceled request succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation was not prompt")
	}
}

func TestBundledRuntimeRootHandlesWindowsAndUnixLayouts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Resources", "python")
	for _, python := range []string{filepath.Join(root, "python.exe"), filepath.Join(root, "bin", "python3")} {
		if got := bundledRuntimeRoot(python); got != root {
			t.Fatalf("got %q want %q", got, root)
		}
	}
}

package thumbnail

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, width, height int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture.png")
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.White)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}
func TestThumbnailIsSmallAndPreservesAspectRatio(t *testing.T) {
	p := fixture(t, 800, 400)
	data, err := Render(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "data:image/jpeg;base64,"
	if !strings.HasPrefix(data, prefix) {
		t.Fatalf("unexpected format %s", data[:min(30, len(data))])
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, prefix))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 160 || cfg.Height != 80 {
		t.Fatalf("unexpected thumbnail size %+v", cfg)
	}
	if len(b) > 24*1024 {
		t.Fatalf("unbounded thumbnail: %d", len(b))
	}
}
func TestThumbnailRejectsBadSourcesAndCancellation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.png")
	os.WriteFile(p, []byte("not an image"), 0600)
	if _, err := Render(t.Context(), p); err == nil {
		t.Fatal("accepted corrupt image")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Render(ctx, fixture(t, 2, 2)); err == nil {
		t.Fatal("ignored cancellation")
	}
	large := filepath.Join(t.TempDir(), "large.png")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(32*1024*1024 + 1)
	f.Close()
	if _, err := Render(t.Context(), large); err == nil {
		t.Fatal("accepted oversized source")
	}
}

func TestCacheReusesAndInvalidatesImages(t *testing.T) {
	p := fixture(t, 400, 200)
	c := NewCache()
	first, err := c.Get(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Get(t.Context(), p)
	if err != nil || again != first || len(c.entries) != 1 {
		t.Fatal("cache did not reuse image", err)
	}
	if err := os.WriteFile(p, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(t.Context(), p); err == nil {
		t.Fatal("reused stale thumbnail")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(t.Context(), p); err == nil {
		t.Fatal("served deleted source")
	}
}

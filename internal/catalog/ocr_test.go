package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineImageTextRecognitionAndSearch(t *testing.T) {
	if !OCRAvailable() {
		t.Skip("native macOS OCR")
	}
	bytes, err := os.ReadFile("testdata/ocr-text.png")
	if err != nil {
		t.Fatal(err)
	}
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	path := filepath.Join(root, "capture.png")
	if err = os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	scan(t, s)
	recognized, recognitionErr := recognizeImage(t.Context(), path)
	if recognitionErr != nil {
		t.Fatal(recognitionErr)
	}
	if recognized == "" {
		t.Fatal("native OCR returned no text from the authored fixture")
	}
	worked, err := s.indexImageText(t.Context())
	if err != nil || !worked {
		t.Fatalf("OCR: %t %v", worked, err)
	}
	var body string
	var version int
	var failure string
	if err = s.db.QueryRow("SELECT ocr_error FROM files WHERE path=?", path).Scan(&failure); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT body,ocr_version FROM files WHERE path=?", path).Scan(&body, &version); err != nil {
		t.Fatal(err)
	}
	if version != 1 || !strings.Contains(body, "4729") || !strings.Contains(body, "图片文字搜索") {
		t.Fatalf("expected authored fixture text, version=%d text=%q", version, body+" [error: "+failure+"]")
	}
	for _, q := range []string{"4729", "图片文字搜索", "OCR"} {
		r, err := s.Search(t.Context(), q, "image", nil)
		if err != nil || len(r.Files) != 1 || r.Files[0].Match != "正文" {
			t.Fatalf("OCR search %q: %#v %v", q, r, err)
		}
	}
	if _, err = s.db.Exec("UPDATE files SET modified=modified+1 WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT ocr_version FROM files WHERE path=?", path).Scan(&version); err != nil || version != 0 {
		t.Fatalf("changed image not queued: %d %v", version, err)
	}
}

func TestOCRDiscardsChangedImageAndPreservesVectors(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	path := filepath.Join(root, "photo.png")
	put(t, path, "authored mock image")
	scan(t, s)
	var id int64
	if err := s.db.QueryRow("SELECT id FROM chunks WHERE path=?", path).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE chunks SET vector=? WHERE id=?", []byte{1, 2, 3, 4}, id); err != nil {
		t.Fatal(err)
	}
	worked, err := s.processImageText(t.Context(), func(_ context.Context, p string) (string, error) {
		put(t, p, "changed authored mock image with another size")
		return "stale recognized words", nil
	})
	if err != nil || !worked {
		t.Fatalf("work: %t %v", worked, err)
	}
	var body string
	var vector []byte
	if err = s.db.QueryRow("SELECT f.body,c.vector FROM files f JOIN chunks c ON c.path=f.path WHERE f.path=?", path).Scan(&body, &vector); err != nil {
		t.Fatal(err)
	}
	if body != "" || string(vector) != string([]byte{1, 2, 3, 4}) {
		t.Fatal("stale OCR stored or vector altered")
	}
	scan(t, s)
	var version int
	if err = s.db.QueryRow("SELECT ocr_version FROM files WHERE path=?", path).Scan(&version); err != nil || version != 0 {
		t.Fatalf("updated file not requeued: %d %v", version, err)
	}
}

func TestOCRIsOptInAndPauseDiscardsCurrentRecognition(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	put(t, filepath.Join(root, "capture.png"), "authored image fixture")
	scan(t, s)
	calls := 0
	recognize := func(context.Context, string) (string, error) { calls++; return "recognized marker", nil }
	worked, err := s.runImageOCR(t.Context(), recognize)
	if err != nil || worked || calls != 0 {
		t.Fatalf("disabled OCR ran: %t %v %d", worked, err, calls)
	}
	s.SetOCREnabled(true)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := s.runImageOCR(t.Context(), func(context.Context, string) (string, error) {
			close(started)
			<-release
			return "discarded marker", nil
		})
		done <- e
	}()
	<-started
	s.SetOCREnabled(false)
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var body string
	var version int
	if err = s.db.QueryRow("SELECT body,ocr_version FROM files WHERE kind='image'").Scan(&body, &version); err != nil {
		t.Fatal(err)
	}
	if body != "" || version != 0 {
		t.Fatalf("paused recognition committed: %q %d", body, version)
	}
	s.SetOCREnabled(true)
	if worked, err = s.runImageOCR(t.Context(), recognize); err != nil || !worked {
		t.Fatalf("resume: %t %v", worked, err)
	}
	s.SetOCREnabled(false)
	result, err := s.Search(t.Context(), "recognized marker", "image", nil)
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("pause removed existing text: %#v %v", result, err)
	}
}

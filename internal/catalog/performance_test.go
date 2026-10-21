package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilenameReaderDoesNotWaitForBackgroundWriter(t *testing.T) {
	s, root := fixture(t)
	put(t, filepath.Join(root, "Out Of Context.mp4"), "movie")
	scan(t, s)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE files SET body='uncommitted' WHERE root=?", root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := s.SearchNames(ctx, "Out Of", "all")
	if err != nil || len(result.Files) != 1 || result.Files[0].Snippet != "" || result.SemanticEligible {
		t.Fatalf("filename reader blocked or noisy: %#v %v", result, err)
	}
}

func TestSnippetFocusesOnSeparatedQueryWords(t *testing.T) {
	body := strings.Repeat("padding ", 80) + "deposit will be returned after the inspection."
	got := snippet(body, "deposit inspection")
	if !strings.Contains(got, "deposit") || !strings.Contains(got, "inspection") {
		t.Fatalf("evidence omitted keywords: %s", got)
	}
}

func TestRealFilenameLatency(t *testing.T) {
	path := os.Getenv("LOCALENS_PERF_DB")
	if path == "" {
		t.Skip("set LOCALENS_PERF_DB to check a real index")
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stats, err := s.Statistics(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real index: %d files", stats.Files)
	for _, query := range []string{"Out", "Out Of", "The_Telegraph"} {
		start := time.Now()
		result, err := s.SearchNames(t.Context(), query, "all")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%q: %d results, %s", query, len(result.Files), time.Since(start))
		for _, file := range result.Files {
			if file.Match != "文件名" || file.Snippet != "" {
				t.Fatalf("incorrect evidence: %#v", file)
			}
		}
		if query == "Out Of" {
			video, err := s.SearchNames(t.Context(), query, "video")
			if err != nil || len(video.Files) != 1 || !strings.HasPrefix(video.Files[0].Name, "Out Of Context Football") {
				t.Fatalf("unexpected video results: %#v %v", video, err)
			}
		}
	}
}

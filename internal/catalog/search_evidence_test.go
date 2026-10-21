package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelEvidenceBoundsWorkPreservesOrderAndSkipsDeleted(t *testing.T) {
	files := make([]File, 80)
	for i := range files {
		files[i].Path = fmt.Sprint(i)
	}
	var active, peak atomic.Int32
	barrier := make(chan struct{})
	var ready sync.Once
	result, err := parallelEvidence(t.Context(), files, func(ctx context.Context, f *File) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		if n == 4 {
			ready.Do(func() { close(barrier) })
		}
		<-barrier
		if f.Path == "3" {
			return sql.ErrNoRows
		}
		f.Snippet = "evidence " + f.Path
		return nil
	})
	if err != nil || len(result) != 79 || peak.Load() > 4 {
		t.Fatalf("workers/count: %d %d %v", peak.Load(), len(result), err)
	}
	for i, f := range result {
		expected := i
		if i >= 3 {
			expected++
		}
		if f.Path != fmt.Sprint(expected) || f.Snippet != "evidence "+f.Path {
			t.Fatalf("order changed at %d: %+v", i, f)
		}
	}
}
func TestParallelEvidenceStopsOnCanceledQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	_, err := parallelEvidence(ctx, make([]File, 80), func(context.Context, *File) error { calls++; return nil })
	if err != context.Canceled || calls != 0 {
		t.Fatalf("cancellation: %d %v", calls, err)
	}
}
func TestKeywordRankKeepsUnicodePhrasesAndMixedNameBodyTerms(t *testing.T) {
	cases := []struct {
		name, body, query string
		score             int64
	}{
		{"notes.py", "WHILE tRuE:\n pass", "while True", 9},
		{"while.txt", "true comes later", "while true", 2},
		{"notes.txt", "İSTANBUL Привет", "istanbul привет", 9},
		{"100%.txt", "literal_under_score", "100% literal_under", 2},
		{"notes.txt", "while elsewhere True", "while True", 2},
		{"while true.py", "unrelated", "while True", 10},
		{"notes.txt", "true only", "while True", -1},
	}
	for _, c := range cases {
		if got := keywordRank(c.name, c.body, c.query); got != c.score {
			t.Fatalf("%q: %d want %d", c.query, got, c.score)
		}
	}
}

func TestConcurrentPhraseSearchesReleaseMetadataReaders(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	for i := range 100 {
		name := fmt.Sprintf("loop-%03d.py", i)
		if _, err := s.db.Exec("INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,1,?,1)", filepath.Join(root, name), root, name, "while True:\n pass"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	done := make(chan error, 4)
	for range 4 {
		go func() {
			<-start
			r, e := s.Search(ctx, "while True", "all", nil)
			if e == nil && len(r.Files) != 80 {
				e = fmt.Errorf("expected 80 hits, got %d", len(r.Files))
			}
			done <- e
		}()
	}
	close(start)
	for range 4 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
func TestChangedDocumentCannotReturnStaleLiteralEvidence(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	path := filepath.Join(root, "loop.py")
	put(t, path, "while True:\n pass")
	scan(t, s)
	result, err := s.Search(t.Context(), "while True", "all", nil)
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("initial result: %#v %v", result, err)
	}
	if _, err = s.db.Exec("UPDATE files SET body='unrelated new content' WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	err = s.enrichSearchFile(t.Context(), &result.Files[0], "while True", 0, nil, false)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale body evidence accepted: %v", err)
	}
}

func TestPhraseProbeBudgetFallsBackWithoutLosingOldExactHit(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(t.Context(), "INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,?,?,1)")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	for i := range 8194 {
		name := fmt.Sprintf("candidate-%05d.txt", i)
		body := "abcd d e efg efgh"
		modified := int64(100)
		if i == 8193 {
			name = "exact.txt"
			body = "abcd efgh"
			modified = 1
		}
		if _, err = stmt.ExecContext(t.Context(), filepath.Join(root, name), root, name, modified, body); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err := s.Search(t.Context(), "abcd efgh", "document", nil)
	if err != nil || len(r.Files) != 80 || r.Files[0].Name != "exact.txt" {
		t.Fatalf("lost exact phrase beyond probe budget: %#v %v", r, err)
	}
}

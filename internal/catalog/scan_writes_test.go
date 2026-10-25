package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A no-change poll must not dirty document pages merely to record a visit.
func TestUnchangedScanDoesNotWriteWAL(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	for i := range 8 {
		put(t, filepath.Join(root, string(rune('a'+i))+".txt"), strings.Repeat("while True: pass\n", 8192))
	}
	scan(t, s)
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := s.db.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		scan(t, s)
	}
	var after int64
	if err := s.db.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.dbPath + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if after != before || info.Size() != 0 {
		t.Fatalf("unchanged scans wrote %d changes and %d WAL bytes", after-before, info.Size())
	}
	stats, err := s.Statistics(t.Context())
	if err != nil || stats.Files != 8 {
		t.Fatalf("unchanged files removed: %+v %v", stats, err)
	}
	if err := os.Remove(filepath.Join(root, "b.txt")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "new.txt"), "a new document")
	scan(t, s)
	stats, err = s.Statistics(t.Context())
	if err != nil || stats.Files != 8 {
		t.Fatalf("mixed scan cleanup: %+v %v", stats, err)
	}
	if _, err := s.Body(t.Context(), filepath.Join(root, "b.txt")); err == nil {
		t.Fatal("deleted file retained")
	}
}

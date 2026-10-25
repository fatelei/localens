package catalog

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedFragmentsKeepIndependentFilesAndCollectLastReference(t *testing.T) {
	s, root := legacyFixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	body := strings.Repeat("相同内容😀 with a searchable phrase\n", 400)
	for _, n := range []string{"one.py", "two.py"} {
		put(t, filepath.Join(root, n), body)
	}
	scan(t, s)
	if err := s.DeduplicateContent(t.Context()); err != nil {
		t.Fatal(err)
	}
	var refs, parts int
	if err := s.db.QueryRow("SELECT count(*) FROM body_parts").Scan(&parts); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM body_refs").Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if parts == 0 || refs != parts*2 {
		t.Fatalf("not shared: %d parts / %d references", parts, refs)
	}
	for _, n := range []string{"one.py", "two.py"} {
		got, err := s.Body(t.Context(), filepath.Join(root, n))
		if err != nil || got != body {
			t.Fatalf("body mismatch %s: %v", n, err)
		}
	}
	r, err := s.Search(t.Context(), "searchable phrase", "document", nil)
	if err != nil || len(r.Files) != 2 {
		t.Fatalf("independent results: %+v %v", r, err)
	}
	for _, n := range []string{"one.py", "two.py"} {
		if _, err := s.db.Exec("DELETE FROM files WHERE path=?", filepath.Join(root, n)); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow("SELECT count(*) FROM body_parts").Scan(&parts); err != nil {
			t.Fatal(err)
		}
		if n == "one.py" && parts == 0 {
			t.Fatal("shared content deleted too early")
		}
	}
	if parts != 0 {
		t.Fatalf("orphan fragments: %d", parts)
	}
}

func TestSharedMigrationReopenEditsAndBoundaryPhrase(t *testing.T) {
	s, root := legacyFixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	body := strings.Repeat("原文😀 line of source code\n", 3000)
	path := filepath.Join(root, "source.py")
	put(t, path, body)
	scan(t, s)
	batch, err := s.PendingChunks(t.Context())
	if err != nil || len(batch) == 0 {
		t.Fatal(err)
	}
	v := make([]float32, 256)
	v[0] = 1
	if err = s.SaveVector(t.Context(), batch[0].ID, v); err != nil {
		t.Fatal(err)
	}
	if err = s.DeduplicateContent(t.Context()); err != nil {
		t.Fatal(err)
	}
	fragments := splitBodyFragments(body)
	if len(fragments) < 2 {
		t.Fatal("fixture must cross a fragment boundary")
	}
	cut := len([]rune(fragments[0].Body))
	runes := []rune(body)
	q := string(runes[cut-25 : cut+25])
	r, err := s.Search(t.Context(), q, "document", nil)
	if err != nil || len(r.Files) != 1 {
		t.Fatalf("boundary phrase lost: %+v %v", r, err)
	}
	reopened, err := Open(s.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Body(t.Context(), path)
	if err != nil || got != body {
		t.Fatal("reopen changed body", err)
	}
	var vector []byte
	if err = s.db.QueryRow("SELECT vector FROM chunks WHERE id=?", batch[0].ID).Scan(&vector); err != nil || len(vector) != 1024 {
		t.Fatal("vector lost", err)
	}
	if _, err = s.db.Exec("UPDATE files SET body=? WHERE path=?", "new unique searchable text", path); err != nil {
		t.Fatal(err)
	}
	got, err = s.Body(t.Context(), path)
	if err != nil || got != "new unique searchable text" {
		t.Fatal("edit failed", err)
	}
	if _, err = s.db.Exec("UPDATE files SET body='' WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM body_parts").Scan(&count); err != nil || count != 0 {
		t.Fatal("old text retained", count, err)
	}
}

// Keep real legacy tables for upgrade tests; new empty catalogs use shared storage.
func legacyFixture(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "library")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = registerUnicodeLower(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "legacy.db")
	db, err := sql.Open(databaseDriver, path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE roots(path TEXT PRIMARY KEY);CREATE TABLE files(path TEXT PRIMARY KEY,root TEXT NOT NULL REFERENCES roots(path) ON DELETE CASCADE,name TEXT NOT NULL,kind TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL,body TEXT NOT NULL,seen INTEGER NOT NULL,placeholder INTEGER NOT NULL DEFAULT 0);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO roots VALUES(?)", root); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO files VALUES(?,?,?,'other',0,0,'',0,0)", filepath.Join(root, "absent-seed"), root, "absent-seed"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}

func TestSharedFragmentsResynchronizeAfterDifferentPrefixes(t *testing.T) {
	s, root := legacyFixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	rng := rand.New(rand.NewPCG(7, 19))
	r := make([]rune, 100000)
	alphabet := []rune("abcdefghijklmnopqrstuvwxyz中文😀")
	for i := range r {
		r[i] = alphabet[rng.IntN(len(alphabet))]
	}
	base := string(r)
	bodies := []string{"one header\n" + base + "one footer", "a differently sized header\n" + base + "second footer"}
	for i, b := range bodies {
		put(t, filepath.Join(root, string(rune('a'+i))+".txt"), b)
	}
	scan(t, s)
	if err := s.DeduplicateContent(t.Context()); err != nil {
		t.Fatal(err)
	}
	var shared int
	if err := s.db.QueryRow("SELECT count(*) FROM (SELECT part_id FROM body_refs GROUP BY part_id HAVING count(DISTINCT path)>1)").Scan(&shared); err != nil || shared == 0 {
		t.Fatal("partial content not shared", shared, err)
	}
	for i, b := range bodies {
		got, err := s.Body(t.Context(), filepath.Join(root, string(rune('a'+i))+".txt"))
		if err != nil || got != b {
			t.Fatal("reconstruction changed text", err)
		}
	}
	parts := splitBodyFragments(bodies[0])
	cut := len([]rune(parts[0].Body))
	all := []rune(bodies[0])
	q := string(all[cut-250 : cut+250])
	if strings.Count(bodies[0], q) != 1 {
		t.Fatal("boundary fixture is ambiguous")
	}
	result, err := s.Search(t.Context(), q, "document", nil)
	if err != nil || len(result.Files) != 2 {
		t.Fatalf("500-character cross-boundary phrase lost: %d %v", len(result.Files), err)
	}
	for _, table := range []string{"body_fts", "body_short_fts"} {
		if _, err = s.db.Exec("INSERT INTO " + table + "(" + table + ",rank) VALUES('integrity-check',1)"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSharedMigrationRollbackAndEmbeddedNUL(t *testing.T) {
	s, root := legacyFixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	body := "before\x00after😀" + strings.Repeat("片段\n", 12000) + "\x00final"
	path := filepath.Join(root, "nul.docx")
	if _, err := s.db.Exec("INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,1,?,1)", path, root, "nul.docx", body); err != nil {
		t.Fatal(err)
	}
	// Fail after the transactional schema changes have begun, and assert the legacy schema survived.
	if _, err := s.db.Exec("CREATE TABLE body_parts(block_migration INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeduplicateContent(t.Context()); err == nil {
		t.Fatal("expected migration failure")
	}
	var typ string
	if err := s.db.QueryRow("SELECT type FROM sqlite_master WHERE name='files'").Scan(&typ); err != nil || typ != "table" {
		t.Fatal("failed migration changed legacy schema", typ, err)
	}
	got, err := s.Body(t.Context(), path)
	if err != nil || got != body {
		t.Fatal("failed migration changed body", err)
	}
	if _, err = s.db.Exec("DROP TABLE body_parts"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = s.DeduplicateContent(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if err = s.DeduplicateContent(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyContentAgainst(t.Context(), s.dbPath); err != nil {
		t.Fatal(err)
	}
	got, err = s.Body(t.Context(), path)
	if err != nil || got != body {
		t.Fatal("NUL or Unicode lost", err)
	}
	rows, err := s.readDB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("broken foreign key after migration")
	}
}

func TestSharedVerificationDetectsChangedContent(t *testing.T) {
	s, root := legacyFixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	path := filepath.Join(root, "verify.txt")
	put(t, path, "original shared body for verification")
	scan(t, s)
	original := filepath.Join(t.TempDir(), "original.db")
	if _, err := s.db.Exec("VACUUM INTO ?", original); err != nil {
		t.Fatal(err)
	}
	if err := s.DeduplicateContent(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyContentAgainst(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE files SET body='different content' WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyContentAgainst(t.Context(), original); err == nil {
		t.Fatal("changed body passed verification")
	}
}

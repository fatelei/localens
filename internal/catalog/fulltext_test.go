package catalog

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFullTextCandidatesKeepLiteralSemantics(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	samples := map[string]string{"phrase.py": "while True:\n  print('多词查询优先')", "split.txt": "while elsewhere and True later", "symbols.txt": "100% literal_under_score and x\"yz", "unicode.txt": "İSTANBUL and Привет"}
	for name, body := range samples {
		put(t, filepath.Join(root, name), body)
	}
	scan(t, s)
	for _, q := range []string{"查询", "优先", "词", "多词查询优先", "while True", "literal_under", "100%", "x\"yz", "istanbul", "ПРИВЕТ"} {
		r, err := s.Search(t.Context(), q, "all", nil)
		if err != nil || len(r.Files) == 0 {
			t.Fatalf("query %q lost: %#v %v", q, r, err)
		}
	}
	r, err := s.Search(t.Context(), "while True", "all", nil)
	if err != nil || len(r.Files) != 2 || r.Files[0].Name != "phrase.py" {
		t.Fatalf("phrase priority: %#v %v", r, err)
	}
	if _, err = s.db.Exec("INSERT INTO body_short_fts(body_short_fts,rank) VALUES('integrity-check',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("INSERT INTO body_fts(body_fts,rank) VALUES('integrity-check',1)"); err != nil {
		t.Fatal(err)
	}
}
func TestFullTextPendingFallbackAndUpdates(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	path := filepath.Join(root, "notes.txt")
	body := "旧的中文正文索引"
	put(t, path, body)
	scan(t, s)
	var rowid int64
	if err := s.db.QueryRow("SELECT id FROM content_documents WHERE path=?", path).Scan(&rowid); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO body_fts(body_fts,rowid,text) VALUES('delete',?,?)", rowid, body); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO body_short_fts(body_short_fts,rowid,text) VALUES('delete',?,?)", rowid, shortTokens(body)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO body_fts_pending(path) VALUES(?)", path); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err := s.Search(t.Context(), "中文正文索引", "all", nil)
	if err != nil || len(r.Files) != 1 {
		t.Fatalf("pending body lost: %#v %v", r, err)
	}
	n, err := s.indexTextBatch(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("batch: %d %v", n, err)
	}
	if _, err = s.db.Exec("UPDATE files SET body=? WHERE path=?", "新的内容更新", path); err != nil {
		t.Fatal(err)
	}
	r, err = s.Search(t.Context(), "中文正文索引", "all", nil)
	if err != nil || len(r.Files) != 0 {
		t.Fatalf("old text retained: %#v %v", r, err)
	}
	r, err = s.Search(t.Context(), "内容更新", "all", nil)
	if err != nil || len(r.Files) != 1 {
		t.Fatalf("new text missing: %#v %v", r, err)
	}
	if _, err = s.db.Exec("DELETE FROM files WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM body_fts WHERE body_fts MATCH ?", trigramCandidates("内容更新")).Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted posting: %d %v", count, err)
	}
}
func TestQuotedTrigramsAndShortFallback(t *testing.T) {
	if trigramCandidates("押金") != "" {
		t.Fatal("short query must use fallback")
	}
	for _, q := range []string{"a\"b", "%_\\", "😀中文", "TRUE"} {
		tokens := trigramCandidates(q)
		if !strings.HasPrefix(tokens, `"`) {
			t.Fatalf("unquoted tokens: %q", tokens)
		}
	}
}

func TestFullTextSurvivesCompactionAndReopen(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	put(t, filepath.Join(root, "gone.txt"), "removed gap record")
	put(t, filepath.Join(root, "target.go"), "package main\n\nfunc main() {\n // 唯一内容索引短语\n}\n")
	scan(t, s)
	if _, err := s.db.Exec("DELETE FROM files WHERE path=?", filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	before, err := s.Search(t.Context(), "唯一内容索引短语", "all", nil)
	if err != nil || len(before.Files) != 1 {
		t.Fatalf("before: %#v %v", before, err)
	}
	if err = s.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := s.Search(t.Context(), "唯一内容索引短语", "all", nil)
	if err != nil || len(after.Files) != 1 || after.Files[0].Path != before.Files[0].Path || after.Files[0].Line != before.Files[0].Line {
		t.Fatalf("compact changed result: %#v %v", after, err)
	}
	if _, err = s.db.Exec("INSERT INTO body_short_fts(body_short_fts,rank) VALUES('integrity-check',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("INSERT INTO body_fts(body_fts,rank) VALUES('integrity-check',1)"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err = reopened.Search(t.Context(), "唯一内容索引短语", "all", nil)
	if err != nil || len(after.Files) != 1 {
		t.Fatalf("reopen: %#v %v", after, err)
	}
}

func TestLegacyFullTextUpgrade(t *testing.T) {
	s, root := fixture(t)
	s.contentCancel()
	s.contentWG.Wait()
	put(t, filepath.Join(root, "legacy.txt"), "旧版查询内容 migration phrase")
	scan(t, s)
	_, err := s.db.Exec(`DROP TABLE content_schema; DROP TABLE body_short_fts; DROP VIEW file_short_content;`)
	if err != nil {
		t.Fatal(err)
	}
	if err = initializeFullText(s.db); err != nil {
		t.Fatal(err)
	}
	for {
		n, e := s.indexTextBatch(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			break
		}
	}
	for _, q := range []string{"查询", "migration phrase"} {
		r, e := s.Search(t.Context(), q, "all", nil)
		if e != nil || len(r.Files) != 1 {
			t.Fatalf("upgrade %q: %#v %v", q, r, e)
		}
	}
	for _, table := range []string{"body_fts", "body_short_fts"} {
		if _, err = s.db.Exec("INSERT INTO " + table + "(" + table + ",rank) VALUES('integrity-check',1)"); err != nil {
			t.Fatal(err)
		}
	}
}

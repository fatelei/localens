package catalog

import (
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhraseRanksBeforeSeparatedWordsEvenPastCandidateLimit(t *testing.T) {
	s, root := fixture(t)
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(t.Context(), "INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,?,?,1)")
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()
	for i := range 4002 {
		name := fmt.Sprintf("source-%d.py", i)
		body := "# true is elsewhere\n\nwhile pending:\n  pass"
		modified := int64(100)
		if i == 4001 {
			name = "target.py"
			body = "# true appears earlier\n\nwhile True:\n  pass"
			modified = 1
		}
		if _, err := statement.ExecContext(t.Context(), filepath.Join(root, name), root, name, modified, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	result, err := s.Search(t.Context(), "while True", "document", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) == 0 || result.Files[0].Name != "target.py" {
		t.Fatalf("phrase not first: %+v", result.Files[:min(1, len(result.Files))])
	}
	if !strings.Contains(result.Files[0].Snippet, "while True") || result.Files[0].Line != 3 {
		t.Fatalf("wrong phrase excerpt: %+v", result.Files[0])
	}
}

func TestRelatedCandidatesNotCrowdedOutByKeywordResults(t *testing.T) {
	s, root := fixture(t)
	for i := range 81 {
		name := fmt.Sprintf("notes-%d.txt", i)
		body := "while True is an exact phrase"
		if i == 80 {
			body = "A loop that continues indefinitely"
		}
		if _, err := s.db.ExecContext(t.Context(), "INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,1,?,1)", filepath.Join(root, name), root, name, body); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "notes-80.txt")
	v := make([]float32, 256)
	v[0] = 1
	encoded := make([]byte, 1024)
	binary.LittleEndian.PutUint32(encoded, math.Float32bits(1))
	if _, err := s.db.ExecContext(t.Context(), "INSERT INTO chunks(path,body,vector) VALUES(?,?,?)", path, "A loop that continues indefinitely", encoded); err != nil {
		t.Fatal(err)
	}
	keywords, err := s.Search(t.Context(), "while True", "all", nil)
	if err != nil || len(keywords.Files) != 80 {
		t.Fatalf("keywords: %d %v", len(keywords.Files), err)
	}
	related, err := s.SearchRelated(t.Context(), "while True", "all", v)
	if err != nil || len(related.Files) != 1 || related.Files[0].Path != path {
		t.Fatalf("related: %#v %v", related, err)
	}
}

func TestRelatedRecallLeavesRoomForKeywordDeduplication(t *testing.T) {
	s, root := fixture(t)
	v := make([]float32, 256)
	v[0] = 1
	for i := range 81 {
		name := fmt.Sprintf("notes-%d.txt", i)
		body := "while True"
		encoded := make([]byte, 1024)
		binary.LittleEndian.PutUint32(encoded, math.Float32bits(1))
		if i == 80 {
			body = "An indefinite loop"
			binary.LittleEndian.PutUint32(encoded, math.Float32bits(0.9))
			binary.LittleEndian.PutUint32(encoded[4:], math.Float32bits(float32(math.Sqrt(0.19))))
		}
		path := filepath.Join(root, name)
		if _, err := s.db.ExecContext(t.Context(), "INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,'document',1,1,?,1)", path, root, name, body); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(t.Context(), "INSERT INTO chunks(path,body,vector) VALUES(?,?,?)", path, body, encoded); err != nil {
			t.Fatal(err)
		}
	}
	related, err := s.SearchRelated(t.Context(), "while True", "all", v)
	if err != nil || len(related.Files) != 81 || related.Files[80].Name != "notes-80.txt" {
		t.Fatalf("dedup recall: %d %v", len(related.Files), err)
	}
}

func TestUncasedBodySearchUsesNativeMatcher(t *testing.T) {
	for _, q := range []string{"多词查询优先", "2026", "海边 日落"} {
		if searchBodyColumn(q) != "body" {
			t.Fatalf("uncased query slow path: %q", q)
		}
	}
	for _, q := range []string{"while True", "İstanbul", "ПРИВЕТ", "多词 Query"} {
		if searchBodyColumn(q) != "unicode_lower(body)" {
			t.Fatalf("lost Unicode folding: %q", q)
		}
	}
	s, root := fixture(t)
	put(t, filepath.Join(root, "notes.txt"), "这是多词查询优先匹配的说明")
	scan(t, s)
	result, err := s.Search(t.Context(), "多词查询优先", "all", nil)
	if err != nil || len(result.Files) != 1 || !strings.Contains(result.Files[0].Snippet, "多词查询优先") {
		t.Fatalf("literal evidence: %#v %v", result, err)
	}
}

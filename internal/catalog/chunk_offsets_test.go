package catalog

import (
	"database/sql"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewChunksReferenceBodyWithoutDuplicatingText(t *testing.T) {
	s, root := fixture(t)
	body := strings.Repeat("中文😀 code\n", 500)
	put(t, filepath.Join(root, "notes.py"), body)
	scan(t, s)
	var stored int
	if err := s.db.QueryRow("SELECT sum(length(body)) FROM chunks").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("duplicated %d characters", stored)
	}
	pending, err := s.PendingChunks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	expected := chunkText(body)
	if len(pending) != len(expected) {
		t.Fatal("chunk count changed")
	}
	for i := range pending {
		if pending[i].Body != expected[i] {
			t.Fatalf("Unicode chunk %d changed", i)
		}
	}
}
func TestLegacyChunkMigrationPreservesUnicodeVectorsAndFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	root := filepath.Join(dir, "library")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "notes.py")
	body := strings.Repeat("中文😀 code\n", 500)
	put(t, file, body)
	if err := registerUnicodeLower(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(databaseDriver, path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`PRAGMA foreign_keys=ON;
 CREATE TABLE roots(path TEXT PRIMARY KEY);
 CREATE TABLE files(path TEXT PRIMARY KEY,root TEXT NOT NULL REFERENCES roots(path),name TEXT NOT NULL,kind TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL,body TEXT NOT NULL,seen INTEGER NOT NULL,placeholder INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE chunks(id INTEGER PRIMARY KEY,path TEXT NOT NULL REFERENCES files(path),body TEXT NOT NULL,vector BLOB);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO roots VALUES(?)", root); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO files VALUES(?,?,?,'document',1,1,?,1,0)", file, root, "notes.py", body); err != nil {
		t.Fatal(err)
	}
	vector := make([]byte, 1024)
	binary.LittleEndian.PutUint32(vector, math.Float32bits(1))
	expected := chunkText(body)
	for _, chunk := range expected {
		if _, err = db.Exec("INSERT INTO chunks(path,body,vector) VALUES(?,?,?)", file, chunk, vector); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO chunks(path,body) VALUES(?,?)", file, "custom fallback"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.readDB.Query("SELECT " + chunkBodySQL + ",c.body,c.vector FROM chunks c JOIN files f ON c.path=f.path ORDER BY c.id")
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	for rows.Next() {
		var recovered, stored string
		var actual []byte
		if err := rows.Scan(&recovered, &stored, &actual); err != nil {
			t.Fatal(err)
		}
		if index < len(expected) {
			if recovered != expected[index] || stored != "" || string(actual) != string(vector) {
				t.Fatalf("chunk %d damaged", index)
			}
		} else if recovered != "custom fallback" || stored != "custom fallback" {
			t.Fatal("unknown chunk layout discarded")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if index != len(expected)+1 {
		t.Fatal("lost chunks")
	}
	if err := s.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}
	queryVector := make([]float32, 256)
	queryVector[0] = 1
	result, err := s.Search(t.Context(), "forest trail", "document", queryVector)
	if err != nil || len(result.Files) != 1 || result.Files[0].Match != "语义" || !strings.Contains(result.Files[0].Snippet, "中文😀") || result.Files[0].Line != 1 {
		t.Fatalf("semantic evidence changed: %+v %v", result, err)
	}
	recovered, err := s.Body(t.Context(), file)
	if err != nil || recovered != body {
		t.Fatal("full body changed")
	}
}

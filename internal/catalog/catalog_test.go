package catalog

import (
	"archive/zip"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "资料")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "data", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err = s.AddRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	return s, root
}
func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func scan(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}
func searchFiles(t *testing.T, s *Store, q, kind string) []File {
	t.Helper()
	r, err := s.Search(t.Context(), q, kind, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r.Files
}
func TestChineseSearchFiltersAndLiteralWildcards(t *testing.T) {
	s, root := fixture(t)
	put(t, filepath.Join(root, "房屋协议.md"), "关于租房押金：退房验收后七日内退还。\n查询 100% 和 a_b。")
	put(t, filepath.Join(root, "租房押金.txt"), "文件名优先")
	put(t, filepath.Join(root, "房屋照片.jpg"), "not a real image")
	scan(t, s)
	found := searchFiles(t, s, "租房押金", "all")
	if len(found) != 2 || found[0].Name != "租房押金.txt" {
		t.Fatalf("name priority / Chinese text: %#v", found)
	}
	if f := searchFiles(t, s, "退房", "document"); len(f) != 1 || !strings.Contains(f[0].Snippet, "退房") {
		t.Fatalf("body match: %#v", f)
	}
	if f := searchFiles(t, s, "房屋", "image"); len(f) != 1 {
		t.Fatalf("filter: %#v", f)
	}
	for _, q := range []string{"100%", "a_b"} {
		if f := searchFiles(t, s, q, "all"); len(f) != 1 {
			t.Fatalf("literal wildcard %s: %#v", q, f)
		}
	}
	if f := searchFiles(t, s, "协议 押金", "all"); len(f) != 1 {
		t.Fatalf("multi-term AND: %#v", f)
	}
}
func TestIncrementalDeletionCancellationAndPersistence(t *testing.T) {
	s, root := fixture(t)
	path := filepath.Join(root, "one.txt")
	put(t, path, "旧版本")
	scan(t, s)
	pending, err := s.PendingChunks(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending %v %v", pending, err)
	}
	v := make([]float32, 256)
	v[0] = 1
	if err = s.SaveVector(t.Context(), pending[0].ID, v); err != nil {
		t.Fatal(err)
	}
	scan(t, s)
	stats, _ := s.Statistics(t.Context())
	if stats.Vectors != 1 {
		t.Fatal("unchanged scan invalidated vectors")
	}
	put(t, path, "新的版本，长度不同")
	scan(t, s)
	if len(searchFiles(t, s, "旧版本", "all")) != 0 || len(searchFiles(t, s, "新的版本", "all")) != 1 {
		t.Fatal("modified content not refreshed")
	}
	stats, _ = s.Statistics(t.Context())
	if stats.Vectors != 0 {
		t.Fatal("stale vector retained")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = s.Scan(ctx, nil); err == nil {
		t.Fatal("canceled scan should fail")
	}
	if len(searchFiles(t, s, "新的版本", "all")) != 1 {
		t.Fatal("canceled scan deleted entries")
	}
	scan(t, s)
	stats, _ = s.Statistics(t.Context())
	if stats.Files != 0 || stats.Chunks != 0 {
		t.Fatalf("deletion failed: %#v", stats)
	}
	put(t, path, "持久化")
	scan(t, s)
	var dbPath string
	if err = s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	other, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if len(searchFiles(t, other, "持久化", "all")) != 1 {
		t.Fatal("index did not persist")
	}
}
func TestSemanticNormalizationRankingAndValidation(t *testing.T) {
	s, root := fixture(t)
	put(t, filepath.Join(root, "海边照片.png"), "image")
	put(t, filepath.Join(root, "笔记.txt"), "山里的日出")
	scan(t, s)
	pending, _ := s.PendingChunks(t.Context())
	for _, p := range pending {
		v := make([]float32, 256)
		if p.Kind == "image" {
			v[0] = 7
		} else {
			v[1] = 3
		}
		if err := s.SaveVector(t.Context(), p.ID, v); err != nil {
			t.Fatal(err)
		}
	}
	q := make([]float32, 256)
	q[0] = 4
	r, err := s.Search(t.Context(), "蓝色的大海", "all", q)
	if err != nil || len(r.Files) != 1 || r.Files[0].Kind != "image" || r.Files[0].Match != "语义" {
		t.Fatalf("cross-modal retrieval: %#v %v", r, err)
	}
	r, err = s.Search(t.Context(), "蓝色的大海", "document", q)
	if err != nil || len(r.Files) != 0 {
		t.Fatalf("semantic filter: %#v %v", r, err)
	}
	if err = s.SaveVector(t.Context(), pending[0].ID, []float32{1}); err == nil {
		t.Fatal("bad dimension accepted")
	}
	q[0] = float32(math.NaN())
	if err = s.SaveVector(t.Context(), pending[0].ID, q); err == nil {
		t.Fatal("NaN accepted")
	}
}
func TestRootSafetySymlinksAndRemove(t *testing.T) {
	s, root := fixture(t)
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRoot(t.Context(), nested); err == nil {
		t.Fatal("overlap accepted")
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	put(t, outside, "secret")
	path := filepath.Join(root, "normal.txt")
	put(t, path, "normal")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	scan(t, s)
	if _, err := s.VerifiedPath(t.Context(), link); err == nil {
		t.Fatal("symlink indexed")
	}
	if _, err := s.VerifiedPath(t.Context(), outside); err == nil {
		t.Fatal("unindexed path accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifiedPath(t.Context(), path); err == nil {
		t.Fatal("replaced symlink accepted")
	}
	if err := s.RemoveRoot(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("source file removed")
	}
	stats, _ := s.Statistics(t.Context())
	if stats.Files != 0 {
		t.Fatal("index not cleared")
	}
	if Within(root, root) || Within(root, filepath.Dir(root)) || Within(root, root+"-sibling/a") {
		t.Fatal("boundary failure")
	}
}
func TestDOCXAndChunkCoverage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "合同.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(`<w:document xmlns:w="word"><w:body><w:p><w:r><w:t>押金协议</w:t></w:r></w:p><w:p><w:r><w:t>退还期限七天</w:t></w:r></w:p></w:body></w:document>`)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := extract(t.Context(), path)
	if err != nil || !strings.Contains(body, "押金协议\n退还期限七天") {
		t.Fatalf("docx extract: %q %v", body, err)
	}
	chunks := chunkText(strings.Repeat("中", 2600))
	if len(chunks) != 3 || len([]rune(chunks[0])) != 1200 || len([]rune(chunks[2])) != 500 {
		t.Fatalf("chunk boundaries: %v", chunks)
	}
}
func TestScanSkipsHiddenAndDependencies(t *testing.T) {
	s, root := fixture(t)
	for _, dir := range []string{".git", "node_modules", "vendor"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(root, dir, "secret.txt"), "secret")
	}
	put(t, filepath.Join(root, "ok.txt"), "hello")
	scan(t, s)
	stats, _ := s.Statistics(t.Context())
	if stats.Files != 1 {
		t.Fatalf("excluded directories indexed: %#v", stats)
	}
}
func TestCancellationMidScanRetainsUnvisitedFiles(t *testing.T) {
	s, root := fixture(t)
	for _, name := range []string{"a.txt", "z.txt"} {
		put(t, filepath.Join(root, name), "old")
	}
	scan(t, s)
	put(t, filepath.Join(root, "a.txt"), "changed")
	if err := os.Remove(filepath.Join(root, "z.txt")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	err := s.Scan(ctx, func(ScanProgress) { cancel() })
	if err == nil {
		t.Fatal("scan expected cancellation")
	}
	stats, _ := s.Statistics(t.Context())
	if stats.Files != 2 {
		t.Fatal("unvisited file deleted on cancellation")
	}
	scan(t, s)
	stats, _ = s.Statistics(t.Context())
	if stats.Files != 1 {
		t.Fatal("completed scan failed cleanup")
	}
}
func TestConcurrentSearchDuringScan(t *testing.T) {
	s, root := fixture(t)
	for i := range 40 {
		put(t, filepath.Join(root, time.Unix(int64(i), 0).Format("150405")+".txt"), "并发查询")
	}
	done := make(chan error, 1)
	go func() { done <- s.Scan(t.Context(), nil) }()
	for range 20 {
		if _, err := s.Search(t.Context(), "并发", "all", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUnicodeCaseInsensitiveSearch(t *testing.T) {
	s, root := fixture(t)
	put(t, filepath.Join(root, "ÉCOLE.txt"), "Über schöne Häuser")
	scan(t, s)
	for _, q := range []string{"école", "über", "HÄUSER"} {
		if len(searchFiles(t, s, q, "all")) != 1 {
			t.Fatalf("Unicode case matching failed: %s", q)
		}
	}
}

func TestExactFilenameSurvivesLargeBodyCandidateSet(t *testing.T) {
	s, root := fixture(t)
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.PrepareContext(t.Context(), "INSERT INTO files(path,root,name,kind,size,modified,body,seen) VALUES(?,?,?,?,?,?,?,?)")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 4001 {
		name := time.Unix(int64(i), 0).Format("150405") + ".txt"
		if _, err = stmt.ExecContext(t.Context(), filepath.Join(root, name), root, name, "document", 1, int64(i+100), "target.txt appears in body", 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = stmt.ExecContext(t.Context(), filepath.Join(root, "target.txt"), root, "target.txt", "document", 1, 1, "exact filename", 1); err != nil {
		t.Fatal(err)
	}
	if err = stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	found := searchFiles(t, s, "target.txt", "all")
	if len(found) == 0 || found[0].Name != "target.txt" {
		t.Fatalf("exact old filename was truncated: %#v", found)
	}
}

func TestHydrationRefreshesBodyWithoutSizeOrMtimeChanges(t *testing.T) {
	s, root := fixture(t)
	path := filepath.Join(root, "cloud-document.txt")
	put(t, path, "下载后可搜索的正文")
	scan(t, s)
	// Simulate the previously persisted placeholder state; the hydrated source retains size and mtime.
	if _, err := s.db.Exec("UPDATE files SET body='',placeholder=1 WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM chunks WHERE path=?", path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("INSERT INTO chunks(path,body) VALUES(?,?)", path, "cloud-document.txt"); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingChunks(t.Context())
	if err != nil || len(pending) != 0 {
		t.Fatalf("cloud placeholder scheduled for download/inference: %#v %v", pending, err)
	}
	scan(t, s)
	found := searchFiles(t, s, "下载后可搜索", "document")
	if len(found) != 1 {
		t.Fatalf("hydrated content not refreshed: %#v", found)
	}
	pending, err = s.PendingChunks(t.Context())
	if err != nil || len(pending) != 1 || !strings.Contains(pending[0].Body, "下载后可搜索") {
		t.Fatalf("hydrated semantic chunks not rebuilt: %#v %v", pending, err)
	}
}

func TestOldCatalogPlaceholderMigrationPreservesFiles(t *testing.T) {
	s, root := fixture(t)
	put(t, filepath.Join(root, "old.txt"), "旧索引正文")
	scan(t, s)
	var path string
	if err := s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	s.contentCancel()
	s.contentWG.Wait()
	if _, err := s.db.Exec("DROP TRIGGER files_ocr_changed; DROP INDEX files_ocr_pending; ALTER TABLE files DROP COLUMN ocr_version; ALTER TABLE files DROP COLUMN ocr_error; ALTER TABLE files DROP COLUMN placeholder"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	found := searchFiles(t, reopened, "旧索引正文", "all")
	if len(found) != 1 {
		t.Fatal("migration lost indexed files")
	}
	scan(t, reopened)
}

func TestUnseenCleanupProtectsOnlyInaccessibleSubtree(t *testing.T) {
	s, root := fixture(t)
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(locked, "keep.txt"), "protected")
	put(t, filepath.Join(root, "deleted.txt"), "remove me")
	if err := s.Scan(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.cleanUnseen(t.Context(), root, time.Now().UnixNano(), []string{locked}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Search(t.Context(), "", "all", nil)
	if err != nil || len(result.Files) != 1 || result.Files[0].Name != "keep.txt" {
		t.Fatalf("partial cleanup: %#v %v", result, err)
	}
}

func TestDirectVideoRegisteredBeforeNestedDirectories(t *testing.T) {
	s, root := fixture(t)
	nested := filepath.Join(root, "A-nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(nested, "nested.txt"), "nested")
	put(t, filepath.Join(root, "Out.mp4"), "video")
	var first string
	if err := s.Scan(t.Context(), func(p ScanProgress) {
		if first == "" {
			first = filepath.Base(p.Current)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if first != "Out.mp4" {
		t.Fatalf("first indexed file=%s", first)
	}
}

func TestShortKeywordDoesNotMatchParentDirectoryBodyOrInnerWord(t *testing.T) {
	s, root := fixture(t)
	directory := filepath.Join(root, "outputs")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(directory, "unrelated.mp4"), "video")
	put(t, filepath.Join(root, "Out Of Context.mp4"), "video")
	put(t, filepath.Join(root, "Router.txt"), "leather and outputs")
	put(t, filepath.Join(root, "The_Telegraph.pdf"), "fixture")
	scan(t, s)
	found := searchFiles(t, s, "Out", "all")
	if len(found) != 1 || found[0].Name != "Out Of Context.mp4" {
		t.Fatalf("noisy short query: %#v", found)
	}
	if found = searchFiles(t, s, "the", "all"); len(found) != 1 || found[0].Name != "The_Telegraph.pdf" {
		t.Fatalf("inner body match: %#v", found)
	}
	if found = searchFiles(t, s, "/outputs/", "video"); len(found) != 0 {
		t.Fatalf("path-only query should not match files: %#v", found)
	}
	vector := make([]float32, 256)
	vector[0] = 1
	result, err := s.Search(t.Context(), "Out", "video", vector)
	if err != nil || result.Semantic || result.SemanticEligible || len(result.Files) != 1 {
		t.Fatalf("short query used model: %#v %v", result, err)
	}
}

func TestOrdinaryKeywordDoesNotMatchParentDirectory(t *testing.T) {
	s, root := fixture(t)
	directory := filepath.Join(root, "outputs")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(directory, "holiday.mp4"), "video")
	scan(t, s)
	if found := searchFiles(t, s, "outputs", "video"); len(found) != 0 {
		t.Fatalf("parent path noise: %#v", found)
	}
}

func TestSemanticQueryKeepsNaturalLanguageAndChineseShortWords(t *testing.T) {
	for _, query := range []string{"日落", "海边", "a dog on snow.", "photos of sunset."} {
		if !SemanticQuery(query, "image") {
			t.Errorf("description rejected: %s", query)
		}
	}
	for _, query := range []string{"Out", "the", "report.pdf", "The_Telegraph", "/outputs/"} {
		if SemanticQuery(query, "all") {
			t.Errorf("keyword/path/filename accepted: %s", query)
		}
	}
}

func TestShortDescriptiveSentenceRetainsSemanticSearch(t *testing.T) {
	if !SemanticQuery("a red dog", "all") {
		t.Fatal("natural description blocked")
	}
	if SemanticQuery("Out Of", "all") {
		t.Fatal("ambiguous filename phrase should not use model")
	}
}

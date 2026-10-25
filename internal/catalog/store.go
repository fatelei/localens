// Package catalog stores and searches a local file index.
package catalog

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store owns the persistent index and serializes indexing jobs.
type Store struct {
	db            *sql.DB
	readDB        *sql.DB
	dbPath        string
	scanMu        sync.Mutex
	contentCancel context.CancelFunc
	contentWG     sync.WaitGroup
	contentMu     sync.Mutex
	contentError  string
	ocrMu         sync.Mutex
	ocrEnabled    bool
	ocrCancel     context.CancelFunc
}

// Root is a directory selected by the user.
type Root struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// File describes an indexed file, without exposing its full body.
type File struct {
	Line        int     `json:"line"`
	SnippetLine int     `json:"snippetLine"`
	Path        string  `json:"path"`
	Root        string  `json:"root"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Size        int64   `json:"size"`
	Modified    int64   `json:"modified"`
	Snippet     string  `json:"snippet"`
	Score       float64 `json:"score"`
	Match       string  `json:"match"`
}

// Stats summarizes index size.
type Stats struct {
	TextPending     int    `json:"textPending"`
	OCRPending      int    `json:"ocrPending"`
	OCRFailed       int    `json:"ocrFailed"`
	OCRAvailable    bool   `json:"ocrAvailable"`
	ContentRevision int64  `json:"contentRevision"`
	ContentError    string `json:"contentError"`
	Files           int    `json:"files"`
	Chunks          int    `json:"chunks"`
	Vectors         int    `json:"vectors"`
	Images          int    `json:"images"`
	Documents       int    `json:"documents"`
	Videos          int    `json:"videos"`
}

// databaseFileURL keeps Windows drive letters in the URI path, not its authority.
func databaseFileURL(path string) *url.URL {
	path = filepath.ToSlash(path)
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		path = "/" + path
	}
	return &url.URL{Scheme: "file", Path: path}
}

// Open creates or opens the private on-disk database.
func Open(path string) (*Store, error) { return openStore(path, true) }

// OpenForMaintenance must only be used while the desktop app is stopped.
// It deliberately starts no OCR or full-text backfill workers.
func OpenForMaintenance(path string) (*Store, error) { return openStore(path, false) }

func openStore(path string, background bool) (*Store, error) {
	if err := registerUnicodeLower(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	uri := databaseFileURL(path)
	params := url.Values{}
	params.Set("_foreign_keys", "on")
	params.Set("_busy_timeout", "5000")
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = params.Encode()
	db, err := sql.Open(databaseDriver, uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; CREATE TABLE IF NOT EXISTS roots(path TEXT PRIMARY KEY);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	var shared int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='shared_body_schema'").Scan(&shared); err != nil {
		db.Close()
		return nil, err
	}
	if shared == 0 {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS files(path TEXT PRIMARY KEY,root TEXT NOT NULL REFERENCES roots(path) ON DELETE CASCADE,name TEXT NOT NULL,kind TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL,body TEXT NOT NULL,seen INTEGER NOT NULL,placeholder INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS files_root ON files(root);
 CREATE INDEX IF NOT EXISTS files_kind ON files(kind);
 CREATE TABLE IF NOT EXISTS chunks(id INTEGER PRIMARY KEY,path TEXT NOT NULL REFERENCES files(path) ON DELETE CASCADE,body TEXT NOT NULL,vector BLOB);
 CREATE INDEX IF NOT EXISTS chunks_path ON chunks(path);`)
		if err != nil {
			db.Close()
			return nil, err
		}
	}

	var placeholderColumns int
	if err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='placeholder'").Scan(&placeholderColumns); err != nil {
		db.Close()
		return nil, err
	}
	if placeholderColumns == 0 {
		if _, err = db.Exec("ALTER TABLE files ADD COLUMN placeholder INTEGER NOT NULL DEFAULT 0"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err = initializeChunkOffsets(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = initializeMetadata(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = initializeOCR(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = initializeFullText(db); err != nil {
		db.Close()
		return nil, err
	}
	readerURI := *uri
	readerParams := readerURI.Query()
	readerParams.Set("mode", "ro")
	readerParams.Add("_pragma", "query_only(1)")
	readerURI.RawQuery = readerParams.Encode()
	reader, err := sql.Open(databaseDriver, readerURI.String())
	if err != nil {
		db.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)
	absPath, err := filepath.Abs(path)
	if err != nil {
		reader.Close()
		db.Close()
		return nil, err
	}
	s := &Store{db: db, readDB: reader, dbPath: absPath}
	if shared == 0 {
		var populated bool
		if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM files LIMIT 1)").Scan(&populated); err != nil {
			reader.Close()
			db.Close()
			return nil, err
		}
		if !populated {
			if err = s.DeduplicateContent(context.Background()); err != nil {
				reader.Close()
				db.Close()
				return nil, err
			}
		}
	}
	if background {
		s.startContentWork()
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error {
	if s.contentCancel != nil {
		s.contentCancel()
		s.contentWG.Wait()
	}
	return errors.Join(s.readDB.Close(), s.db.Close())
}

// Within reports whether path is strictly inside root.
func Within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

// AddRoot registers a canonical directory, rejecting overlaps.
func (s *Store) AddRoot(ctx context.Context, path string) error {
	abs, err := canonicalDirectory(path)
	if err != nil {
		return err
	}
	roots, err := s.Roots(ctx)
	if err != nil {
		return err
	}
	for _, r := range roots {
		if r.Path == abs || Within(r.Path, abs) || Within(abs, r.Path) {
			return errors.New("这个目录已包含在索引范围中，或与已有目录重叠")
		}
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO roots(path) VALUES(?)", abs)
	return err
}

func canonicalDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("请选择文件夹")
	}
	return abs, nil
}

// ExpandRoot merges existing child scopes into a parent, retaining files and vectors.
func (s *Store) ExpandRoot(ctx context.Context, path string) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	abs, err := canonicalDirectory(path)
	if err != nil {
		return err
	}
	roots, err := s.Roots(ctx)
	if err != nil {
		return err
	}
	for _, root := range roots {
		if root.Path == abs || Within(root.Path, abs) {
			return nil
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO roots(path) VALUES(?)", abs); err != nil {
		return err
	}
	for _, root := range roots {
		if !Within(abs, root.Path) {
			continue
		}
		if _, err = tx.ExecContext(ctx, "UPDATE files SET root=? WHERE root=?", abs, root.Path); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM roots WHERE path=?", root.Path); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Roots lists watched directories and their indexed file counts.
func (s *Store) Roots(ctx context.Context) ([]Root, error) {
	rows, err := s.readDB.QueryContext(ctx, "SELECT r.path,COUNT(f.path) FROM roots r LEFT JOIN file_names f ON f.root=r.path GROUP BY r.path ORDER BY r.path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Root{}
	for rows.Next() {
		var r Root
		if err = rows.Scan(&r.Path, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RemoveRoot removes only index entries, never source files. It waits for an active scan.
func (s *Store) RemoveRoot(ctx context.Context, path string) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	_, err := s.db.ExecContext(ctx, "DELETE FROM roots WHERE path=?", path)
	return err
}

// Statistics returns live counts.
func (s *Store) Statistics(ctx context.Context) (Stats, error) {
	var v Stats
	err := s.readDB.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(kind='image'),0),COALESCE(SUM(kind='document'),0),COALESCE(SUM(kind='video'),0) FROM file_names`).Scan(&v.Files, &v.Images, &v.Documents, &v.Videos)
	if err != nil {
		return v, err
	}
	err = s.readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks").Scan(&v.Chunks)
	if err != nil {
		return v, err
	}
	err = s.readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE (vector IS NOT NULL)=1").Scan(&v.Vectors)
	if err != nil {
		return v, err
	}
	if err = s.readDB.QueryRowContext(ctx, "SELECT count(*) FROM body_fts_pending").Scan(&v.TextPending); err != nil {
		return v, err
	}
	if err = s.readDB.QueryRowContext(ctx, "SELECT version FROM content_revision WHERE id=1").Scan(&v.ContentRevision); err != nil {
		return v, err
	}
	v.OCRAvailable = OCRAvailable()
	if v.OCRAvailable {
		if err = s.readDB.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE kind='image' AND ocr_version=0 AND placeholder=0").Scan(&v.OCRPending); err != nil {
			return v, err
		}
		if err = s.readDB.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE kind='image' AND ocr_error<>''").Scan(&v.OCRFailed); err != nil {
			return v, err
		}
	}
	s.contentMu.Lock()
	v.ContentError = s.contentError
	s.contentMu.Unlock()
	return v, err
}

// VerifiedPath permits only existing, non-symlink files still beneath their indexed root.
func (s *Store) VerifiedPath(ctx context.Context, path string) (File, error) {
	var f File
	err := s.db.QueryRowContext(ctx, "SELECT path,root,name,kind,size,modified FROM files WHERE path=?", path).Scan(&f.Path, &f.Root, &f.Name, &f.Kind, &f.Size, &f.Modified)
	if err != nil {
		return f, errors.New("文件不在索引中")
	}
	resolved, err := filepath.EvalSymlinks(f.Path)
	if err != nil {
		return f, err
	}
	root, err := filepath.EvalSymlinks(f.Root)
	if err != nil {
		return f, err
	}
	if resolved != f.Path || root != f.Root || !Within(root, resolved) {
		return f, errors.New("文件已移动或目录包含符号链接，请重新索引")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return f, err
	}
	if !info.Mode().IsRegular() {
		return f, errors.New("不是普通文件")
	}
	return f, nil
}

// Body returns indexed text for the preview panel.
func (s *Store) Body(ctx context.Context, path string) (string, error) {
	var body string
	err := s.db.QueryRowContext(ctx, "SELECT CAST(body AS BLOB) FROM files WHERE path=?", path).Scan(&body)
	return body, err
}

// Embedder produces normalized embeddings in a shared text/image space.
type Embedder interface {
	Encode(context.Context, string, string, string) ([]float32, error)
}

// Pending is a chunk awaiting a semantic embedding.
type Pending struct {
	ID                     int64
	Path, Body, Kind, Name string
}

// PendingChunks returns a bounded batch; video and unsupported files remain lexical-only.
func (s *Store) PendingChunks(ctx context.Context) ([]Pending, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT c.id,c.path,"+chunkBodySQL+",f.kind,f.name FROM chunks c JOIN files f ON f.path=c.path WHERE (c.vector IS NOT NULL)=0 AND f.kind IN ('document','image') AND f.placeholder=0 ORDER BY c.id LIMIT 32")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pending{}
	for rows.Next() {
		var p Pending
		if err = rows.Scan(&p.ID, &p.Path, &p.Body, &p.Kind, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaveVector validates a model result before persistence.
func (s *Store) SaveVector(ctx context.Context, id int64, v []float32) error {
	if len(v) != 256 {
		return fmt.Errorf("期望 256 维向量，实际 %d", len(v))
	}
	var norm float64
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return errors.New("模型返回无效向量")
		}
		norm += float64(x) * float64(x)
	}
	if norm < 1e-12 {
		return errors.New("模型返回零向量")
	}
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(float32(float64(x)/math.Sqrt(norm))))
	}
	_, err := s.db.ExecContext(ctx, "UPDATE chunks SET vector=? WHERE id=?", b, id)
	return err
}

// EmbedPending indexes pending chunks until cancellation or the first inference failure.
func (s *Store) EmbedPending(ctx context.Context, e Embedder, progress func(int)) error {
	count := 0
	for {
		batch, err := s.PendingChunks(ctx)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, p := range batch {
			if _, err = s.VerifiedPath(ctx, p.Path); err != nil {
				return fmt.Errorf("请重新扫描后再生成向量：%w", err)
			}
			v, err := e.Encode(ctx, p.Kind, p.Path, "title: "+p.Name+" | text: "+p.Body)
			if err != nil {
				return err
			}
			if err = s.SaveVector(ctx, p.ID, v); err != nil {
				return err
			}
			count++
			if progress != nil {
				progress(count)
			}
		}
	}
}

// LastModified converts a stored nanosecond timestamp to a time.
func LastModified(n int64) time.Time { return time.Unix(0, n) }

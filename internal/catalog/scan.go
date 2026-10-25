package catalog

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ScanProgress describes background work without claiming an unknowable total.
type ScanProgress struct {
	Processed int      `json:"processed"`
	Changed   int      `json:"changed"`
	Skipped   int      `json:"skipped"`
	Current   string   `json:"current"`
	Warnings  []string `json:"warnings"`
}

// Scan refreshes all selected roots. Cancellation preserves unvisited entries.
func (s *Store) Scan(ctx context.Context, progress func(ScanProgress)) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	roots, err := s.Roots(ctx)
	if err != nil {
		return err
	}
	p := ScanProgress{Warnings: []string{}}
	generation := time.Now().UnixNano()
	warn := func(message string) {
		p.Skipped++
		if len(p.Warnings) < 8 {
			p.Warnings = append(p.Warnings, message)
		}
	}
	for _, root := range roots {
		var unreadable []string
		// Keep compact row IDs in memory instead of rewriting every document on each poll.
		var unchanged []int64
		process := func(path string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				unreadable = append(unreadable, path)
				warn(path + ": " + walkErr.Error())
				return nil
			}
			if path == root.Path {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if s.skipDirectory(root.Path, path, name) {
					return filepath.SkipDir
				}
				return nil
			}
			if path == s.dbPath || path == s.dbPath+"-wal" || path == s.dbPath+"-shm" {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				unreadable = append(unreadable, path)
				warn(path + ": " + err.Error())
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			p.Processed++
			p.Current = path
			placeholder := isPlaceholder(info)
			var previousPlaceholder bool
			var rowID, size, modified int64
			err = s.db.QueryRowContext(ctx, "SELECT rowid,size,modified,placeholder FROM files WHERE path=?", path).Scan(&rowID, &size, &modified, &previousPlaceholder)
			if err == nil && size == info.Size() && modified == info.ModTime().UnixNano() && (placeholder == previousPlaceholder || placeholder) {
				unchanged = append(unchanged, rowID)
				if placeholder != previousPlaceholder {
					if _, err = s.db.ExecContext(ctx, "UPDATE files SET placeholder=? WHERE path=?", placeholder, path); err != nil {
						return err
					}
				}
				if progress != nil && p.Processed%20 == 0 {
					progress(p)
				}
				return nil
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			kind := Kind(path)
			body := ""
			if kind == "document" && !placeholder {
				body, err = extract(ctx, path)
				if err != nil {
					warn(filepath.Base(path) + ": 正文提取失败，仍可按文件名搜索")
				}
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if rowID == 0 {
				_, err = tx.ExecContext(ctx, `INSERT INTO files(path,root,name,kind,size,modified,body,seen,placeholder) VALUES(?,?,?,?,?,?,?,?,?)`, path, root.Path, filepath.Base(path), kind, info.Size(), info.ModTime().UnixNano(), body, generation, placeholder)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE files SET root=?,name=?,kind=?,size=?,modified=?,body=?,seen=?,placeholder=? WHERE path=?`, root.Path, filepath.Base(path), kind, info.Size(), info.ModTime().UnixNano(), body, generation, placeholder, path)
			}
			if err == nil {
				_, err = tx.ExecContext(ctx, "DELETE FROM chunks WHERE path=?", path)
			}
			chunks := chunkText(body)
			if kind == "image" {
				chunks = []string{""}
			} else if kind == "document" && len(chunks) == 0 {
				chunks = []string{filepath.Base(path)}
			}
			for index, chunk := range chunks {
				if err != nil {
					break
				}
				start, span := -1, 0
				stored := chunk
				if kind == "document" && body != "" {
					start, span = index*1050, len([]rune(chunk))
					stored = ""
				}
				_, err = tx.ExecContext(ctx, "INSERT INTO chunks(path,body,start,span) VALUES(?,?,?,?)", path, stored, start, span)
			}
			if err != nil {
				tx.Rollback()
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			p.Changed++
			if progress != nil {
				progress(p)
			}
			return nil
		}
		// Register videos/audio and other direct children before large subtrees.
		entries, readErr := os.ReadDir(root.Path)
		if readErr == nil {
			for _, entry := range entries {
				path := filepath.Join(root.Path, entry.Name())
				if !entry.IsDir() && Kind(path) != "document" {
					if err = process(path, entry, nil); err != nil {
						return err
					}
				}
			}
		}
		err = filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, walkErr error) error {
			if readErr == nil && walkErr == nil && filepath.Dir(path) == root.Path && !d.IsDir() && Kind(path) != "document" {
				return nil
			}
			return process(path, d, walkErr)
		})
		if err != nil {
			return err
		}
		if err = s.cleanUnseen(ctx, root.Path, generation, unreadable, unchanged); err != nil {
			return err
		}

	}
	if progress != nil {
		progress(p)
	}
	return nil
}

// skipDirectory excludes generated/runtime assets while keeping cloud document folders.
func (s *Store) skipDirectory(root, path, name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasSuffix(strings.ToLower(name), ".app") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
		return true
	}
	if path == filepath.Dir(s.dbPath) {
		return true
	}
	home, _ := os.UserHomeDir()
	if filepath.Dir(path) == filepath.Join(home, "Library") && name != "CloudStorage" && name != "Mobile Documents" {
		return true
	}
	if filepath.Dir(path) == root && root != home {
		switch name {
		case "System", "Library", "Windows", "Program Files", "Program Files (x86)", "System Volume Information", "$RECYCLE.BIN":
			return true
		}
	}
	if name == "Resources" {
		if _, err := os.Stat(filepath.Join(path, "model-manifest.json")); err == nil {
			return true
		}
	}
	return false
}

// cleanUnseen preserves inaccessible subtrees, but removes stale files elsewhere.
func (s *Store) cleanUnseen(ctx context.Context, root string, generation int64, unreadable []string, unchanged []int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(unreadable) == 0 && len(unchanged) == 0 {
		_, err := s.db.ExecContext(ctx, "DELETE FROM files WHERE root=? AND seen<>?", root, generation)
		return err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT rowid,path FROM files WHERE root=? AND seen<>?", root, generation)
	if err != nil {
		return err
	}
	slices.Sort(unchanged)
	var stale []string
	for rows.Next() {
		var id int64
		var path string
		if err = rows.Scan(&id, &path); err != nil {
			rows.Close()
			return err
		}
		if _, found := slices.BinarySearch(unchanged, id); found {
			continue
		}
		protected := false
		for _, failed := range unreadable {
			if path == failed || Within(failed, path) {
				protected = true
				break
			}
		}
		if !protected {
			stale = append(stale, path)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return ctx.Err()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, path := range stale {
		if _, err = tx.ExecContext(ctx, "DELETE FROM files WHERE path=?", path); err != nil {
			return err
		}
	}
	return tx.Commit()
}

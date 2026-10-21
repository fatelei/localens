package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
)

// parallelEvidence bounds both database reads and CPU work while preserving rank order.
func parallelEvidence(ctx context.Context, files []File, enrich func(context.Context, *File) error) ([]File, error) {
	workers := min(4, len(files))
	failures := make([]error, len(files))
	var group sync.WaitGroup
	for worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := worker; i < len(files); i += workers {
				if err := ctx.Err(); err != nil {
					failures[i] = err
					return
				}
				failures[i] = enrich(ctx, &files[i])
			}
		}()
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	valid := make([]File, 0, len(files))
	for i, err := range failures {
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		valid = append(valid, files[i])
	}
	return valid, nil
}

// enrichSearchFile rechecks evidence after the ranking snapshot so changed
// documents cannot display an unrelated passage under a stale literal match.
func (s *Store) enrichSearchFile(ctx context.Context, f *File, query string, chunkID int64, vector []byte, preferLiteral bool) error {
	if f.Match != "正文" && f.Match != "语义" {
		return nil
	}
	var full, name string
	var modified int64
	if err := s.readDB.QueryRowContext(ctx, "SELECT name,modified,body FROM files WHERE path=?", f.Path).Scan(&name, &modified, &full); err != nil {
		return err
	}
	if name != f.Name || modified != f.Modified {
		return sql.ErrNoRows
	}
	rank := int64(-1)
	if f.Match == "正文" || preferLiteral {
		rank = keywordRank(strings.ToLower(name), full, query)
	}
	if f.Match == "正文" && rank != int64(f.Score) {
		return sql.ErrNoRows
	}
	var passage string
	if f.Match == "语义" {
		if chunkID == 0 {
			return sql.ErrNoRows
		}
		if err := s.readDB.QueryRowContext(ctx, "SELECT "+chunkBodySQL+" FROM chunks c JOIN files f ON f.path=c.path WHERE c.id=? AND c.path=? AND c.vector=?", chunkID, f.Path, vector).Scan(&passage); err != nil {
			return err
		}
	}
	if f.Match == "正文" || preferLiteral && rank >= 0 {
		f.Snippet = snippet(full, query)
	} else {
		f.Snippet = snippet(passage, query)
	}

	if IsCode(f.Path) {
		f.Snippet, f.SnippetLine, f.Line = CodeExcerpt(full, query, f.Snippet)
	}
	return nil
}

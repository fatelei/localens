package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// phraseLeaders fills the highest rank tiers first. Once 80 name/phrase hits
// exist, lower-ranking scattered words cannot enter the displayed page.
func (s *Store) phraseLeaders(ctx context.Context, query, kind string) ([]File, bool, error) {
	names, err := s.search(ctx, query, kind, nil, true, false)
	if err != nil {
		return nil, false, err
	}
	leaders := make([]File, 0, 80)
	seen := map[string]bool{}
	for _, f := range names.Files {
		if f.Score >= 10 {
			leaders = append(leaders, f)
			seen[f.Path] = true
		}
	}
	if len(leaders) == 80 {
		return leaders, true, nil
	}
	token := trigramCandidates(query)
	if token == "" {
		return leaders, false, nil
	}
	clause := `path IN (SELECT path FROM content_documents WHERE id IN(SELECT rowid FROM body_fts WHERE body_fts MATCH ?) UNION SELECT path FROM body_fts_pending)`
	args := []any{token}
	if kind != "" && kind != "all" {
		clause += " AND kind=?"
		args = append(args, kind)
	}
	// Read bounded metadata pages and release the cursor before requesting any
	// body connection. Concurrent searches must never exhaust the reader pool
	// while each holds a cursor and waits for another connection.
	const pageSize = 512
	const probeLimit = 8192
	q := strings.ToLower(query)
	var lastModified int64
	lastPath := ""
	for checked := 0; checked < probeLimit; checked += pageSize {
		pageClause := clause
		pageArgs := append([]any{}, args...)
		if checked > 0 {
			pageClause += " AND (modified<? OR (modified=? AND path>?))"
			pageArgs = append(pageArgs, lastModified, lastModified, lastPath)
		}
		rows, e := s.readDB.QueryContext(ctx, "SELECT path,root,name,kind,size,modified FROM file_names WHERE "+pageClause+" ORDER BY modified DESC,path ASC LIMIT ?", append(pageArgs, pageSize)...)
		if e != nil {
			return nil, false, e
		}
		batch := make([]File, 0, pageSize)
		for rows.Next() {
			var f File
			if e = rows.Scan(&f.Path, &f.Root, &f.Name, &f.Kind, &f.Size, &f.Modified); e != nil {
				rows.Close()
				return nil, false, e
			}
			batch = append(batch, f)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, false, e
		}
		for _, f := range batch {
			if seen[f.Path] {
				continue
			}
			var body string
			e = s.readDB.QueryRowContext(ctx, "SELECT body FROM files WHERE path=?", f.Path).Scan(&body)
			if errors.Is(e, sql.ErrNoRows) {
				continue
			}
			if e != nil {
				return nil, false, e
			}
			if searchBodyColumn(query) != "body" {
				body = strings.ToLower(body)
			}
			if !strings.Contains(body, q) {
				continue
			}
			f.Match = "正文"
			f.Score = 9
			leaders = append(leaders, f)
			seen[f.Path] = true
			if len(leaders) == 80 {
				return leaders, true, nil
			}
		}
		if len(batch) < pageSize {
			return leaders, len(strings.Fields(query)) == 1, nil
		}
		last := batch[len(batch)-1]
		lastModified, lastPath = last.Modified, last.Path
	}
	// A large number of trigram false positives uses the complete literal fallback.
	return leaders, false, nil
}

package catalog

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// SearchResult includes the semantic availability and elapsed work from the caller.
type SearchResult struct {
	Files            []File `json:"files"`
	Query            string `json:"query"`
	Semantic         bool   `json:"semantic"`
	SemanticEligible bool   `json:"semanticEligible"`
	ContentEligible  bool   `json:"contentEligible"`
	FilenameOnly     bool   `json:"filenameOnly"`
	Warning          string `json:"warning"`
	Elapsed          int64  `json:"elapsed"`
}

func like(value string) string {
	return "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(value)) + "%"
}
func snippet(body, query string) string {
	r := []rune(body)
	if len(r) == 0 {
		return ""
	}
	start := 0
	lower := strings.ToLower(body)
	at := -1
	if strings.TrimSpace(query) != "" {
		at = strings.Index(lower, strings.ToLower(query))
	}
	if at < 0 {
		for term := range strings.FieldsSeq(strings.ToLower(query)) {
			if found := strings.Index(lower, term); found >= 0 && (at < 0 || found < at) {
				at = found
			}
		}
	}
	if at >= 0 {
		start = max(0, len([]rune(lower[:at]))-45)
	}
	end := min(start+190, len(r))
	v := strings.TrimSpace(string(r[start:end]))
	if start > 0 {
		v = "…" + v
	}
	if end < len(r) {
		v += "…"
	}
	return v
}

// Uncased queries can use SQLite's native matcher without copying and lowercasing
// every document. Cased queries retain the existing Unicode folding semantics.
func searchBodyColumn(query string) string {
	if strings.ToLower(query) == strings.ToUpper(query) {
		return "body"
	}
	return "unicode_lower(body)"
}

// Search performs literal Unicode substring matching and optional exact vector retrieval.
func (s *Store) Search(ctx context.Context, query, kind string, vector []float32) (SearchResult, error) {
	return s.search(ctx, query, kind, vector, false, false)
}

// SearchRelated returns vector candidates without letting literal hits consume the result cap.
func (s *Store) SearchRelated(ctx context.Context, query, kind string, vector []float32) (SearchResult, error) {
	return s.search(ctx, query, kind, vector, false, true)
}

func (s *Store) search(ctx context.Context, query, kind string, vector []float32, filenameOnly, relatedOnly bool) (SearchResult, error) {
	eligible := SemanticQuery(query, kind)
	if !eligible {
		vector = nil
	}
	result := SearchResult{Files: []File{}, Query: strings.TrimSpace(query), Semantic: len(vector) > 0, SemanticEligible: eligible, ContentEligible: KeywordQuery(query, kind)}
	query = strings.TrimSpace(query)
	terms := strings.Fields(query)
	if len([]rune(query)) > 500 || len(terms) > 16 {
		return result, errors.New("搜索词太长，请限制在 500 字和 16 个词以内")
	}
	clause := "1=1"
	if relatedOnly {
		clause = "0=1"
	}
	args := []any{}
	if kind != "" && kind != "all" {
		clause += " AND kind=?"
		args = append(args, kind)
	}
	shortQuery := shortKeywords(query) && !eligible && kind != "image"
	result.FilenameOnly = filenameOnly || shortQuery || kind == "video" || kind == "audio"
	for _, term := range terms {
		if !filenameOnly && !relatedOnly && !shortQuery {
			table, token := "body_fts", trigramCandidates(term)
			if token == "" {
				table = "body_short_fts"
				token = shortToken(term)
			}
			if token != "" {
				clause += ` AND path IN(SELECT path FROM file_names WHERE name_lower LIKE ? ESCAPE '\' UNION SELECT path FROM content_documents WHERE id IN(SELECT rowid FROM ` + table + ` WHERE ` + table + ` MATCH ?) UNION SELECT path FROM body_fts_pending)`
				args = append(args, like(term), token)
			}
		}
		if shortQuery {
			clause += " AND filename_word_prefix(name,?)=1"
			args = append(args, term)
		} else if filenameOnly {
			clause += ` AND name_lower LIKE ? ESCAPE '\'`
			args = append(args, like(term))
		}
	}
	// Materialize only ranked metadata, never thousands of complete result bodies.
	// This also keeps SQLite from evaluating the body matcher again during sorting.
	metadata := "path,root,name,kind,size,modified"
	table := "(SELECT f.*,n.name_lower FROM files f JOIN file_names n ON n.path=f.path)"
	rank := "keyword_rank(name_lower,body,?)"
	rankArgs := []any{query}
	if filenameOnly || shortQuery || query == "" {
		if filenameOnly || query == "" {
			table = "file_names"
		}
		rank = "0"
		rankArgs = nil
		if query != "" {
			rank = `CASE WHEN name_lower=? THEN 12 WHEN name_lower LIKE ? ESCAPE '\' THEN 10 ELSE 8 END`
			rankArgs = []any{strings.ToLower(query), like(query)}
		}
	}
	sqlQuery := "WITH ranked AS MATERIALIZED (SELECT " + metadata + "," + rank + " AS relevance FROM " + table + " WHERE " + clause + ") SELECT " + metadata + ",relevance FROM ranked WHERE relevance>=0 ORDER BY relevance DESC,modified DESC,path ASC LIMIT 80"
	var leaders []File
	var complete bool
	var err error
	if !relatedOnly && !filenameOnly && !shortQuery && query != "" {
		leaders, complete, err = s.phraseLeaders(ctx, query, kind)
		if err != nil {
			return result, err
		}
	}
	var rows *sql.Rows
	if !complete {
		rows, err = s.readDB.QueryContext(ctx, sqlQuery, append(rankArgs, args...)...)
		if err != nil {
			return result, err
		}
	}
	byPath := map[string]File{}
	evidenceIDs := map[string]int64{}
	evidenceVectors := map[string][]byte{}
	for _, f := range leaders {
		byPath[f.Path] = f
	}
	if rows != nil {
		for rows.Next() {
			var f File
			if err = rows.Scan(&f.Path, &f.Root, &f.Name, &f.Kind, &f.Size, &f.Modified, &f.Score); err != nil {
				rows.Close()
				return result, err
			}
			f.Match = "正文"
			if query == "" {
				f.Match = "最近修改"
			} else if f.Score >= 10 || filenameOnly || shortQuery {
				f.Match = "文件名"
			}
			byPath[f.Path] = f
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
	}
	if len(vector) > 0 {
		if len(vector) != 256 {
			return result, errors.New("查询向量维度错误")
		}
		var norm float64
		for _, x := range vector {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return result, errors.New("查询向量无效")
			}
			norm += float64(x) * float64(x)
		}
		if norm < 1e-12 {
			return result, errors.New("查询向量为零")
		}
		norm = math.Sqrt(norm)
		sqlQuery := "SELECT f.path,f.root,f.name,f.kind,f.size,f.modified,c.id,c.vector FROM chunks c JOIN files f ON f.path=c.path WHERE (c.vector IS NOT NULL)=1"
		vargs := []any{}
		if kind != "" && kind != "all" {
			sqlQuery += " AND f.kind=?"
			vargs = append(vargs, kind)
		}
		rows, err = s.readDB.QueryContext(ctx, sqlQuery, vargs...)
		if err != nil {
			return result, err
		}
		defer rows.Close()
		for rows.Next() {
			var f File
			var chunkID int64
			var b []byte
			if err = rows.Scan(&f.Path, &f.Root, &f.Name, &f.Kind, &f.Size, &f.Modified, &chunkID, &b); err != nil {
				return result, err
			}
			if len(b) != 4*len(vector) {
				continue
			}
			var dot float64
			for i, x := range vector {
				dot += float64(x) * float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))) / norm
			}
			if dot < 0.65 {
				continue
			}
			f.Score = dot * 3
			f.Match = "语义"
			if old, ok := byPath[f.Path]; !ok || f.Score > old.Score {
				evidenceIDs[f.Path] = chunkID
				evidenceVectors[f.Path] = slices.Clone(b)
				byPath[f.Path] = f
			}
		}
		if err = rows.Err(); err != nil {
			return result, err
		}
		rows.Close()
	}
	for _, f := range byPath {
		result.Files = append(result.Files, f)
	}
	slices.SortFunc(result.Files, func(a, b File) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Modified, a.Modified); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})
	limit := 80
	if relatedOnly {
		limit = 160
	} // Up to 80 keyword paths are removed by the caller.
	if len(result.Files) > limit {
		result.Files = result.Files[:limit]
	}
	result.Files, err = parallelEvidence(ctx, result.Files, func(ctx context.Context, f *File) error {
		return s.enrichSearchFile(ctx, f, query, evidenceIDs[f.Path], evidenceVectors[f.Path], !relatedOnly)
	})
	if err != nil {
		return result, err
	}

	return result, nil
}

// Short Latin keywords use filename word prefixes, rather than arbitrary inner substrings.
func shortKeyword(query string) bool {
	if len(query) == 0 || len(query) > 3 {
		return false
	}
	for _, r := range query {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
func filenameWordPrefix(name, term string) bool {
	name = strings.ToLower(name)
	term = strings.ToLower(term)
	var previous rune
	for offset, r := range name {
		if (offset == 0 || (!unicode.IsLetter(previous) && !unicode.IsDigit(previous))) && strings.HasPrefix(name[offset:], term) {
			return true
		}
		previous = r
	}
	return false
}

// SemanticQuery avoids inventing meaning for short keywords, paths, filenames and unsupported media.
// KeywordQuery controls literal body retrieval independently of model suitability.
func KeywordQuery(query, kind string) bool {
	query = strings.TrimSpace(query)
	if query == "" || kind == "video" || kind == "audio" || strings.ContainsAny(query, `/\`) {
		return false
	}
	if kind != "image" && shortKeywords(query) && len(strings.Fields(query)) < 3 {
		return false
	}
	if len(strings.Fields(query)) == 1 && Kind(query) != "other" {
		return false
	}
	return true
}

func SemanticQuery(query, kind string) bool {
	query = strings.TrimSpace(query)
	if query == "" || kind == "video" || kind == "audio" || (shortKeyword(query) || (shortKeywords(query) && len(strings.Fields(query)) < 3 && kind != "image")) || strings.ContainsAny(query, `/\`) {
		return false
	}
	if len(strings.Fields(query)) == 1 {
		if strings.Contains(query, "_") {
			return false
		}
		if Kind(query) != "other" {
			return false
		}
		switch strings.ToLower(filepath.Ext(query)) {
		case ".zip", ".tar", ".gz", ".7z", ".exe", ".app", ".dmg", ".deb", ".msi", ".pkg", ".iso":
			return false
		}
	}
	return true
}

func shortKeywords(query string) bool {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return false
	}
	for _, term := range terms {
		if !shortKeyword(term) {
			return false
		}
	}
	return true
}

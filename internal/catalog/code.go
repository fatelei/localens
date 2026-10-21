package catalog

import (
	"path/filepath"
	"strings"
)

// IsCode identifies text formats that benefit from an editor and line navigation.
func IsCode(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".html", ".css", ".rs", ".java", ".c", ".cpp", ".h", ".sql", ".json", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}

// CodeExcerpt preserves code line breaks and maps semantic excerpts back to the full file.
// Unknown positions return zero instead of inventing a location.
func CodeExcerpt(body, query, evidence string) (string, int, int) {
	// Prefer the supplied passage, so a semantic chunk is not replaced by an unrelated word elsewhere.
	offset := -1
	anchor := strings.Trim(strings.TrimSpace(evidence), "…")
	if anchor != "" {
		if at := strings.Index(body, anchor); at >= 0 {
			offset = len([]rune(body[:at]))
			if relative := codeMatchOffset(anchor, query); relative >= 0 {
				offset += relative
			}
		}
	}
	if offset < 0 {
		offset = codeMatchOffset(body, query)
	}
	if offset < 0 {
		return evidence, 0, 0
	}
	runes := []rune(body)
	if offset > len(runes) {
		return evidence, 0, 0
	}
	line := 1 + strings.Count(string(runes[:offset]), "\n")
	lines := strings.Split(body, "\n")
	start := max(0, line-2)
	end := min(len(lines), line+1)
	excerpt := strings.Join(lines[start:end], "\n")
	// Bound pathological generated lines while keeping the matching region visible.
	if len([]rune(excerpt)) > 600 {
		return snippet(body, query), 0, line
	}
	return excerpt, start + 1, line
}

func codeMatchOffset(body, query string) int {
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
	if at < 0 {
		return -1
	}
	return len([]rune(lower[:at]))
}

package catalog

import "strings"

// keywordRank verifies all terms and computes phrase priority with one body normalization.
// A negative score rejects false-positive postings; name is already normalized.
func keywordRank(name, body, query string) int64 {
	q := strings.ToLower(query)
	if q == "" {
		return 0
	}
	if name == q {
		return 12
	}
	if strings.Contains(name, q) {
		return 10
	}
	lower := body
	if searchBodyColumn(query) != "body" {
		lower = strings.ToLower(body)
	}
	for term := range strings.FieldsSeq(q) {
		if !strings.Contains(name, term) && !strings.Contains(lower, term) {
			return -1
		}
	}
	if strings.Contains(lower, q) {
		return 9
	}
	return 2
}

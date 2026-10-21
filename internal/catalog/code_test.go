package catalog

import (
	"strings"
	"testing"
)

func TestCodeExcerpt(t *testing.T) {
	body := "// 中文\npackage main\nfunc searchFiles() {\n\tprintln(\"hello\")\n}\n"
	excerpt, start, line := CodeExcerpt(body, "searchFiles", "")
	if start != 2 || line != 3 || !strings.Contains(excerpt, "\n\tprintln") {
		t.Fatalf("%q start=%d line=%d", excerpt, start, line)
	}
	_, _, line = CodeExcerpt(body, "unrelated description", "func searchFiles() {\n\tprintln(\"hello\")")
	if line != 3 {
		t.Fatalf("semantic line=%d", line)
	}
	_, _, line = CodeExcerpt(body, "absent", "unknown")
	if line != 0 {
		t.Fatalf("invented location %d", line)
	}
}

func TestCodeExcerptUnicodeSemanticAnchor(t *testing.T) {
	body := strings.Repeat("İK", 20) + "\n// related above\n\nfunc target() {}"
	_, _, line := CodeExcerpt(body, "related concept", "func target() {}")
	if line != 4 {
		t.Fatalf("wrong semantic anchor line %d", line)
	}
}

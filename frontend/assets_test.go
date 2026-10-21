package frontend

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestHTMLReferencesResolveInsideEmbeddedAssets(t *testing.T) {
	html, err := Assets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	references := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllSubmatch(html, -1)
	for _, match := range references {
		name := string(match[1])
		// Wails serves its runtime dynamically rather than embedding it here.
		if name == "/wails/runtime.js" {
			continue
		}
		if strings.HasPrefix(name, "#") || strings.Contains(name, ":") {
			continue
		}
		if _, err := fs.Stat(Assets, name); err != nil {
			t.Errorf("missing embedded resource %s: %v", name, err)
		}
	}
	if len(references) == 0 {
		t.Fatal("no static assets found in entry page")
	}
	if _, err := fs.Stat(Assets, "assets.go"); err == nil {
		t.Fatal("developer Go source exposed as a UI asset")
	}
}

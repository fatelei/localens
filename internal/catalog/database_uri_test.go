package catalog

import (
	"net/url"
	"testing"
)

func TestDatabaseFileURLPreservesWindowsDriveAndEscapesPath(t *testing.T) {
	for _, path := range []string{"C:/Users/test/index.db", "D:/资料/my index?#.db", "/tmp/my index?#.db"} {
		t.Run(path, func(t *testing.T) {
			uri := databaseFileURL(path)
			parsed, err := url.Parse(uri.String())
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Host != "" {
				t.Fatalf("SQLite URI must not have an authority: %s", uri)
			}
			want := path
			if path[0] != '/' {
				want = "/" + path
			}
			if parsed.Path != want || parsed.RawQuery != "" || parsed.Fragment != "" {
				t.Fatalf("URI changed the database path: %s -> %#v", path, parsed)
			}
		})
	}
}

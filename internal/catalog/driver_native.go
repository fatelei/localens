//go:build darwin && cgo && sqlite_fts5

package catalog

import (
	"database/sql"
	"strings"
	"sync"

	sqlite "github.com/mattn/go-sqlite3"
)

const databaseDriver = "localens_native_sqlite"

// Keep Unicode matching identical to the portable driver while using native
// SQLite for large document scans on the desktop release.
var registerUnicodeLower = sync.OnceValue(func() error {
	sql.Register(databaseDriver, &sqlite.SQLiteDriver{ConnectHook: func(conn *sqlite.SQLiteConn) error {
		if err := conn.RegisterFunc("fragment_tail", func(body []byte, skip int64) []byte { return []byte(fragmentTail(string(body), skip)) }, true); err != nil {
			return err
		}
		if err := conn.RegisterFunc("body_fragments", func(body []byte) string { return bodyFragmentsJSON(string(body)) }, true); err != nil {
			return err
		}
		if err := conn.RegisterFunc("keyword_rank", keywordRank, true); err != nil {
			return err
		}
		if err := conn.RegisterFunc("unicode_lower", strings.ToLower, true); err != nil {
			return err
		}
		if err := conn.RegisterFunc("unicode_short_tokens", shortTokens, true); err != nil {
			return err
		}
		return conn.RegisterFunc("filename_word_prefix", func(name, term string) int64 {
			if filenameWordPrefix(name, term) {
				return 1
			}
			return 0
		}, true)
	}})
	return nil
})

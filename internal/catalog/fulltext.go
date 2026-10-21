package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// initializeFullText creates the compact postings index and a restartable backfill queue.
func initializeFullText(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='content_schema'").Scan(&version); err != nil {
		return err
	}
	if version != 0 {
		if err = tx.QueryRow("SELECT version FROM content_schema WHERE id=1").Scan(&version); err != nil {
			return err
		}
		if version == 1 {
			return tx.Commit()
		}
		return fmt.Errorf("unsupported content schema version %d", version)
	}
	// Upgrade older single-index schemas atomically; original content and vectors stay intact.
	if _, err = tx.Exec(`
 DROP TRIGGER IF EXISTS body_fts_insert; DROP TRIGGER IF EXISTS body_fts_delete; DROP TRIGGER IF EXISTS body_fts_update;
 DROP TRIGGER IF EXISTS content_revision_insert; DROP TRIGGER IF EXISTS content_revision_delete; DROP TRIGGER IF EXISTS content_revision_update;
 DROP VIEW IF EXISTS file_search_content; DROP VIEW IF EXISTS file_short_content;
 DROP TABLE IF EXISTS body_fts; DROP TABLE IF EXISTS body_short_fts;
 DROP TABLE IF EXISTS body_fts_pending; DROP TABLE IF EXISTS content_documents; DROP TABLE IF EXISTS content_revision;
 `); err != nil {
		return err
	}
	_, err = tx.Exec(`
 CREATE TABLE content_schema(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL);
 INSERT INTO content_schema VALUES(1,1);
 CREATE TABLE content_revision(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL);
 INSERT INTO content_revision VALUES(1,0);
 CREATE TRIGGER content_revision_insert AFTER INSERT ON files BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TRIGGER content_revision_delete AFTER DELETE ON files BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TRIGGER content_revision_update AFTER UPDATE OF path,root,name,kind,size,modified,body ON files WHEN OLD.path<>NEW.path OR OLD.root<>NEW.root OR OLD.name<>NEW.name OR OLD.kind<>NEW.kind OR OLD.size<>NEW.size OR OLD.modified<>NEW.modified OR OLD.body<>NEW.body BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TABLE content_documents(id INTEGER PRIMARY KEY AUTOINCREMENT,path TEXT NOT NULL UNIQUE);
 INSERT INTO content_documents(path) SELECT path FROM files WHERE body<>'' ORDER BY modified DESC;
 CREATE VIEW file_search_content AS SELECT d.id AS rowid,unicode_lower(f.body) AS text FROM files f JOIN content_documents d ON d.path=f.path;
 CREATE VIEW file_short_content AS SELECT d.id AS rowid,unicode_short_tokens(f.body) AS text FROM files f JOIN content_documents d ON d.path=f.path;
 CREATE VIRTUAL TABLE body_short_fts USING fts5(text,content='file_short_content',content_rowid='rowid',tokenize='unicode61',detail=none,columnsize=0);
 CREATE VIRTUAL TABLE body_fts USING fts5(text,content='file_search_content',content_rowid='rowid',tokenize='trigram case_sensitive 1',detail=none,columnsize=0);
 CREATE TABLE body_fts_pending(id INTEGER PRIMARY KEY,path TEXT NOT NULL UNIQUE);
 INSERT INTO body_fts_pending(path) SELECT path FROM files WHERE body<>'' ORDER BY modified DESC;
 CREATE TRIGGER body_fts_insert AFTER INSERT ON files WHEN NEW.body<>'' BEGIN
 INSERT INTO content_documents(path) VALUES(NEW.path);
 INSERT INTO body_fts(rowid,text) SELECT id,unicode_lower(NEW.body) FROM content_documents WHERE path=NEW.path;
 INSERT INTO body_short_fts(rowid,text) SELECT id,unicode_short_tokens(NEW.body) FROM content_documents WHERE path=NEW.path; END;
 CREATE TRIGGER body_fts_delete AFTER DELETE ON files BEGIN
 INSERT INTO body_fts(body_fts,rowid,text) SELECT 'delete',id,unicode_lower(OLD.body) FROM content_documents WHERE path=OLD.path AND OLD.body<>'' AND NOT EXISTS(SELECT 1 FROM body_fts_pending WHERE path=OLD.path);
 INSERT INTO body_short_fts(body_short_fts,rowid,text) SELECT 'delete',id,unicode_short_tokens(OLD.body) FROM content_documents WHERE path=OLD.path AND OLD.body<>'' AND NOT EXISTS(SELECT 1 FROM body_fts_pending WHERE path=OLD.path);
 DELETE FROM body_fts_pending WHERE path=OLD.path; DELETE FROM content_documents WHERE path=OLD.path; END;
 CREATE TRIGGER body_fts_update AFTER UPDATE OF body,path ON files WHEN OLD.body<>NEW.body OR OLD.path<>NEW.path BEGIN
 INSERT INTO body_fts(body_fts,rowid,text) SELECT 'delete',id,unicode_lower(OLD.body) FROM content_documents WHERE path=OLD.path AND OLD.body<>'' AND NOT EXISTS(SELECT 1 FROM body_fts_pending WHERE path=OLD.path);
 INSERT INTO body_short_fts(body_short_fts,rowid,text) SELECT 'delete',id,unicode_short_tokens(OLD.body) FROM content_documents WHERE path=OLD.path AND OLD.body<>'' AND NOT EXISTS(SELECT 1 FROM body_fts_pending WHERE path=OLD.path);
 UPDATE content_documents SET path=NEW.path WHERE path=OLD.path;
 INSERT INTO content_documents(path) SELECT NEW.path WHERE NEW.body<>'' AND NOT EXISTS(SELECT 1 FROM content_documents WHERE path=NEW.path);
 INSERT INTO body_fts(rowid,text) SELECT id,unicode_lower(NEW.body) FROM content_documents WHERE path=NEW.path AND NEW.body<>'';
 INSERT INTO body_short_fts(rowid,text) SELECT id,unicode_short_tokens(NEW.body) FROM content_documents WHERE path=NEW.path AND NEW.body<>'';
 DELETE FROM body_fts_pending WHERE path=OLD.path; END;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// indexTextBatch atomically backfills a small batch without blocking all future scans.
func (s *Store) indexTextBatch(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT q.id,d.id,f.body FROM body_fts_pending q JOIN files f ON f.path=q.path JOIN content_documents d ON d.path=f.path ORDER BY q.id LIMIT 32")
	if err != nil {
		return 0, err
	}
	type entry struct {
		id, rowid int64
		body      string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.rowid, &e.body); err != nil {
			rows.Close()
			return 0, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if _, err = tx.ExecContext(ctx, "INSERT INTO body_fts(rowid,text) VALUES(?,?)", e.rowid, strings.ToLower(e.body)); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO body_short_fts(rowid,text) VALUES(?,?)", e.rowid, shortTokens(e.body)); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM body_fts_pending WHERE id=?", e.id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// trigramCandidates returns quoted three-rune tokens. Exact verification still
// runs on the candidates, since detail=none does not store token positions.
func trigramCandidates(term string) string {
	r := []rune(strings.ToLower(term))
	if len(r) < 3 {
		return ""
	}
	seen := map[string]bool{}
	var tokens []string
	for i := 0; i+3 <= len(r); i++ {
		token := string(r[i : i+3])
		if !seen[token] {
			seen[token] = true
			tokens = append(tokens, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
		}
	}
	return strings.Join(tokens, " AND ")
}

func shortToken(term string) string {
	r := []rune(strings.ToLower(term))
	if len(r) == 1 && r[0] > 127 {
		return "u" + strconv.FormatInt(int64(r[0]), 16)
	}
	if len(r) == 2 && (r[0] > 127 || r[1] > 127) {
		return "p" + strconv.FormatInt(int64(r[0]), 16) + "x" + strconv.FormatInt(int64(r[1]), 16)
	}
	return ""
}

// shortTokens supplies compact postings for single/two-character Unicode terms.
func shortTokens(body string) string {
	seen := map[string]bool{}
	var out strings.Builder
	var previous rune
	hasPrevious := false
	add := func(token string) {
		if !seen[token] {
			seen[token] = true
			out.WriteString(token)
			out.WriteByte(' ')
		}
	}
	for _, r := range strings.ToLower(body) {
		if r > 127 {
			add("u" + strconv.FormatInt(int64(r), 16))
		}
		if hasPrevious && (r > 127 || previous > 127) {
			add("p" + strconv.FormatInt(int64(previous), 16) + "x" + strconv.FormatInt(int64(r), 16))
		}
		previous = r
		hasPrevious = true
	}
	return out.String()
}

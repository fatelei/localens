package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Fragments include query-sized overlap; references omit it when rebuilding a body.
type bodyFragment struct {
	Hash string `json:"hash"`
	Body string `json:"body"`
	Skip int    `json:"skip"`
}

func splitBodyFragments(body string) []bodyFragment {
	runes := []rune(body)
	out := []bodyFragment{}
	start := 0
	var rolling uint64
	emit := func(end int) {
		prefix := max(0, start-499)
		text := string(runes[prefix:end])
		sum := sha256.Sum256([]byte(text))
		out = append(out, bodyFragment{hex.EncodeToString(sum[:]), text, start - prefix})
		start = end
		rolling = 0
	}
	for i, r := range runes {
		// A gear hash forgets old prefix changes after 64 characters.
		x := uint64(r) + 0x9e3779b97f4a7c15
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		rolling = (rolling << 1) + (x ^ (x >> 31))
		n := i + 1 - start
		if n >= 4096 && (rolling&8191 == 0 || n >= 16384) {
			emit(i + 1)
		}
	}
	if start < len(runes) {
		emit(len(runes))
	}
	return out
}

// fragmentTail preserves embedded NULs, unlike SQLite's TEXT substr().
func fragmentTail(body string, skip int64) string {
	if skip <= 0 {
		return body
	}
	for offset := range body {
		if skip == 0 {
			return body[offset:]
		}
		skip--
	}
	return ""
}

func bodyFragmentsJSON(body string) string {
	b, _ := json.Marshal(splitBodyFragments(body))
	return string(b)
}

// ContentStorageAnalysis compares live text bytes to a shared-fragment representation.
type ContentStorageAnalysis struct {
	Files           int64 `json:"files"`
	OriginalBytes   int64 `json:"originalBytes"`
	UniqueBytes     int64 `json:"uniqueBytes"`
	References      int64 `json:"references"`
	UniqueFragments int64 `json:"uniqueFragments"`
}

// AnalyzeContent reads one bounded batch at a time; it never modifies the index.
func (s *Store) AnalyzeContent(ctx context.Context) (ContentStorageAnalysis, error) {
	var result ContentStorageAnalysis
	seen := map[string]bool{}
	var last int64
	for {
		rows, err := s.readDB.QueryContext(ctx, "SELECT rowid,CAST(body AS BLOB) FROM files WHERE rowid>? ORDER BY rowid LIMIT 64", last)
		if err != nil {
			return result, err
		}
		type item struct {
			id   int64
			body string
		}
		batch := []item{}
		for rows.Next() {
			var x item
			if err = rows.Scan(&x.id, &x.body); err != nil {
				rows.Close()
				return result, err
			}
			batch = append(batch, x)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		if len(batch) == 0 {
			return result, nil
		}
		for _, x := range batch {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			last = x.id
			if x.body == "" {
				continue
			}
			result.Files++
			result.OriginalBytes += int64(len(x.body))
			for _, p := range splitBodyFragments(x.body) {
				result.References++
				if !seen[p.Hash] {
					seen[p.Hash] = true
					result.UniqueFragments++
					result.UniqueBytes += int64(len(p.Body))
				}
			}
		}
	}
}

const sharedTextDDL = `
 CREATE TABLE body_parts(id INTEGER PRIMARY KEY,hash TEXT NOT NULL UNIQUE,body TEXT NOT NULL);
 CREATE TABLE body_refs(path TEXT NOT NULL REFERENCES file_records(path) ON DELETE CASCADE ON UPDATE CASCADE,ordinal INTEGER NOT NULL,part_id INTEGER NOT NULL REFERENCES body_parts(id),skip INTEGER NOT NULL,PRIMARY KEY(path,ordinal)) WITHOUT ROWID;
 CREATE INDEX body_refs_part ON body_refs(part_id,path);
 CREATE TRIGGER body_refs_collect AFTER DELETE ON body_refs BEGIN
 DELETE FROM body_parts WHERE id=OLD.part_id AND NOT EXISTS(SELECT 1 FROM body_refs WHERE part_id=OLD.part_id); END;
 CREATE TABLE shared_body_schema(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL);
 INSERT INTO shared_body_schema VALUES(1,1);
`

const sharedFTSDDL = `
 DROP TRIGGER IF EXISTS body_parts_fts_insert; DROP TRIGGER IF EXISTS body_parts_fts_delete;
 DROP TABLE IF EXISTS body_fts; DROP TABLE IF EXISTS body_short_fts;
 DROP VIEW IF EXISTS file_search_content; DROP VIEW IF EXISTS file_short_content;
 CREATE VIEW file_search_content AS SELECT id AS rowid,unicode_lower(body) AS text FROM body_parts;
 CREATE VIEW file_short_content AS SELECT id AS rowid,unicode_short_tokens(body) AS text FROM body_parts;
 CREATE VIRTUAL TABLE body_fts USING fts5(text,content='file_search_content',content_rowid='rowid',tokenize='trigram case_sensitive 1',detail=none,columnsize=0);
 CREATE VIRTUAL TABLE body_short_fts USING fts5(text,content='file_short_content',content_rowid='rowid',tokenize='unicode61',detail=none,columnsize=0);
 CREATE TRIGGER body_parts_fts_insert AFTER INSERT ON body_parts BEGIN
 INSERT INTO body_fts(rowid,text) VALUES(NEW.id,unicode_lower(NEW.body));
 INSERT INTO body_short_fts(rowid,text) VALUES(NEW.id,unicode_short_tokens(NEW.body)); END;
 CREATE TRIGGER body_parts_fts_delete AFTER DELETE ON body_parts BEGIN
 INSERT INTO body_fts(body_fts,rowid,text) VALUES('delete',OLD.id,unicode_lower(OLD.body));
 INSERT INTO body_short_fts(body_short_fts,rowid,text) VALUES('delete',OLD.id,unicode_short_tokens(OLD.body)); END;
 CREATE TABLE IF NOT EXISTS content_schema(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL);
 INSERT OR REPLACE INTO content_schema VALUES(1,2);
`

// insertBodyRefsSQL checks the bytes as well as the digest before sharing content.
func insertBodyRefsSQL(path, body string) string {
	return `INSERT OR IGNORE INTO body_parts(hash,body) SELECT json_extract(value,'$.hash'),json_extract(value,'$.body') FROM json_each(body_fragments(CAST(` + body + ` AS BLOB)));
 SELECT CASE WHEN EXISTS(SELECT 1 FROM json_each(body_fragments(CAST(` + body + ` AS BLOB))) j JOIN body_parts p ON p.hash=json_extract(j.value,'$.hash') WHERE p.body<>json_extract(j.value,'$.body')) THEN RAISE(ABORT,'content hash collision') END;
 INSERT INTO body_refs(path,ordinal,part_id,skip) SELECT ` + path + `,CAST(j.key AS INTEGER),p.id,json_extract(j.value,'$.skip') FROM json_each(body_fragments(CAST(` + body + ` AS BLOB))) j JOIN body_parts p ON p.hash=json_extract(j.value,'$.hash');`
}

// DeduplicateContent is offline maintenance. The complete schema change is atomic;
// failure/cancellation leaves the legacy database usable by the previous binary.
func (s *Store) DeduplicateContent(ctx context.Context) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name='shared_body_schema'").Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `
 DROP TRIGGER IF EXISTS body_fts_insert; DROP TRIGGER IF EXISTS body_fts_update; DROP TRIGGER IF EXISTS body_fts_delete;
 DROP TRIGGER IF EXISTS content_revision_insert; DROP TRIGGER IF EXISTS content_revision_update; DROP TRIGGER IF EXISTS content_revision_delete;
 DROP VIEW IF EXISTS file_search_content; DROP VIEW IF EXISTS file_short_content;
 DROP TABLE IF EXISTS body_fts; DROP TABLE IF EXISTS body_short_fts;
 DROP TABLE IF EXISTS body_fts_pending; DROP TABLE IF EXISTS content_documents;
 ALTER TABLE files RENAME TO file_records;`+sharedTextDDL+sharedFTSDDL)
	if err != nil {
		return fmt.Errorf("prepare shared content: %w", err)
	}
	var last int64
	for {
		rows, e := tx.QueryContext(ctx, "SELECT rowid,path,CAST(body AS BLOB) FROM file_records WHERE rowid>? ORDER BY rowid LIMIT 64", last)
		if e != nil {
			return e
		}
		type item struct {
			id         int64
			path, body string
		}
		batch := []item{}
		for rows.Next() {
			var x item
			if e = rows.Scan(&x.id, &x.path, &x.body); e != nil {
				rows.Close()
				return e
			}
			batch = append(batch, x)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(batch) == 0 {
			break
		}
		for _, x := range batch {
			last = x.id
			for ordinal, p := range splitBodyFragments(x.body) {
				if _, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO body_parts(hash,body) VALUES(?,?)", p.Hash, p.Body); e != nil {
					return e
				}
				var id int64
				if e = tx.QueryRowContext(ctx, "SELECT id FROM body_parts WHERE hash=? AND body=?", p.Hash, p.Body).Scan(&id); e != nil {
					return fmt.Errorf("fragment collision or missing content: %w", e)
				}
				if _, e = tx.ExecContext(ctx, "INSERT INTO body_refs VALUES(?,?,?,?)", x.path, ordinal, id, p.Skip); e != nil {
					return e
				}
			}
		}
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE file_records DROP COLUMN body;
 CREATE VIEW files AS SELECT f.rowid AS rowid,f.path,f.root,f.name,f.kind,f.size,f.modified,
 COALESCE((SELECT group_concat(piece,'') FROM(SELECT CAST(fragment_tail(CAST(p.body AS BLOB),r.skip) AS TEXT) AS piece FROM body_refs r JOIN body_parts p ON p.id=r.part_id WHERE r.path=f.path ORDER BY r.ordinal)),'') AS body,
 f.seen,f.placeholder,f.ocr_version,f.ocr_error FROM file_records f;
 CREATE VIEW content_documents AS SELECT path,part_id AS id FROM body_refs;
 CREATE TABLE body_fts_pending(id INTEGER PRIMARY KEY,path TEXT NOT NULL UNIQUE);
 CREATE TRIGGER content_revision_insert AFTER INSERT ON file_records BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TRIGGER content_revision_delete AFTER DELETE ON file_records BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TRIGGER content_revision_update AFTER UPDATE ON file_records WHEN OLD.path<>NEW.path OR OLD.root<>NEW.root OR OLD.name<>NEW.name OR OLD.kind<>NEW.kind OR OLD.size<>NEW.size OR OLD.modified<>NEW.modified BEGIN UPDATE content_revision SET version=version+1 WHERE id=1; END;
 CREATE TRIGGER files_shared_insert INSTEAD OF INSERT ON files BEGIN
 INSERT INTO file_records(path,root,name,kind,size,modified,seen,placeholder,ocr_version,ocr_error) VALUES(NEW.path,NEW.root,NEW.name,NEW.kind,NEW.size,NEW.modified,NEW.seen,COALESCE(NEW.placeholder,0),COALESCE(NEW.ocr_version,0),COALESCE(NEW.ocr_error,''));
 `+insertBodyRefsSQL("NEW.path", "COALESCE(NEW.body,'')")+` END;
 CREATE TRIGGER files_shared_delete INSTEAD OF DELETE ON files BEGIN
 DELETE FROM body_fts_pending WHERE path=OLD.path;
 DELETE FROM file_records WHERE path=OLD.path; END;
 CREATE TRIGGER files_shared_update INSTEAD OF UPDATE ON files BEGIN
 UPDATE file_records SET path=NEW.path,root=NEW.root,name=NEW.name,kind=NEW.kind,size=NEW.size,modified=NEW.modified,seen=NEW.seen,placeholder=NEW.placeholder,ocr_version=NEW.ocr_version,ocr_error=NEW.ocr_error WHERE path=OLD.path;
 DELETE FROM body_refs WHERE path=NEW.path AND OLD.body<>COALESCE(NEW.body,'');
 `+insertBodyRefsSQL("NEW.path", "CASE WHEN OLD.body<>COALESCE(NEW.body,'') THEN COALESCE(NEW.body,'') ELSE '' END")+`
 UPDATE content_revision SET version=version+1 WHERE id=1 AND OLD.body<>COALESCE(NEW.body,'');
 DELETE FROM body_fts_pending WHERE path=OLD.path; END;`)
	if err != nil {
		return fmt.Errorf("publish shared content schema: %w", err)
	}
	return tx.Commit()
}

func initializeSharedFullText(db *sql.DB) error {
	var version int
	err := db.QueryRow("SELECT version FROM content_schema WHERE id=1").Scan(&version)
	if err == nil && version == 2 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(sharedFTSDDL + `INSERT INTO body_fts(body_fts) VALUES('rebuild');INSERT INTO body_short_fts(body_short_fts) VALUES('rebuild');`); err != nil {
		return err
	}
	return tx.Commit()
}

// AnalyzeDatabase opens the existing database read-only, without schema upgrades or workers.
func AnalyzeDatabase(ctx context.Context, path string) (ContentStorageAnalysis, error) {
	if err := registerUnicodeLower(); err != nil {
		return ContentStorageAnalysis{}, err
	}
	uri := databaseFileURL(path)
	params := uri.Query()
	params.Set("mode", "ro")
	uri.RawQuery = params.Encode()
	db, err := sql.Open(databaseDriver, uri.String())
	if err != nil {
		return ContentStorageAnalysis{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	return (&Store{readDB: db}).AnalyzeContent(ctx)
}

// VerifyContentAgainst checks every reconstructed body/metadata row, root and
// semantic chunk against an offline source database before the new copy is used.
func (s *Store) VerifyContentAgainst(ctx context.Context, path string) error {
	conn, err := s.readDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	uri := databaseFileURL(path)
	params := uri.Query()
	params.Set("mode", "ro")
	uri.RawQuery = params.Encode()
	if _, err = conn.ExecContext(ctx, "ATTACH DATABASE ? AS original", uri.String()); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "DETACH DATABASE original")
	checks := []string{
		`SELECT (SELECT count(*) FROM main.files)<>(SELECT count(*) FROM original.files)`,
		`SELECT EXISTS(SELECT 1 FROM original.files o LEFT JOIN main.files f ON f.path=o.path WHERE f.path IS NULL OR f.root IS NOT o.root OR f.name IS NOT o.name OR f.kind IS NOT o.kind OR f.size IS NOT o.size OR f.modified IS NOT o.modified OR f.body IS NOT o.body OR f.placeholder IS NOT o.placeholder OR f.ocr_version IS NOT o.ocr_version OR f.ocr_error IS NOT o.ocr_error)`,
		`SELECT (SELECT count(*) FROM main.chunks)<>(SELECT count(*) FROM original.chunks)`,
		`SELECT EXISTS(SELECT 1 FROM original.chunks o LEFT JOIN main.chunks c ON c.id=o.id WHERE c.id IS NULL OR c.path IS NOT o.path OR c.body IS NOT o.body OR c.start IS NOT o.start OR c.span IS NOT o.span OR c.vector IS NOT o.vector)`,
		`SELECT EXISTS(SELECT path FROM main.roots EXCEPT SELECT path FROM original.roots) OR EXISTS(SELECT path FROM original.roots EXCEPT SELECT path FROM main.roots)`,
		`SELECT EXISTS(SELECT 1 FROM pragma_foreign_key_check)`,
	}
	for i, q := range checks {
		var mismatch bool
		if err = conn.QueryRowContext(ctx, q).Scan(&mismatch); err != nil {
			return err
		}
		if mismatch {
			return fmt.Errorf("shared migration verification failed at check %d", i+1)
		}
	}
	return nil
}

// CompareContentSearch checks literal results and evidence against an offline legacy index.
func (s *Store) CompareContentSearch(ctx context.Context, path string) error {
	original, err := OpenForMaintenance(path)
	if err != nil {
		return err
	}
	defer original.Close()
	for _, q := range []string{"while True", "return nil", "多词查询优先"} {
		started := time.Now()
		before, err := original.Search(ctx, q, "all", nil)
		if err != nil {
			return err
		}
		oldTime := time.Since(started)
		started = time.Now()
		after, err := s.Search(ctx, q, "all", nil)
		if err != nil {
			return err
		}
		newTime := time.Since(started)
		if len(before.Files) != len(after.Files) {
			return fmt.Errorf("search result count changed for %q: %d vs %d", q, len(before.Files), len(after.Files))
		}
		for i, b := range before.Files {
			a := after.Files[i]
			if a.Path != b.Path || a.Score != b.Score || a.Snippet != b.Snippet || a.Line != b.Line || a.SnippetLine != b.SnippetLine {
				return fmt.Errorf("search evidence/ranking changed for %q at result %d", q, i)
			}
		}
		fmt.Printf("Search %q verified (%d results): before=%s after=%s\n", q, len(after.Files), oldTime, newTime)
	}
	return nil
}

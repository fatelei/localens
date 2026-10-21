package catalog

import (
	"context"
	"database/sql"
)

// Chunks reference Unicode character ranges in files.body, rather than duplicate text.
// Unmapped legacy/fallback chunks retain their original text with start=-1.
const chunkBodySQL = "CASE WHEN c.start>=0 THEN substr(f.body,c.start+1,c.span) ELSE c.body END"

func initializeChunkOffsets(db *sql.DB) error {
	var columns int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('chunks') WHERE name='start'").Scan(&columns); err != nil {
		return err
	}
	if columns != 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("ALTER TABLE chunks ADD COLUMN start INTEGER NOT NULL DEFAULT -1; ALTER TABLE chunks ADD COLUMN span INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Process each full body once, rather than repeatedly decoding it for every chunk.
	update, err := tx.Prepare("UPDATE chunks SET start=?,span=?,body='' WHERE id=? AND body=?")
	if err != nil {
		return err
	}
	defer update.Close()
	files, err := tx.Query("SELECT path,body FROM files WHERE body<>''")
	if err != nil {
		return err
	}
	defer files.Close()
	for files.Next() {
		var path, body string
		if err := files.Scan(&path, &body); err != nil {
			return err
		}
		runes := []rune(body)
		rows, err := tx.Query("SELECT id,body FROM chunks WHERE path=? ORDER BY id", path)
		if err != nil {
			return err
		}
		type existing struct {
			id   int64
			body string
		}
		chunks := []existing{}
		for rows.Next() {
			var chunk existing
			if err := rows.Scan(&chunk.id, &chunk.body); err != nil {
				rows.Close()
				return err
			}
			chunks = append(chunks, chunk)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for index, chunk := range chunks {
			start := index * 1050
			if chunk.body == "" || start >= len(runes) {
				continue
			}
			end := min(start+1200, len(runes))
			// Unknown layouts stay verbatim. Only byte-for-byte equal copies are cleared.
			if string(runes[start:end]) != chunk.body {
				continue
			}
			if _, err := update.Exec(start, end-start, chunk.id, chunk.body); err != nil {
				return err
			}
		}
	}
	if err := files.Err(); err != nil {
		return err
	}
	files.Close()
	return tx.Commit()
}

// Compact reclaims obsolete SQLite pages without rebuilding or discarding vectors.
// Call while desktop/background indexing is stopped for predictable maintenance.
func (s *Store) Compact(ctx context.Context) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

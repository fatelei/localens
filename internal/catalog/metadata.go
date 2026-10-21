package catalog

import (
	"context"
	"database/sql"
)

// initializeMetadata atomically builds a small filename index separate from document bodies.
func initializeMetadata(db *sql.DB) error {
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='file_names'").Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
 CREATE TABLE file_names(path TEXT PRIMARY KEY REFERENCES files(path) ON DELETE CASCADE,root TEXT NOT NULL,name TEXT NOT NULL,name_lower TEXT NOT NULL,kind TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL);
 CREATE INDEX file_names_kind ON file_names(kind);
 CREATE INDEX file_names_root ON file_names(root);
 CREATE TRIGGER file_names_insert AFTER INSERT ON files BEGIN
 INSERT INTO file_names VALUES(NEW.path,NEW.root,NEW.name,unicode_lower(NEW.name),NEW.kind,NEW.size,NEW.modified); END;
 CREATE TRIGGER file_names_update AFTER UPDATE OF name,root,kind,size,modified ON files BEGIN
 UPDATE file_names SET root=NEW.root,name=NEW.name,name_lower=unicode_lower(NEW.name),kind=NEW.kind,size=NEW.size,modified=NEW.modified WHERE path=NEW.path; END;
 INSERT INTO file_names SELECT path,root,name,unicode_lower(name),kind,size,modified FROM files;
 CREATE INDEX IF NOT EXISTS chunks_have_vector ON chunks((vector IS NOT NULL));`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SearchNames provides the fast, default filename lookup without touching document bodies.
func (s *Store) SearchNames(ctx context.Context, query, kind string) (SearchResult, error) {
	return s.search(ctx, query, kind, nil, true, false)
}

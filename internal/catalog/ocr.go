package catalog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"unicode/utf8"
)

func initializeOCR(db *sql.DB) error {
	var columns int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('files') WHERE name='ocr_version'").Scan(&columns); err != nil {
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
	_, err = tx.Exec(`ALTER TABLE files ADD COLUMN ocr_version INTEGER NOT NULL DEFAULT 0;
 ALTER TABLE files ADD COLUMN ocr_error TEXT NOT NULL DEFAULT '';
 CREATE INDEX files_ocr_pending ON files(kind,ocr_version,placeholder,modified DESC);
 CREATE TRIGGER files_ocr_changed AFTER UPDATE OF size,modified,placeholder ON files WHEN OLD.size<>NEW.size OR OLD.modified<>NEW.modified OR OLD.placeholder<>NEW.placeholder BEGIN UPDATE files SET ocr_version=0,ocr_error='' WHERE path=NEW.path; END;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) indexImageText(ctx context.Context) (bool, error) {
	if !OCRAvailable() {
		return false, nil
	}
	return s.processImageText(ctx, recognizeImage)
}
func (s *Store) processImageText(ctx context.Context, recognize func(context.Context, string) (string, error)) (bool, error) {
	var path string
	var size, modified int64
	err := s.readDB.QueryRowContext(ctx, "SELECT path,size,modified FROM files WHERE kind='image' AND ocr_version=0 AND placeholder=0 ORDER BY modified DESC LIMIT 1").Scan(&path, &size, &modified)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, verifyErr := s.VerifiedPath(ctx, path)
	info, err := os.Stat(path)
	text := ""
	message := ""
	if err != nil || verifyErr != nil {
		message = "Image is not readable or no longer within its indexed folder"
	} else if isPlaceholder(info) {
		message = "Image is not available locally"
	} else if info.Size() != size || info.ModTime().UnixNano() != modified {
		message = "Image changed; waiting for rescan"
	} else if size > 32*1024*1024 {
		message = "Image exceeds the OCR size limit"
	} else {
		text, err = recognize(ctx, path)
		if err != nil {
			message = "Offline text recognition failed"
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	text = strings.TrimSpace(text)
	if len(text) > maxBodyBytes {
		text = text[:maxBodyBytes]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	// Check metadata and indexed scope again after recognition.
	_, verifyErr = s.VerifiedPath(ctx, path)
	if latest, e := os.Stat(path); e != nil || verifyErr != nil || latest.Size() != size || latest.ModTime().UnixNano() != modified || isPlaceholder(latest) {
		text = ""
		message = "Image changed or became unavailable; waiting for rescan"
	}
	_, err = s.db.ExecContext(ctx, "UPDATE files SET body=?,ocr_version=1,ocr_error=? WHERE path=? AND size=? AND modified=? AND placeholder=0 AND ocr_version=0", text, message, path, size, modified)
	return true, err
}

// RetryImageOCR retries local image recognition without changing existing vectors.
func (s *Store) RetryImageOCR(ctx context.Context) error {
	if !OCRAvailable() {
		return errors.New("image OCR is unavailable on this platform")
	}
	_, err := s.db.ExecContext(ctx, "UPDATE files SET ocr_version=0,ocr_error='' WHERE kind='image' AND placeholder=0")
	return err
}

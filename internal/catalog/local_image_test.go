package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedLocalImageRejectsUnindexedChangedAndDeletedFiles(t *testing.T) {
	s, root := fixture(t)
	path := filepath.Join(root, "photo.png")
	put(t, path, "image fixture")
	if _, err := s.VerifiedLocalImage(t.Context(), path); err == nil {
		t.Fatal("accepted unindexed path")
	}
	scan(t, s)
	if _, err := s.VerifiedLocalImage(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(root, "notes.txt")
	put(t, text, "text")
	scan(t, s)
	if _, err := s.VerifiedLocalImage(t.Context(), text); err == nil {
		t.Fatal("accepted text")
	}
	put(t, path, "changed image fixture")
	if _, err := s.VerifiedLocalImage(t.Context(), path); err == nil {
		t.Fatal("accepted stale image")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifiedLocalImage(t.Context(), path); err == nil {
		t.Fatal("accepted deleted image")
	}
}

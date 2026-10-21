package catalog

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const maxBodyBytes = 512 * 1024

// Kind classifies extensions for filters and extraction.
func Kind(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp":
		return "image"
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v":
		return "video"
	case ".mp3", ".wav", ".flac", ".m4a", ".ogg":
		return "audio"
	case ".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml", ".toml", ".log", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".html", ".css", ".rs", ".java", ".c", ".cpp", ".h", ".sql", ".pdf", ".docx":
		return "document"
	default:
		return "other"
	}
}
func extract(ctx context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var text string
	switch ext {
	case ".docx":
		z, err := zip.OpenReader(path)
		if err != nil {
			return "", err
		}
		defer z.Close()
		for _, f := range z.File {
			if f.Name != "word/document.xml" {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return "", err
			}
			defer r.Close()
			d := xml.NewDecoder(io.LimitReader(r, 4*maxBodyBytes))
			var b strings.Builder
			for b.Len() < maxBodyBytes {
				t, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					return "", err
				}
				switch x := t.(type) {
				case xml.CharData:
					b.Write(x)
				case xml.EndElement:
					if x.Name.Local == "p" {
						b.WriteByte('\n')
					}
				}
			}
			text = b.String()
			break
		}
	case ".pdf":
		if _, err := exec.LookPath("pdftotext"); err != nil {
			return "", err
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cctx, "pdftotext", "-f", "1", "-l", "100", "-enc", "UTF-8", path, "-")
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return "", err
		}
		if err = cmd.Start(); err != nil {
			return "", err
		}
		b, readErr := io.ReadAll(io.LimitReader(pipe, maxBodyBytes+1))
		if len(b) > maxBodyBytes {
			_ = cmd.Process.Kill()
		}
		waitErr := cmd.Wait()
		if readErr != nil {
			return "", readErr
		}
		if waitErr != nil && len(b) <= maxBodyBytes {
			return "", waitErr
		}
		text = string(b)
	default:
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, maxBodyBytes))
		if err != nil {
			return "", err
		}
		if !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
			return "", nil
		}
		text = string(b)
	}
	return strings.ToValidUTF8(text, ""), nil
}
func chunkText(body string) []string {
	r := []rune(body)
	if len(r) == 0 {
		return nil
	}
	out := []string{}
	for start := 0; start < len(r) && len(out) < 96; start += 1050 {
		end := min(start+1200, len(r))
		out = append(out, string(r[start:end]))
		if end == len(r) {
			break
		}
	}
	return out
}

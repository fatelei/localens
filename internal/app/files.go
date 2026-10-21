package app

import (
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GetPreview reads only a verified indexed file with strict preview size limits.
func (a *App) GetPreview(path string) (Preview, error) {
	if err := a.ready(); err != nil {
		return Preview{}, err
	}
	f, err := a.store.VerifiedPath(a.ctx, path)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{File: f}
	if f.Kind == "image" && f.Size <= 12*1024*1024 {
		file, err := os.Open(f.Path)
		if err != nil {
			return p, err
		}
		defer file.Close()
		b, err := io.ReadAll(io.LimitReader(file, 12*1024*1024+1))
		if err != nil {
			return p, err
		}
		if len(b) > 12*1024*1024 {
			return p, errors.New("图片超过预览大小限制")
		}
		mime := map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp"}[strings.ToLower(filepath.Ext(path))]
		if mime != "" {
			p.Image = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
		}
	} else if f.Kind == "document" {
		p.Text, err = a.store.Body(a.ctx, path)
		r := []rune(p.Text)
		if len(r) > 6000 {
			p.Text = string(r[:6000]) + "\n…"
		}
	}
	return p, err
}

// OpenFile opens an indexed file using the operating system's default handler.
func (a *App) OpenFile(path string) error { return a.open(path, false) }

// RevealFile reveals a file in Finder, Explorer, or its containing folder.
func (a *App) RevealFile(path string) error { return a.open(path, true) }
func (a *App) open(path string, reveal bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	f, err := a.store.VerifiedPath(a.ctx, path)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			cmd = exec.Command("open", "-R", f.Path)
		} else {
			cmd = exec.Command("open", f.Path)
		}
	case "windows":
		if reveal {
			cmd = exec.Command("explorer.exe", "/select,"+f.Path)
		} else {
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", f.Path)
		}
	default:
		path = f.Path
		if reveal {
			path = filepath.Dir(path)
		}
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Run()
}

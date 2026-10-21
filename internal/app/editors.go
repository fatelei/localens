package app

import (
	"errors"
	"fmt"
	"io"
	"localens/internal/catalog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

type Editor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type editorSpec struct {
	Editor
	command, app string
}

var editorSpecs = []editorSpec{
	{Editor{"code", "Visual Studio Code"}, "code", "Visual Studio Code.app/Contents/Resources/app/bin/code"},
	{Editor{"cursor", "Cursor"}, "cursor", "Cursor.app/Contents/Resources/app/bin/cursor"},
	{Editor{"zed", "Zed"}, "zed", "Zed.app/Contents/MacOS/cli"},
	{Editor{"subl", "Sublime Text"}, "subl", "Sublime Text.app/Contents/SharedSupport/bin/subl"},
	{Editor{"goland", "GoLand"}, "goland", "GoLand.app/Contents/MacOS/goland"},
	{Editor{"idea", "IntelliJ IDEA"}, "idea", "IntelliJ IDEA.app/Contents/MacOS/idea"},
}

func editorExecutable(id string) (string, error) {
	for _, spec := range editorSpecs {
		if spec.ID != id {
			continue
		}
		if path, err := exec.LookPath(spec.command); err == nil {
			return path, nil
		}
		if runtime.GOOS == "darwin" {
			home, _ := os.UserHomeDir()
			for _, root := range []string{"/Applications", filepath.Join(home, "Applications")} {
				path := filepath.Join(root, spec.app)
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
					return path, nil
				}
			}
		}
		return "", fmt.Errorf("editor unavailable: %s", spec.Name)
	}
	return "", errors.New("unsupported editor")
}

// GetEditors lists installed editors with known line-navigation commands.
func (a *App) GetEditors() []Editor {
	editors := []Editor{}
	for _, spec := range editorSpecs {
		if _, err := editorExecutable(spec.ID); err == nil {
			editors = append(editors, spec.Editor)
		}
	}
	return editors
}
func (a *App) SetEditor(id string) error {
	if id != "" {
		if _, err := editorExecutable(id); err != nil {
			return err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.preferences.Editor
	a.preferences.Editor = id
	if err := a.savePreferencesLocked(); err != nil {
		a.preferences.Editor = old
		return err
	}
	return nil
}
func editorArguments(id, path string, line int) []string {
	if line < 1 {
		return []string{path}
	}
	switch id {
	case "code", "cursor":
		return []string{"--goto", path + ":" + strconv.Itoa(line) + ":1"}
	case "goland", "idea":
		return []string{"--line", strconv.Itoa(line), path}
	default:
		return []string{path + ":" + strconv.Itoa(line)}
	}
}

// OpenCode uses the saved editor and recalculates the location from current file contents.
func (a *App) OpenCode(path, query, evidence string) error {
	if err := a.ready(); err != nil {
		return err
	}
	f, err := a.store.VerifiedPath(a.ctx, path)
	if err != nil {
		return err
	}
	a.mu.Lock()
	id := a.preferences.Editor
	a.mu.Unlock()
	if id == "" || !catalog.IsCode(path) {
		return a.OpenFile(path)
	}
	executable, err := editorExecutable(id)
	if err != nil {
		return err
	}
	line := 0
	// Current contents keep navigation correct after edits; bound reads to indexed extraction size.
	if f.Size <= 2*1024*1024 {
		file, err := os.Open(f.Path)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
		if err != nil {
			return err
		}
		if len(body) <= 2*1024*1024 {
			_, _, line = catalog.CodeExcerpt(string(body), query, evidence)
		}
	}
	cmd := exec.Command(executable, editorArguments(id, f.Path, line)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// A newly launched IDE may remain alive; release the launcher immediately and reap it later.
	go func() { _ = cmd.Wait() }()
	return nil
}

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type InitResult struct {
	Path    string
	Written bool
	Exists  bool
}

func EnsureFile(example []byte) (InitResult, error) {
	for _, p := range []string{Path(), legacyPath()} {
		if _, err := os.Stat(p); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return InitResult{Path: p, Exists: true}, nil
		}
	}
	if err := writeNew(Path(), example); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return InitResult{Path: Path(), Exists: true}, nil
		}
		return InitResult{}, err
	}
	return InitResult{Path: Path(), Written: true}, nil
}

func InitFile(example []byte, force bool) (InitResult, error) {
	r, err := EnsureFile(example)
	if err != nil || r.Written || !force {
		return r, err
	}
	next := Path() + ".new"
	if err := os.MkdirAll(filepath.Dir(next), 0o755); err != nil {
		return InitResult{}, err
	}
	if err := os.WriteFile(next, example, 0o644); err != nil {
		return InitResult{}, err
	}
	return InitResult{Path: next, Written: true, Exists: true}, nil
}

func writeNew(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

func Tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !filepath.IsAbs(rel) && (len(rel) < 3 || rel[:3] != "../") {
		return filepath.Join("~", rel)
	}
	return path
}

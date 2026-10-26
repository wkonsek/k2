package config

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/k2"
	}
	return filepath.Join(home, ".k2")
}

func Path() string { return filepath.Join(DefaultDir(), "config.yaml") }

func FilePath() string {
	if _, err := os.Stat(Path()); err == nil {
		return Path()
	}
	if _, err := os.Stat(legacyPath()); err == nil {
		return legacyPath()
	}
	return Path()
}

func legacyPath() string { return filepath.Join(DefaultDir(), "config") }

func HomePath(p string) string {
	if p == "" || p == "no kubeconfig" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

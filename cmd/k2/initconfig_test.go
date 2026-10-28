package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2"
	"k2/internal/config"
)

func TestEmbeddedExampleIsTheRepoFile(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, k2.ConfigExample) {
		t.Fatal("embedded example differs from config.example.yaml")
	}

	if c, w := config.Parse(k2.ConfigExample); len(w) != 0 || c.Theme != config.Default().Theme {
		t.Fatalf("example is not inert: %v", w)
	}
}

func TestFirstStartWritesExampleOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".k2", "config.yaml")
	notice, warn := firstStart()
	if warn != "" || notice != "created ~/.k2/config.yaml (all options commented)" {
		t.Fatalf("notice %q warn %q", notice, warn)
	}
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, k2.ConfigExample) {
		t.Fatalf("not written: %v", err)
	}
	_ = os.WriteFile(path, []byte("theme: dracula\n"), 0o644)
	if notice, warn := firstStart(); notice != "" || warn != "" {
		t.Fatalf("second start: %q %q", notice, warn)
	}
	if b, _ := os.ReadFile(path); string(b) != "theme: dracula\n" {
		t.Fatalf("overwritten: %q", b)
	}
}

func TestFirstStartRespectsLegacyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".k2"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".k2", "config"), []byte("theme: gruvbox\n"), 0o644)
	if notice, _ := firstStart(); notice != "" {
		t.Fatalf("notice %q", notice)
	}
	if _, err := os.Stat(filepath.Join(home, ".k2", "config.yaml")); err == nil {
		t.Fatal("config.yaml written beside the legacy config")
	}
}

func TestFirstStartUnwritableHome(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	home := t.TempDir()
	_ = os.Chmod(home, 0o500)
	defer os.Chmod(home, 0o700)
	t.Setenv("HOME", home)
	if notice, warn := firstStart(); notice != "" || !strings.Contains(warn, "could not create ~/.k2/config.yaml") {
		t.Fatalf("notice %q warn %q", notice, warn)
	}
}

func TestInitConfigFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".k2", "config.yaml")
	out, exit := cliAction([]string{"--init-config"}, "v")
	if !exit || !strings.Contains(out, "wrote ~/.k2/config.yaml") {
		t.Fatalf("first: %q", out)
	}
	_ = os.WriteFile(path, []byte("theme: nord\n"), 0o644)
	out, _ = cliAction([]string{"--init-config"}, "v")
	if !strings.Contains(out, "already exists, left unchanged") {
		t.Fatalf("second: %q", out)
	}
	out, _ = cliAction([]string{"--init-config", "--force"}, "v")
	if !strings.Contains(out, "wrote the example beside it: ~/.k2/config.yaml.new") {
		t.Fatalf("force: %q", out)
	}
	if b, _ := os.ReadFile(path); string(b) != "theme: nord\n" {
		t.Fatalf("config overwritten: %q", b)
	}
	if b, _ := os.ReadFile(path + ".new"); !bytes.Equal(b, k2.ConfigExample) {
		t.Fatal("config.yaml.new is not the example")
	}
	if !strings.Contains(usage, "--init-config") || !strings.Contains(usage, "--force") || !strings.Contains(usage, "--check-config") {
		t.Fatal("--help does not mention --init-config")
	}
}

func TestCheckConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	out, code := checkConfig()
	if code != 0 || !strings.Contains(out, "does not exist") {
		t.Fatalf("missing: %d %q", code, out)
	}
	dir := filepath.Join(home, ".k2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("theme: nope\nui:\n  sparkles: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = checkConfig()
	if code != 1 || !strings.Contains(out, "unknown theme") || !strings.Contains(out, "sparkles") || !strings.Contains(out, "2 problems") {
		t.Fatalf("bad: %d %q", code, out)
	}
	if _, exit := cliAction([]string{"--check"}, "v"); !exit {
		t.Fatal("--check must not start the UI")
	}
	if err := os.WriteFile(path, []byte("theme: nord\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = checkConfig()
	if code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("ok: %d %q", code, out)
	}
}

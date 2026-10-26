package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertContextTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kc := filepath.Join(home, ".kube", "config")

	next, err := UpsertContextTheme(nil, "kind-dev", kc, "Dracula")
	if err != nil || len(next) != 1 || next[0].Context != "kind-dev" || next[0].Kubeconfig != "~/.kube/config" || next[0].Theme != "dracula" {
		t.Fatalf("new %+v %v", next, err)
	}

	wide := []ContextTheme{{Context: "kind-dev", Theme: "nord"}, {Context: "*-dev*", Theme: "matrix"}}
	next, err = UpsertContextTheme(wide, "kind-dev", kc, "gruvbox")
	if err != nil || len(next) != 2 || next[0].Kubeconfig != "" || next[0].Theme != "gruvbox" || next[1].Context != "*-dev*" {
		t.Fatalf("wide %+v %v", next, err)
	}

	other := []ContextTheme{{Context: "kind-dev", Kubeconfig: "~/.kube/other", Theme: "nord"}, {Context: "*-prod*", Theme: "dracula"}}
	next, err = UpsertContextTheme(other, "kind-dev", kc, "matrix")
	if err != nil || len(next) != 3 || next[0].Kubeconfig != "~/.kube/config" || next[0].Theme != "matrix" || next[1].Kubeconfig != "~/.kube/other" {
		t.Fatalf("other file %+v %v", next, err)
	}
	if got := (Config{ContextThemes: next, Theme: "nord"}).ThemeFor(kc, "kind-dev"); got != "matrix" {
		t.Fatalf("ThemeFor %v", next)
	}

	next, err = UpsertContextTheme(next, "kind-dev", kc, "catppuccin")
	if err != nil || len(next) != 3 || next[0].Theme != "catppuccin" || next[0].Kubeconfig != "~/.kube/config" {
		t.Fatalf("update %+v %v", next, err)
	}
	if _, err := UpsertContextTheme(nil, "", "", "nord"); err == nil {
		t.Fatal("empty context accepted")
	}
	if _, err := UpsertContextTheme(nil, "kind-dev", kc, "sparkle"); err == nil {
		t.Fatal("unknown theme accepted")
	}
}

func TestWriteContextThemeKeepsComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".k2")
	path := filepath.Join(dir, "config.yaml")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	in := "# keep this\ntheme: nord\n# still here\nui:\n  # first match wins\n  background: transparent\n  context_themes:\n    - {context: \"*-prod*\", theme: dracula}\n    - {context: kind-dev, kubeconfig: ~/.kube/other, theme: nord}\nkeys:\n  quit: [Q, ctrl+c]\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	kc := filepath.Join(home, ".kube", "config")
	entries := []ContextTheme{{Context: "*-prod*", Theme: "dracula"}, {Context: "kind-dev", Kubeconfig: "~/.kube/other", Theme: "nord"}}
	next, err := WriteContextTheme(path, entries, "kind-dev", kc, "matrix")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"# keep this", "# still here", "theme: nord", "background: transparent", "*-prod*", "~/.kube/other", "~/.kube/config", "matrix", "quit:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	c, w := Parse(b)
	if len(w) != 0 || c.Theme != "nord" || c.UI.Background != "transparent" || !c.Keys.Quit.Has("Q") {
		t.Fatalf("parsed theme %s bg %s quit %v warnings %v", c.Theme, c.UI.Background, c.Keys.Quit, w)
	}
	if got := c.ThemeFor(kc, "kind-dev"); got != "matrix" {
		t.Fatalf("ThemeFor %s entries %+v", got, c.ContextThemes)
	}
	if len(next) != len(c.ContextThemes) {
		t.Fatalf("memory %+v file %+v", next, c.ContextThemes)
	}

	commented := []byte("# ~/.k2/config.yaml\n# theme: nord\n# ui:\n#   background: transparent\n")
	if err := os.WriteFile(path, commented, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := WriteContextTheme(path, nil, "lab", kc, "gruvbox")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "# theme: nord") || !strings.Contains(string(b), "#   background: transparent") || !strings.Contains(string(b), "context: lab") || !strings.Contains(string(b), "theme: gruvbox") {
		t.Fatalf("commented file:\n%s", b)
	}
	if len(out) != 1 || out[0].Theme != "gruvbox" {
		t.Fatalf("out %+v", out)
	}
}

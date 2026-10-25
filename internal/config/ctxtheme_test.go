package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlob(t *testing.T) {
	for _, c := range []struct {
		pat, s string
		want   bool
	}{
		{"kind-kindest", "kind-kindest", true},
		{"kind-*", "kind-kindest", true},
		{"*-prod*", "shop-prod-eu", true},
		{"*-prod*", "prod-eu", false},
		{"*", "default/api-demo:6443/me", true},
		{"*/api-demo:6443/*", "shop/api-demo:6443/kube:admin", true},
		{"dev-?", "dev-1", true},
		{"dev-?", "dev-12", false},
		{"a.b", "aXb", false},
		{"Kind-*", "kind-x", false},
	} {
		if got := Glob(c.pat, c.s); got != c.want {
			t.Errorf("Glob(%q, %q) = %v", c.pat, c.s, got)
		}
	}
}

func TestContextThemes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, w := Parse([]byte(`
theme: gruvbox
ui:
  context_themes:
    - {context: kind-kindest, theme: matrix}
    - context: "*-prod*"
      theme: Dracula
    - {kubeconfig: ~/.kube/lab.yaml, theme: nord}
    - {kubeconfig: "/clusters/*.yaml", context: "ops-*", theme: kanagawa}
`))
	if len(w) != 0 {
		t.Fatalf("warnings: %v", w)
	}
	lab := filepath.Join(home, ".kube", "lab.yaml")
	for _, k := range []struct{ kc, ctx, want string }{
		{"/x/config", "kind-kindest", "matrix"},
		{"/x/config", "shop-prod-1", "dracula"},
		{lab, "anything", "nord"},
		{"~/.kube/lab.yaml", "anything", "nord"},
		{lab, "kind-kindest", "matrix"},
		{"/clusters/a.yaml", "ops-1", "kanagawa"},
		{"/clusters/a.yaml", "dev-1", "gruvbox"},
		{"/x/config", "dev", "gruvbox"},
		{"", "dev", "gruvbox"},
	} {
		if got := c.ThemeFor(k.kc, k.ctx); got != k.want {
			t.Errorf("ThemeFor(%q, %q) = %s, want %s", k.kc, k.ctx, got, k.want)
		}
	}
}

func TestContextThemesInvalid(t *testing.T) {
	c, w := Parse([]byte(`
ui:
  context_themes:
    - {context: a, theme: sparkle}
    - {context: b}
    - {theme: nord}
    - {context: c, theme: nord, colour: red}
    - just-a-string
    - {context: d, theme: matrix}
`))
	if len(c.ContextThemes) != 1 || c.ContextThemes[0].Context != "d" {
		t.Fatalf("kept %+v", c.ContextThemes)
	}
	want := []string{
		`config: ui.context_themes[1]: unknown theme "sparkle" (entry ignored; have nord, dracula`,
		"config: ui.context_themes[2]: no theme",
		"config: ui.context_themes[3]: needs a context or a kubeconfig pattern",
		`config: ui.context_themes[4]: unknown field "colour"`,
		"config: ui.context_themes[5]: expected {context: …, theme: …}",
	}
	if len(w) != len(want) {
		t.Fatalf("warnings:\n%s", strings.Join(w, "\n"))
	}
	for i, p := range want {
		if !strings.HasPrefix(w[i], p) {
			t.Errorf("warning %d: %q, want prefix %q", i, w[i], p)
		}
	}
	if _, w := Parse([]byte("ui: {context_themes: nord}")); len(w) != 1 || !strings.Contains(w[0], "expected a list") {
		t.Errorf("scalar: %v", w)
	}
	if c, w := Parse([]byte("ui:\n  context_themes:\n")); len(w) != 0 || c.ContextThemes != nil {
		t.Errorf("empty: %v %v", w, c.ContextThemes)
	}
}

func TestContextThemesExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	in := false
	for _, l := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(l, "#   context_themes:"):
			in = true
			lines = append(lines, "ui:", "  context_themes:")
		case in && strings.HasPrefix(l, "#     - "):
			lines = append(lines, strings.TrimPrefix(l, "# "))
		case in && strings.HasPrefix(l, "#    "):
		default:
			in = false
		}
	}
	c, w := Parse([]byte(strings.Join(lines, "\n")))
	if len(w) != 0 || len(c.ContextThemes) != 3 || c.ThemeFor("", "kind-kindest") != "matrix" {
		t.Fatalf("example: %v %+v\n%s", w, c.ContextThemes, strings.Join(lines, "\n"))
	}
}

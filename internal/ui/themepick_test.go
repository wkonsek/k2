package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/config"
	"k2/internal/theme"
)

func TestThemePopupSavesContext(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig = "/kc"
	m.context = "kind-dev"
	m = pressSettle(t, m, "ctrl+t")
	if m.dlg == nil || m.dlg.act != "theme" || len(m.dlg.options) != len(theme.Names()) {
		t.Fatalf("dialog %+v", m.dlg)
	}
	if m.dlg.picked() != "nord" || len(m.dlg.notes) != 0 {
		t.Fatalf("cursor %q notes %v", m.dlg.picked(), m.dlg.notes)
	}
	if strings.Contains(ansi.Strip(m.View()), "arctic") {
		t.Fatal("theme popup still shows descriptions")
	}
	for m.dlg.picked() != "dracula" {
		m = press(t, m, "down")
	}
	m = pressSettle(t, m, "enter")
	if m.dlg != nil || m.themeName != "dracula" || m.cfg.ThemeFor("/kc", "kind-dev") != "dracula" {
		t.Fatalf("theme %s for %s dlg %v", m.themeName, m.cfg.ThemeFor("/kc", "kind-dev"), m.dlg != nil)
	}
	b, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "kind-dev") || !strings.Contains(string(b), "dracula") {
		t.Fatalf("config:\n%s", b)
	}
	c, w := config.Parse(b)
	if len(w) != 0 || c.ThemeFor("/kc", "kind-dev") != "dracula" {
		t.Fatalf("parsed %+v %v", c.ContextThemes, w)
	}

	m = pressSettle(t, m, "ctrl+t", "enter")
	if len(m.cfg.ContextThemes) != 1 {
		t.Fatalf("duplicate rules %+v", m.cfg.ContextThemes)
	}
	b, _ = os.ReadFile(config.Path())
	if strings.Count(string(b), "kind-dev") != 1 {
		t.Fatalf("file duplicated:\n%s", b)
	}

	m.context = ""
	m = pressSettle(t, m, "ctrl+t")
	if m.dlg != nil || !strings.Contains(m.status, "context") {
		t.Fatalf("no context: dlg %v status %q", m.dlg != nil, m.status)
	}
}

func TestThemePopupPreviews(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = pressSettle(t, m, "ctrl+t", "down")
	if m.themeName != "dracula" || th.Name != "dracula" {
		t.Fatalf("no preview: %s %s", m.themeName, th.Name)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg != nil || m.themeName != "nord" || th.Name != "nord" || len(m.cfg.ContextThemes) != 0 {
		t.Fatalf("not restored: %s %s %+v", m.themeName, th.Name, m.cfg.ContextThemes)
	}
}

func TestThemeCommand(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.context = "kind-dev"
	m = open(t, m, "theme")
	if m.dlg == nil || m.dlg.act != "theme" {
		t.Fatalf("dlg %+v status %q", m.dlg, m.status)
	}
}

func TestThemePopupScrollsInAShortPane(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.context = "kind-dev"
	m.height = 14
	m = press(t, m, "ctrl+t")
	names := theme.Names()
	box := ansi.Strip(m.dialogBox(40))
	if strings.Contains(box, "arctic") || strings.Contains(box, names[len(names)-1]) || !strings.Contains(box, names[0]) {
		t.Fatalf("box:\n%s", box)
	}
	m.dlg.idx = len(names) - 1
	box = ansi.Strip(m.dialogBox(40))
	if !strings.Contains(box, names[len(names)-1]) || strings.Contains(box, names[0]) {
		t.Fatalf("scrolled box:\n%s", box)
	}
}

package ui

import (
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

type noopMsg struct{}

func TestContextThemeLiveSwitch(t *testing.T) {
	cfg := testEnv(t)
	cfg.Theme = "gruvbox"
	cfg.ContextThemes = []config.ContextTheme{{Context: "kind-*", Theme: "matrix"}, {Kubeconfig: "/lab/*", Theme: "dracula"}}
	m := newTestModel(t, cfg, nil)
	t.Cleanup(func() { useTheme(theme.Nord()) })
	step := func() {
		next, _ := m.Update(noopMsg{})
		m = next.(model)
	}
	step()
	if th.Name != "gruvbox" || m.themeName != "gruvbox" {
		t.Fatalf("start: %s", th.Name)
	}
	m.context = "kind-kindest"
	step()
	if th.Name != "matrix" {
		t.Fatalf("kind context: %s", th.Name)
	}
	matrix, _ := theme.Lookup("matrix")
	if !strings.Contains(m.View(), stylePrefix(titleStyle)) || titleStyle.GetForeground() != matrix.Styles().Title.GetForeground() {
		t.Fatal("styles not rebuilt for matrix")
	}
	if m.cmd.PromptStyle.GetForeground() != keyStyle.GetForeground() {
		t.Fatal("inputs keep the old colors")
	}
	m.context, m.kubeconfig = "dev", "/lab/a.yaml"
	step()
	if th.Name != "dracula" {
		t.Fatalf("kubeconfig match: %s", th.Name)
	}
	m.kubeconfig = "/home/me/.kube/config"
	step()
	if th.Name != "gruvbox" {
		t.Fatalf("back to the global theme: %s", th.Name)
	}

	m.commit()
	other := m.snap()
	other.context = "kind-kindest"
	other.logID = newPaneID()
	m.panes = append(m.panes, other)
	m.focusPane(1)
	step()
	if th.Name != "matrix" {
		t.Fatalf("focused pane on kind: %s", th.Name)
	}
	m.focusPane(0)
	step()
	if th.Name != "gruvbox" {
		t.Fatalf("focus back: %s", th.Name)
	}
	var _ tea.Model = m
}

func TestContextThemeAtStart(t *testing.T) {
	cfg := testEnv(t)
	cfg.ContextThemes = []config.ContextTheme{{Context: "kind-*", Theme: "matrix"}}
	t.Setenv("KUBECONFIG", "")
	m := newTestModel(t, cfg, nil)
	t.Cleanup(func() { useTheme(theme.Nord()) })
	m.context = "kind-a"
	m.syncTheme()
	if th.Name != "matrix" || m.themeName != "matrix" {
		t.Fatalf("%s", th.Name)
	}
	m.syncTheme()
	if th.Name != "matrix" {
		t.Fatal("unstable")
	}
}

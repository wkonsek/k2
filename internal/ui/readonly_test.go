package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestReadOnlyToggleAndPadlock(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	if m.readOnly {
		t.Fatal("starts off")
	}
	next, _ := m.toggleReadOnly()
	m = next.(model)
	if !m.readOnly {
		t.Fatal("should be on")
	}
	lines := sidebarPlain(m, 26, 60)
	joined := strings.Join(lines[:3], "\n")
	if !strings.Contains(ansi.Strip(joined), strings.TrimRight(padlock[0], " ")) && !strings.Contains(joined, "▐") {
		t.Fatalf("padlock missing:\n%s", joined)
	}
	next, _ = m.toggleReadOnly()
	m = next.(model)
	if m.readOnly || strings.Contains(ansi.Strip(m.sidebarBody(26, 60)), "▐██") {
		t.Fatal("padlock should hide when off")
	}
}

func TestReadOnlyBlocksMutations(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	next, _ := m.openList("workpods", "pods")
	m = next.(model)
	m.readOnly = true
	if next, _, blocked := m.readOnlyBlock("delete"); !blocked {
		t.Fatal("expected block")
	} else if !strings.Contains(next.(model).status, "view-only") {
		t.Fatalf("status %q", next.(model).status)
	}
	m.readOnly = true
	got, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_ = cmd
	gm := got.(model)
	if gm.ed != nil && gm.ed.live() {
		t.Fatal("shell must not start in view-only")
	}
}

func TestReadOnlyMountainStripKeepsPadlock(t *testing.T) {
	m := mountainModel(t, "bottom")
	m.readOnly = true
	lines := strings.Split(ansi.Strip(m.stripBody(130, stripRows)), "\n")
	if !strings.Contains(lines[0], "▄") || !strings.Contains(lines[0]+lines[1], "▐") && !strings.Contains(lines[1], "█") {
		t.Fatalf("strip:\n%s", strings.Join(lines, "\n"))
	}
	if len(lines) > stripRows {
		t.Fatalf("strip grew to %d rows", len(lines))
	}
}

func TestKustomizeKeysDefaults(t *testing.T) {
	k := newTestModel(t, testEnv(t), nil).cfg.Keys
	if !k.Apply.Has("A") || k.Apply.Has("a") {
		t.Fatalf("apply %v", k.Apply)
	}
	if !k.KustomizeDelete.Has("ctrl+d") || k.KustomizeDelete.Has("d") {
		t.Fatalf("kust delete %v", k.KustomizeDelete)
	}
}

func TestReadOnlyBlocksApplyAndDeleteKeys(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	_ = m.showKList("/k", []string{"/k/overlays/dev"})
	m.readOnly = true
	if got := press(t, m, "A"); got.asking() != "" || !strings.Contains(got.status, "view-only") {
		t.Fatalf("apply: ask %q status %q", got.asking(), got.status)
	}
	if got := press(t, m, "ctrl+d"); got.asking() != "" || !strings.Contains(got.status, "view-only") {
		t.Fatalf("delete: ask %q status %q", got.asking(), got.status)
	}
	m.readOnly = false
	if got := press(t, m, "A"); got.asking() == "" {
		t.Fatal("apply should work again")
	}
}

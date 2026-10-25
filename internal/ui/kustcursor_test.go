package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func selected(m model) string {
	if sel := m.content.SelectedItem(); sel != nil {
		return sel.(item).title
	}
	return ""
}

func TestKustomizeBackSelectsDirCameFrom(t *testing.T) {
	root := kustTree(t)
	if err := os.MkdirAll(filepath.Join(root, "docs", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := browseModel(t, root)
	selectTitle(t, &m, "docs")
	m = press(t, m, "enter")
	if m.kroot != filepath.Join(root, "docs") {
		t.Fatalf("kroot %q", m.kroot)
	}
	selectTitle(t, &m, "deep")
	m = press(t, m, "enter", "esc")
	if got := selected(m); got != "deep" {
		t.Fatalf("back from deep selects %q", got)
	}
	m = press(t, m, "esc")
	if m.kroot != root || selected(m) != "docs" {
		t.Fatalf("back from docs: kroot %q selected %q", m.kroot, selected(m))
	}

	m = press(t, m, "enter")
	if selected(m) != "deep" {
		t.Fatalf("re-entering docs selects %q", selected(m))
	}
}

func TestKustomizeActionsKeepCursor(t *testing.T) {
	root := kustTree(t)
	m := browseModel(t, root)
	selectTitle(t, &m, "base")
	m = press(t, m, "D")
	if m.stage != "kout" {
		t.Fatalf("D: stage %q", m.stage)
	}
	m = press(t, m, "esc")
	if m.stage != "kdir" || selected(m) != "base" {
		t.Fatalf("after diff: stage %q selected %q", m.stage, selected(m))
	}

	m = press(t, m, "a", "y")
	if m.stage != "kout" {
		t.Fatalf("apply: stage %q", m.stage)
	}
	m = press(t, m, "esc")
	if selected(m) != "base" {
		t.Fatalf("after apply: selected %q", selected(m))
	}

	old := vimCommand
	vimCommand = func(string) *exec.Cmd { return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })
	selectTitle(t, &m, "deploy.yaml")
	m = press(t, m, "e")
	next, _ := m.Update(lessDoneMsg{})
	m = next.(model)
	if selected(m) != "deploy.yaml" {
		t.Fatalf("after edit: selected %q", selected(m))
	}

	if err := os.WriteFile(filepath.Join(root, "aaa.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = m.showKDir(root)
	if selected(m) != "deploy.yaml" {
		t.Fatalf("after refresh: selected %q", selected(m))
	}
}

func TestKustomizeCursorFallsBackToIndex(t *testing.T) {
	root := kustTree(t)
	m := browseModel(t, root)
	selectTitle(t, &m, "deploy.yaml")
	last := m.content.Index()
	if err := os.Remove(filepath.Join(root, "deploy.yaml")); err != nil {
		t.Fatal(err)
	}
	_ = m.showKDir(root)
	if got := m.content.Index(); got != last-1 {
		t.Fatalf("index %d, want %d (clamped)", got, last-1)
	}
	selectTitle(t, &m, "docs")
	idx := m.content.Index()
	if err := os.Remove(filepath.Join(root, "docs", "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "docs")); err != nil {
		t.Fatal(err)
	}
	_ = m.showKDir(root)
	if got := m.content.Index(); got != idx {
		t.Fatalf("index %d, want %d", got, idx)
	}
}

func TestKustomizeRescanAndVisibility(t *testing.T) {
	root := kustTree(t)
	for i := 0; i < 60; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("z%02d.yaml", i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := browseModel(t, root)
	selectTitle(t, &m, "z55.yaml")
	_ = m.showKDir(root)
	if selected(m) != "z55.yaml" || !strings.Contains(m.content.View(), "z55.yaml") {
		t.Fatalf("z55.yaml not selected and visible:\n%s", m.content.View())
	}

	_ = m.showKList(root, []string{root, filepath.Join(root, "base")})
	m.content.Select(1)
	_ = m.showKList(root, []string{root, filepath.Join(root, "base"), filepath.Join(root, "x")})
	if got := m.content.SelectedItem().(item).id; got != filepath.Join(root, "base") {
		t.Fatalf("rescan selects %q", got)
	}

	m = press(t, m, "esc")
	if m.stage != "kdir" || selected(m) != "z55.yaml" {
		t.Fatalf("back from scan: stage %q selected %q", m.stage, selected(m))
	}
}

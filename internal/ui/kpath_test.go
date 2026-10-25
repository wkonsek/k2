package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pathTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"overlays-dev/app", "overlays-prod", "base", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCompletePath(t *testing.T) {
	root := pathTree(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		typed, value string
		cands        []string
	}{
		{"ba", "ba" + "se/", nil},
		{"ov", "overlays-", []string{"overlays-dev", "overlays-prod"}},
		{root + "/overlays-d", root + "/overlays-dev/", nil},
		{"overlays-dev/a", "overlays-dev/app/", nil},
		{"base/../ov", "base/../overlays-", []string{"overlays-dev", "overlays-prod"}},
		{"no", "", nil},
		{"zz", "", nil},
		{".h", ".hidden/", nil},
		{"~/re", "~/repo/", nil},
	}
	for _, c := range cases {
		value, cands := completePath(c.typed, root)
		if value != c.value || !reflect.DeepEqual(cands, c.cands) {
			t.Errorf("completePath(%q) = %q %v, want %q %v", c.typed, value, cands, c.value, c.cands)
		}
	}
	if v, cands := completePath("", root); v != "" || len(cands) != 3 {
		t.Errorf("empty: %q %v (hidden must be left out)", v, cands)
	}
	if got := expandPath("~/repo", root); got != filepath.Join(home, "repo") {
		t.Errorf("expand ~: %q", got)
	}
	if got := expandPath("base/..", root); got != root {
		t.Errorf("expand ..: %q", got)
	}
	if got := expandPath("../", filepath.Join(root, "base")); got != root {
		t.Errorf("expand ../: %q", got)
	}
}

func openPathDialog(t *testing.T, root string) model {
	t.Helper()
	m := browseModel(t, root)
	m = press(t, m, "p")
	if m.dlg == nil || m.dlg.act != "kpath" {
		t.Fatal("p did not open the path dialog")
	}
	return m
}

func TestKustomizePathJump(t *testing.T) {
	root := pathTree(t)
	m := openPathDialog(t, root)
	if got := m.dlg.input.Value(); got != root+"/" {
		t.Fatalf("prefill %q", got)
	}

	m = typeField(t, m, "qdaesD")
	if m.dlg == nil || m.quitting || m.dlg.input.Value() != root+"/qdaesD" {
		t.Fatalf("typed keys leaked: dlg %v quitting %v", m.dlg != nil, m.quitting)
	}

	m = press(t, m, "ctrl+w")
	if m.dlg.input.Value() != "" {
		t.Fatalf("ctrl+w left %q", m.dlg.input.Value())
	}
	m = typeField(t, m, root+"/ba")
	m = press(t, m, "tab", "enter")
	if m.dlg != nil || m.stage != "kdir" || m.kroot != filepath.Join(root, "base") {
		t.Fatalf("enter: dlg %v stage %q kroot %q", m.dlg != nil, m.stage, m.kroot)
	}

	m = press(t, m, "p", "ctrl+u")
	m = typeField(t, m, "..")
	m = press(t, m, "enter")
	if m.kroot != root {
		t.Fatalf("..: kroot %q", m.kroot)
	}

	m = press(t, m, "p")
	m = typeField(t, m, "base")
	m = press(t, m, "esc")
	if m.dlg != nil || m.kroot != root {
		t.Fatalf("esc: dlg %v kroot %q", m.dlg != nil, m.kroot)
	}
}

func TestKustomizePathNotADir(t *testing.T) {
	root := pathTree(t)
	m := openPathDialog(t, root)
	m = typeField(t, m, "notes.txt")
	m = press(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.toast.text, "not a directory") || m.kroot != root {
		t.Fatalf("dlg %v toast %q kroot %q", m.dlg != nil, m.toast.text, m.kroot)
	}
	m = press(t, m, "ctrl+u")
	m = typeField(t, m, "missing")
	m = press(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.toast.text, "not a directory") {
		t.Fatalf("missing: toast %q", m.toast.text)
	}
}

func TestKustomizePathTabCycles(t *testing.T) {
	root := pathTree(t)
	m := openPathDialog(t, root)
	m = typeField(t, m, "ov")
	m = press(t, m, "tab")
	if got := m.dlg.input.Value(); got != root+"/overlays-" {
		t.Fatalf("common prefix: %q", got)
	}
	box := m.dialogBox(60)
	if !strings.Contains(box, "overlays-dev/") || !strings.Contains(box, "overlays-prod/") {
		t.Fatalf("candidates not listed:\n%s", box)
	}
	want := []string{"overlays-dev/", "overlays-prod/", "overlays-dev/"}
	for _, w := range want {
		m = press(t, m, "tab")
		if got := m.dlg.input.Value(); got != root+"/"+w {
			t.Fatalf("cycle: %q, want %q", got, w)
		}
	}
	m = typeField(t, m, "a")
	if m.dlg.cands != nil {
		t.Fatal("typing should end the cycle")
	}
	m = press(t, m, "tab")
	if got := m.dlg.input.Value(); got != root+"/overlays-dev/app/" {
		t.Fatalf("after cycle: %q", got)
	}
	m = press(t, m, "enter")
	if m.kroot != filepath.Join(root, "overlays-dev", "app") {
		t.Fatalf("kroot %q", m.kroot)
	}
}

func typeField(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
	}
	return m
}

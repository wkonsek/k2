package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func kustTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range []string{"kustomization.yaml", "deploy.yaml", "README.md", "base/kustomization.yaml", "docs/notes.txt"} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func browseModel(t *testing.T, dir string) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.cfg.UI.EditorInline = false
	m.cfg.UI.Toasts = true
	_ = m.showKDir(dir)
	if m.stage != "kdir" {
		t.Fatalf("stage %q", m.stage)
	}
	return m
}

func selectTitle(t *testing.T, m *model, name string) {
	t.Helper()
	for i, li := range m.content.Items() {
		if li.(item).title == name {
			m.content.Select(i)
			return
		}
	}
	t.Fatalf("no row %q", name)
}

func titles(m model) []string {
	var out []string
	for _, li := range m.content.Items() {
		out = append(out, li.(item).title)
	}
	return out
}

func TestKustomizeShowsAllFiles(t *testing.T) {
	trueColor(t)
	root := kustTree(t)
	m := browseModel(t, root)
	got := strings.Join(titles(m), " ")
	if got != ".. kustomization.yaml base docs README.md deploy.yaml" {
		t.Fatalf("rows %q", got)
	}
	m.content.Select(0)
	view := m.content.View()
	if !strings.Contains(view, okStyle.Render("●")+" "+itemStyle.Render("kustomization.yaml")) ||
		!strings.Contains(view, okStyle.Render("●")+" "+itemStyle.Render("base/")) {
		t.Fatalf("green dots missing:\n%q", view)
	}
	if !strings.Contains(view, "base/") || !strings.Contains(view, "docs/") {
		t.Fatalf("directories should end in /:\n%s", view)
	}
	if !strings.Contains(view, dimStyle.Render("  README.md")) {
		t.Fatalf("README.md should be dimmed:\n%q", view)
	}
	if strings.Contains(view, dimStyle.Render("  deploy.yaml")) {
		t.Fatalf("deploy.yaml must not be dimmed")
	}
}

func TestKustomizeActionsNeedKustomization(t *testing.T) {
	root := kustTree(t)
	for _, row := range []string{"README.md", "docs"} {
		for _, k := range []string{"a", "D", "d"} {
			m := browseModel(t, root)
			selectTitle(t, &m, row)
			m = press(t, m, k)
			if m.stage != "kdir" || m.asking() != "" {
				t.Fatalf("%s on %s: stage %q confirm %q", k, row, m.stage, m.asking())
			}
			if m.toast.text != "not a kustomization" {
				t.Fatalf("%s on %s: toast %q", k, row, m.toast.text)
			}
		}
	}
	for _, k := range []string{"D", "d"} {
		m := browseModel(t, root)
		selectTitle(t, &m, "deploy.yaml")
		m = press(t, m, k)
		if m.asking() != "" || m.toast.text != "not a kustomization" {
			t.Fatalf("%s on deploy.yaml: confirm %q toast %q", k, m.asking(), m.toast.text)
		}
	}
	m := browseModel(t, root)
	selectTitle(t, &m, "deploy.yaml")
	if got := press(t, m, "a"); got.asking() != "⇪ kubectl apply" {
		t.Fatalf("a on deploy.yaml: confirm %q", got.asking())
	}

	for _, row := range []string{"kustomization.yaml", "base"} {
		m := browseModel(t, root)
		selectTitle(t, &m, row)
		if got := press(t, m, "a"); got.asking() != "⇪ kustomize apply" {
			t.Fatalf("a on %s: confirm %q", row, got.asking())
		}
		if got := press(t, m, "D"); got.stage != "kout" {
			t.Fatalf("D on %s: stage %q", row, got.stage)
		}
	}
}

func TestKustomizeEditAnyFile(t *testing.T) {
	root := kustTree(t)
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd { opened = path; return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })

	cases := map[string]string{
		"README.md":          filepath.Join(root, "README.md"),
		"deploy.yaml":        filepath.Join(root, "deploy.yaml"),
		"kustomization.yaml": filepath.Join(root, "kustomization.yaml"),
		"base":               filepath.Join(root, "base", "kustomization.yaml"),
	}
	for row, want := range cases {
		opened = ""
		m := browseModel(t, root)
		selectTitle(t, &m, row)
		next, cmd := m.Update(key(t, "e"))
		_ = next
		if cmd == nil {
			t.Fatalf("e on %s: no command", row)
		}
		if opened != want {
			t.Fatalf("e on %s opened %q, want %q", row, opened, want)
		}
	}
	opened = ""
	m := browseModel(t, root)
	selectTitle(t, &m, "README.md")
	if _, cmd := m.Update(key(t, "enter")); cmd == nil || opened != filepath.Join(root, "README.md") {
		t.Fatalf("enter on README.md opened %q", opened)
	}
	m = browseModel(t, root)
	selectTitle(t, &m, "docs")
	m = press(t, m, "e")
	if opened != filepath.Join(root, "README.md") || !strings.Contains(m.toast.text, "not a file") {
		t.Fatalf("e on a plain dir: opened %q toast %q", opened, m.toast.text)
	}
}

package ui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"k2/internal/kube"
)

func TestPreferHelmValuesCache(t *testing.T) {
	m := appModel(t, appCluster(t))
	current := kube.ValuesYAML(map[string]any{"replicas": 3, "image": map[string]any{"tag": "1.27"}})
	cached := "# keep my comment\nimage:\n  tag: \"1.27\"\nreplicas: 3\n"
	m.writeHelmValuesCache("kind-dev", "shop", "web", cached)
	got := m.preferHelmValues("kind-dev", "shop", "web", current)
	if got != cached {
		t.Fatalf("cache not used:\n%s\ncurrent:\n%s", got, current)
	}
	other := kube.ValuesYAML(map[string]any{"replicas": 9, "image": map[string]any{"tag": "1.27"}})
	got = m.preferHelmValues("kind-dev", "shop", "web", other)
	if got != other {
		t.Fatalf("stale cache used:\n%s", got)
	}
}

func TestHelmValuesPreviewUsesCache(t *testing.T) {
	f := appCluster(t)
	m := appModel(t, f)
	cached := "# keep my comment\nreplicas: 3\n"
	m.writeHelmValuesCache(m.context, "shop", "web", cached)
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "enter")
	if m.stage != "yaml" || !strings.Contains(m.logText, "# keep my comment") || !strings.Contains(m.logText, "replicas: 3") {
		t.Fatalf("preview:\n%s", m.logText)
	}
	if strings.HasPrefix(strings.TrimSpace(m.logText), "replicas:") && !strings.Contains(m.logText, "# keep my comment") {
		t.Fatal("comment lost")
	}
}

func TestHelmEditValuesUsesCache(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.UI.EditorInline = false
	cached := "# edited by hand\nreplicas: 3\n"
	m.writeHelmValuesCache(m.context, "shop", "web", cached)
	m = selectRow(t, open(t, m, "helm"), "web")
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd {
		opened = path
		return exec.Command("true")
	}
	t.Cleanup(func() { vimCommand = old })
	next, cmd := m.Update(key(t, "v"))
	m = next.(model)
	if opened == "" || cmd == nil {
		t.Fatalf("v: opened %q cmd %v", opened, cmd != nil)
	}
	raw, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != cached {
		t.Fatalf("editor file:\n%s", raw)
	}
}

func TestHelmUpgradeSavesValuesCache(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.UI.EditorInline = false
	m = selectRow(t, open(t, m, "helm"), "web")
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd {
		opened = path
		return exec.Command("true")
	}
	t.Cleanup(func() { vimCommand = old })
	next, _ := m.Update(key(t, "v"))
	m = next.(model)
	if opened == "" {
		t.Fatal("v did not open a file")
	}
	orig, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	edited := "# after upgrade\nreplicas: 7\n"
	if err := os.WriteFile(opened, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	r, ok := m.selectedRelease()
	if !ok {
		t.Fatal("no release")
	}
	chart, version := splitReleaseChart(r.Chart)
	msg := helmValuesEditResult(opened, string(orig), r, chart, version, nil)
	next, cmd := m.Update(msg)
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "y")
	if m.dlg == nil || m.dlg.act != "helmresult" || m.dlg.danger {
		t.Fatalf("expected result popup, got %+v", m.dlg)
	}
	got, err := os.ReadFile(m.helmValuesCachePath(m.context, "shop", "web"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != edited {
		t.Fatalf("cache:\n%s", got)
	}
}

func TestFormatHelmValuesListIndent(t *testing.T) {
	in := "env:\n    - name: FOO\n      value: bar\nreplicaCount: 2\n"
	got := formatHelmValues(in)
	if !strings.Contains(got, "env:\n  - name: FOO\n    value: bar\n") {
		t.Fatalf("not compact 2-space lists:\n%s", got)
	}
	if strings.Contains(got, "    - name:") {
		t.Fatalf("kept 4-space list item:\n%s", got)
	}
}

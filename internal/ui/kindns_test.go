package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/kube"

	"github.com/charmbracelet/x/ansi"
)

func kindFieldKeys(m model) []string {
	var out []string
	for _, f := range m.dlg.hfields {
		out = append(out, f.key)
	}
	return out
}

func kindFieldIndex(m model, key string) int {
	for i, f := range m.dlg.hfields {
		if f.key == key {
			return i
		}
	}
	return -1
}

func TestKindChartNamespaceRowToggles(t *testing.T) {
	withKindCLIs(t)
	m := open(t, appModel(t, kindFake(t, nil)), "kind")
	m = pressSettle(t, m, "A")
	if kindFieldIndex(m, "n:metallb") >= 0 {
		t.Fatalf("ns row shown while off: %v", kindFieldKeys(m))
	}
	m = focusKindField(t, m, "c:metallb")
	m = press(t, m, " ")
	at := kindFieldIndex(m, "n:metallb")
	if at != kindFieldIndex(m, "c:metallb")+1 {
		t.Fatalf("ns row not right under the chart: %v", kindFieldKeys(m))
	}
	if helmFieldValue(m.dlg, "n:metallb") != "metallb-system" || m.dlg.hfields[m.dlg.idx].key != "c:metallb" {
		t.Fatalf("ns %q focus %s", helmFieldValue(m.dlg, "n:metallb"), m.dlg.hfields[m.dlg.idx].key)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "namespace") || !strings.Contains(view, "metallb-system") {
		t.Fatalf("ns row not rendered:\n%s", view)
	}
	m = press(t, m, "down")
	if m.dlg.hfields[m.dlg.idx].key != "n:metallb" {
		t.Fatalf("down landed on %s", m.dlg.hfields[m.dlg.idx].key)
	}
	m = press(t, m, "ctrl+u")
	for _, r := range "lb-cv" {
		m = press(t, m, string(r))
	}
	if helmFieldValue(m.dlg, "n:metallb") != "lb-cv" {
		t.Fatalf("typed ns %q", helmFieldValue(m.dlg, "n:metallb"))
	}
	m = press(t, m, "up", " ")
	if kindFieldIndex(m, "n:metallb") >= 0 {
		t.Fatalf("ns row kept after toggling off: %v", kindFieldKeys(m))
	}
	if m.vs.kop.namespaces["metallb"] != "lb-cv" {
		t.Fatalf("override lost on collapse %v", m.vs.kop.namespaces)
	}
	m = press(t, m, " ")
	if helmFieldValue(m.dlg, "n:metallb") != "lb-cv" {
		t.Fatalf("override not restored %q", helmFieldValue(m.dlg, "n:metallb"))
	}
}

func TestKindCreateUsesOverriddenNamespace(t *testing.T) {
	withKindCLIs(t)
	metalRetryWait, metalRetryN = 0, 1
	f := kindFake(t, nil)
	m := open(t, appModel(t, f), "kind")
	m = pressSettle(t, m, "A")
	m = focusKindField(t, m, "c:kafbat")
	m = press(t, m, " ")
	setHelmField(m.dlg, "n:kafbat", "cluster1")
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "kafbat → cluster1") {
		t.Fatalf("confirm lacks namespace:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var got string
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "install kafbat") {
			got = c
		}
	}
	if !strings.Contains(got, "-n cluster1") || !strings.Contains(got, "--create-namespace") {
		t.Fatalf("install %q in %v", got, helmCalls(f))
	}
}

func TestKindChartsFormNamespace(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = open(t, m, "kind")
	op := kindOp{name: "lab", kubeconfig: path, charts: []string{"metallb"}, installed: []string{"metallb"}, form: "kindcharts", namespaces: map[string]string{"metallb": "lb"}}
	d := m.newDialog("kindcharts", "charts · lab", "kind", "", "lab")
	m.vs.kop = &op
	m.fillKindChartsForm(d, &op)
	m.dlg = d
	i := kindFieldIndex(m, "n:metallb")
	if i < 0 || !m.dlg.hfields[i].locked || helmFieldValue(m.dlg, "n:metallb") != "lb" {
		t.Fatalf("installed chart ns row %v", kindFieldKeys(m))
	}
	m = focusKindField(t, m, "c:argocd")
	m = press(t, m, " ")
	setHelmField(m.dlg, "n:argocd", "gitops")
	m = focusKindField(t, m, "c:metallb")
	m = press(t, m, " ")
	next, cmd := m.submitKindCharts()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "argocd → gitops") || !strings.Contains(view, "metallb → lb") {
		t.Fatalf("confirm:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var inst, uninst string
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "install argocd") {
			inst = c
		}
		if strings.HasPrefix(c, "uninstall metallb") {
			uninst = c
		}
	}
	if !strings.Contains(inst, "-n gitops") || !strings.Contains(uninst, "-n lb") {
		t.Fatalf("install %q uninstall %q", inst, uninst)
	}
}

func TestKindChartsBackKeepsInstalledToggles(t *testing.T) {
	withKindCLIs(t)
	m := open(t, appModel(t, kindFake(t, []string{"lab"})), "kind")
	op := kindOp{name: "lab", kubeconfig: "/kc", charts: []string{"metallb"}, installed: []string{"metallb"}, form: "kindcharts"}
	d := m.newDialog("kindcharts", "charts · lab", "kind", "", "lab")
	m.vs.kop = &op
	m.fillKindChartsForm(d, &op)
	m.dlg = d
	m = focusKindField(t, m, "c:argocd")
	m = press(t, m, " ")
	next, cmd := m.submitKindCharts()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("back did not reopen the form")
	}
	for _, key := range []string{"c:metallb", "c:argocd"} {
		if i := kindFieldIndex(m, key); i < 0 || !m.dlg.hfields[i].on {
			t.Fatalf("%s lost after back", key)
		}
	}
}

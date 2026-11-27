package ui

import (
	"os"
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/kube"

	"github.com/charmbracelet/x/ansi"
)

func TestHelmCatalogCompletesFirstAndFillsForm(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	if m.dlg == nil {
		t.Fatal("no form")
	}
	cands := m.helmChartCandidates()
	if len(cands) < 8 || cands[0] != "istio" {
		t.Fatalf("catalog should lead candidates, got %v", cands[:min(8, len(cands))])
	}
	for _, name := range []string{"istio", "sealed-secrets", "metallb", "argocd", "cert-manager", "metrics-server", "kube-prometheus-stack", "kafbat"} {
		if !containsStr(cands, name) {
			t.Fatalf("missing catalog %s in %v", name, cands)
		}
	}
	for i, f := range m.dlg.hfields {
		if f.key == "chart" {
			m.dlg.idx = i
			helmFocus(&m.dlg.hfields[i])
			break
		}
	}
	setHelmField(m.dlg, "chart", "kafbat")
	next, cmd, done := m.completeHelmField()
	m = settle(t, next.(model), cmd)
	if !done || helmFieldValue(m.dlg, "chart") != "kafbat/kafka-ui" {
		t.Fatalf("chart=%q done=%v", helmFieldValue(m.dlg, "chart"), done)
	}
	if helmFieldValue(m.dlg, "name") != "kafbat-ui" || helmFieldValue(m.dlg, "ns") != "kafbat" || !m.dlg.hcreate {
		t.Fatalf("fill name=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
	if m.vs.hop == nil || m.vs.hop.catalog != "kafbat" {
		t.Fatalf("catalog not marked: %+v", m.vs.hop)
	}
}

func TestHelmCatalogCertManagerValuesAndRepos(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	setHelmField(m.dlg, "chart", "cert-manager")
	m.maybeFillCatalog()
	if helmFieldValue(m.dlg, "name") != "cert-manager" || helmFieldValue(m.dlg, "ns") != "cert-manager" || !m.dlg.hcreate {
		t.Fatalf("fill name=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
	vals := helmFieldValue(m.dlg, "values")
	if vals == "" {
		t.Fatal("values not filled")
	}
	raw, err := os.ReadFile(vals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "crds:") || !strings.Contains(string(raw), "enabled: true") {
		t.Fatalf("values:\n%s", raw)
	}
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "htpl" {
		t.Fatalf("preview stage %s status %q text %s", m.stage, m.status, m.logText)
	}
	helms := helmCalls(f)
	var added, updated, templated bool
	for _, c := range helms {
		if strings.HasPrefix(c, "repo add jetstack ") || strings.Contains(c, "repo add jetstack https://charts.jetstack.io") {
			added = true
		}
		if strings.HasPrefix(c, "repo update jetstack") {
			updated = true
		}
		if strings.HasPrefix(c, "template cert-manager jetstack/cert-manager") && strings.Contains(c, "-n cert-manager") {
			templated = true
		}
	}
	if !added || !updated || !templated {
		t.Fatalf("helm calls %v", helms)
	}
}

func TestHelmCatalogIstioExpandsToBaseThenIstiod(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	setHelmField(m.dlg, "chart", "istio")
	m.maybeFillCatalog()
	if helmFieldValue(m.dlg, "chart") != "istio" || helmFieldValue(m.dlg, "ns") != "istio-system" {
		t.Fatalf("istio fill chart=%s ns=%s", helmFieldValue(m.dlg, "chart"), helmFieldValue(m.dlg, "ns"))
	}
	steps := m.helmSteps(m.vs.hop)
	if len(steps) != 2 || steps[0].HelmRelease() != "istio-base" || steps[0].HelmChart() != "istio/base" || steps[1].HelmRelease() != "istiod" || steps[1].HelmChart() != "istio/istiod" {
		t.Fatalf("steps %+v", steps)
	}
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "htpl" || !strings.Contains(m.logText, "istio-base") || !strings.Contains(m.logText, "istiod") {
		t.Fatalf("preview %s:\n%s", m.stage, m.logText)
	}
	var base, istiod bool
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "template istio-base istio/base") {
			base = true
		}
		if strings.HasPrefix(c, "template istiod istio/istiod") {
			istiod = true
		}
	}
	if !base || !istiod {
		t.Fatalf("helm calls %v", helmCalls(f))
	}
}

func TestHelmCatalogOverrideStillCompletes(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.Helm.Charts = map[string]config.Chart{
		"mycustom": {Name: "mycustom", Repo: "https://example.com/charts", RepoName: "example", Chart: "mycustom", Release: "mine", Namespace: "tools", CreateNamespace: boolPtr(true)},
	}
	m = pressSettle(t, open(t, m, "helm"), "A")
	if !containsStr(m.helmChartCandidates(), "mycustom") {
		t.Fatalf("override missing: %v", m.helmChartCandidates())
	}
	setHelmField(m.dlg, "chart", "mycustom")
	m.maybeFillCatalog()
	if helmFieldValue(m.dlg, "name") != "mine" || helmFieldValue(m.dlg, "ns") != "tools" || !m.dlg.hcreate {
		t.Fatalf("fill name=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
}

func TestHelmNonCatalogInstallSkipsRepoAdd(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	setHelmField(m.dlg, "name", "kafbat-ui")
	setHelmField(m.dlg, "chart", "kafbat-ui/kafka-ui")
	setHelmField(m.dlg, "ns", "kafka")
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "repo add") {
			t.Fatalf("non-catalog install added a repo: %v", helmCalls(f))
		}
	}
	if m.stage != "htpl" {
		t.Fatalf("stage %s status %q", m.stage, m.status)
	}
}

func TestHelmCatalogCommandPrefill(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	next, cmd := m.runCommand("helm install demo metallb")
	m = settle(t, next.(model), cmd)
	if m.dlg == nil {
		t.Fatalf("no form status %q", m.status)
	}
	if helmFieldValue(m.dlg, "name") != "demo" || helmFieldValue(m.dlg, "chart") != "metallb/metallb" || helmFieldValue(m.dlg, "ns") != "metallb-system" || !m.dlg.hcreate {
		t.Fatalf("prefill name=%s chart=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "chart"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func boolPtr(v bool) *bool { return &v }

func focusHelmChart(t *testing.T, m model) model {
	t.Helper()
	if m.dlg == nil {
		t.Fatal("no form")
	}
	for i, f := range m.dlg.hfields {
		if f.key == "chart" {
			m.dlg.idx = i
			break
		}
	}
	return m.moveHelmField(0)
}

func TestHelmCatalogTabCyclesCharts(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	m = focusHelmChart(t, m)
	want := []string{"istio", "sealed-secrets", "metallb", "argocd", "cert-manager", "metrics-server", "kube-prometheus-stack", "kafbat"}
	var got []string
	for i := 0; i < len(want); i++ {
		m = pressSettle(t, m, "tab")
		if m.dlg == nil || m.dlg.hfields[m.dlg.idx].key != "chart" {
			t.Fatalf("tab %d left the chart field idx=%d", i, m.dlg.idx)
		}
		name := ""
		if m.vs.hop != nil {
			name = m.vs.hop.catalog
		}
		if name == "" {
			name = helmFieldValue(m.dlg, "chart")
		}
		got = append(got, name)
	}
	if len(got) != len(want) {
		t.Fatalf("cycled %v", got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("tab cycle %d: got %v want %v", i, got, want)
		}
	}
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "istio" {
		t.Fatalf("tab should wrap to istio, got %+v chart=%s", m.vs.hop, helmFieldValue(m.dlg, "chart"))
	}
	m = pressSettle(t, m, "shift+tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "kafbat" {
		t.Fatalf("shift+tab should cycle back to kafbat, got %+v chart=%s", m.vs.hop, helmFieldValue(m.dlg, "chart"))
	}
	if helmFieldValue(m.dlg, "name") != "kafbat-ui" || helmFieldValue(m.dlg, "ns") != "kafbat" || !m.dlg.hcreate {
		t.Fatalf("kafbat fill name=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
	if helmFieldValue(m.dlg, "chart") != "kafbat/kafka-ui" {
		t.Fatalf("kafbat chart=%s", helmFieldValue(m.dlg, "chart"))
	}
}

func TestHelmCatalogCycleUpdatesAndKeepsEdits(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	m = focusHelmChart(t, m)
	m = pressSettle(t, m, "tab")
	if helmFieldValue(m.dlg, "chart") != "istio" || helmFieldValue(m.dlg, "ns") != "istio-system" || !m.dlg.hcreate {
		t.Fatalf("istio fill chart=%s ns=%s create=%v", helmFieldValue(m.dlg, "chart"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "sealed-secrets" || helmFieldValue(m.dlg, "name") != "sealed-secrets" || helmFieldValue(m.dlg, "ns") != "kube-system" || m.dlg.hcreate {
		t.Fatalf("sealed-secrets fill name=%s ns=%s create=%v catalog=%s", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate, m.vs.hop.catalog)
	}
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "metallb" || helmFieldValue(m.dlg, "name") != "metallb" || helmFieldValue(m.dlg, "ns") != "metallb-system" || !m.dlg.hcreate || helmFieldValue(m.dlg, "chart") != "metallb/metallb" {
		t.Fatalf("metallb fill name=%s chart=%s ns=%s create=%v", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "chart"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
	setHelmField(m.dlg, "name", "my-release")
	if m.vs.hop != nil {
		m.vs.hop.autoName = false
	}
	m = focusHelmChart(t, m)
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "argocd" {
		t.Fatalf("next catalog %+v chart=%s", m.vs.hop, helmFieldValue(m.dlg, "chart"))
	}
	if helmFieldValue(m.dlg, "name") != "my-release" {
		t.Fatalf("manual name was overwritten: %s", helmFieldValue(m.dlg, "name"))
	}
	if helmFieldValue(m.dlg, "ns") != "argocd" || !m.dlg.hcreate {
		t.Fatalf("argocd auto ns=%s create=%v", helmFieldValue(m.dlg, "ns"), m.dlg.hcreate)
	}
}

func TestHelmCatalogSuggestsUnderChart(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "▸ istio") || strings.Contains(view, "sealed-secrets") && strings.Contains(view, "cycle charts") {
		t.Fatalf("suggestions should wait until chart is focused:\n%s", view)
	}
	m = focusHelmChart(t, m)
	view = ansi.Strip(m.View())
	for _, want := range []string{"istio", "sealed-secrets", "metallb", "argocd", "cert-manager", "metrics-server", "kube-prometheus-stack", "kafbat", "cycle charts"} {
		if !strings.Contains(view, want) {
			t.Fatalf("focused chart view lacks %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, kindIconSel+" istio") && !strings.Contains(view, kindIconSel+"  istio") {
		t.Fatalf("first suggestion should be marked:\n%s", view)
	}
	m = pressSettle(t, m, "tab", "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "sealed-secrets" {
		t.Fatalf("second tab catalog %+v", m.vs.hop)
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, kindIconSel+" sealed-secrets") && !strings.Contains(view, kindIconSel+"  sealed-secrets") {
		t.Fatalf("current suggestion should be marked:\n%s", view)
	}
	setHelmField(m.dlg, "chart", "met")
	m.syncHelmChartSuggest()
	sugs := m.helmChartSuggestions(helmFieldValue(m.dlg, "chart"))
	if !containsStr(sugs, "metallb") || !containsStr(sugs, "metrics-server") {
		t.Fatalf("typed prefix should match metallb and metrics-server: %v chart=%s", sugs, helmFieldValue(m.dlg, "chart"))
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "metallb") || !strings.Contains(view, "metrics-server") {
		t.Fatalf("filtered list missing matches:\n%s", view)
	}
	if containsStr(m.dlg.cands, "sealed-secrets") || containsStr(m.dlg.cands, "istio") || containsStr(m.dlg.cands, "kube-prometheus-stack") {
		t.Fatalf("filtered candidates still include a non-match: %v", m.dlg.cands)
	}
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "metallb" {
		t.Fatalf("tab on met should pick metallb, got %+v chart=%s", m.vs.hop, helmFieldValue(m.dlg, "chart"))
	}
	m = pressSettle(t, m, "tab")
	if m.vs.hop == nil || m.vs.hop.catalog != "metrics-server" {
		t.Fatalf("second tab should pick metrics-server, got %+v", m.vs.hop)
	}
	before := m.dlg.idx
	m = pressSettle(t, m, "down")
	if m.dlg.idx == before {
		t.Fatal("down should leave the chart field")
	}
	m = pressSettle(t, m, "up")
	if m.dlg.hfields[m.dlg.idx].key != "chart" {
		t.Fatalf("up should return to chart, at %s", m.dlg.hfields[m.dlg.idx].key)
	}
}

func TestHelmChartSearchSuggestions(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	m.vs.hop.charts = []kube.HelmChart{{Name: "bitnami/redis"}, {Name: "bitnami/nginx"}, {Name: "metallb/metallb"}}
	m = focusHelmChart(t, m)
	empty := m.helmChartSuggestions("")
	if containsStr(empty, "bitnami/redis") {
		t.Fatalf("empty prefix should stay on the catalog, got %v", empty)
	}
	hits := m.helmChartSuggestions("bit")
	if !containsStr(hits, "bitnami/redis") || !containsStr(hits, "bitnami/nginx") {
		t.Fatalf("search hits missing: %v", hits)
	}
	if containsStr(hits, "metallb") || containsStr(hits, "istio") {
		t.Fatalf("unrelated catalog leaked into bit prefix: %v", hits)
	}
}

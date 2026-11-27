package ui

import (
	"os"
	"strings"
	"testing"

	"k2/internal/config"
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

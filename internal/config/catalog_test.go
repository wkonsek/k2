package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCatalog(t *testing.T) {
	m := DefaultChartMap()
	order := DefaultChartOrder()
	if len(m) != len(order) {
		t.Fatalf("len %d %d", len(m), len(order))
	}
	istio, ok := m["istio"]
	if !ok || len(istio.InstallSteps()) != 2 {
		t.Fatalf("istio steps %+v", istio)
	}
	steps := istio.InstallSteps()
	if steps[0].HelmChart() != "istio/base" || steps[0].HelmRelease() != "istio-base" || !steps[0].CreatesNamespace() {
		t.Fatalf("base %+v", steps[0])
	}
	if steps[0].Repo != "https://blob.istio.io/istio-release/charts" {
		t.Fatalf("official istio repo %s", steps[0].Repo)
	}
	if steps[0].Values["defaultRevision"] != "default" {
		t.Fatalf("defaultRevision %+v", steps[0].Values)
	}
	if steps[1].HelmChart() != "istio/istiod" || steps[1].HelmRelease() != "istiod" {
		t.Fatalf("istiod %+v", steps[1])
	}
	kafbat := m["kafbat"]
	if kafbat.HelmChart() != "kafbat/kafka-ui" || kafbat.HelmRelease() != "kafbat-ui" || kafbat.Namespace != "kafbat" || !kafbat.CreatesNamespace() {
		t.Fatalf("kafbat %+v", kafbat)
	}
	cm := m["cert-manager"]
	crds, _ := cm.Values["crds"].(map[string]any)
	if crds["enabled"] != true || cm.Repo != "https://charts.jetstack.io" {
		t.Fatalf("cert-manager %+v", cm)
	}
	ms := m["metrics-server"]
	if ms.CreatesNamespace() || ms.Namespace != "kube-system" {
		t.Fatalf("metrics-server ns")
	}
	args, _ := ms.Values["args"].([]any)
	if len(args) != 1 || args[0] != "--kubelet-insecure-tls" {
		t.Fatalf("metrics-server args %+v", args)
	}
	prom := m["kube-prometheus-stack"]
	if prom.Namespace != "monitoring" || !prom.CreatesNamespace() {
		t.Fatalf("prometheus ns %+v", prom)
	}
	ss := m["sealed-secrets"]
	if ss.CreatesNamespace() || ss.Repo != "https://bitnami.github.io/sealed-secrets" {
		t.Fatalf("sealed-secrets %+v", ss)
	}
}

func TestMergeChartsOverrideAndExtend(t *testing.T) {
	over := map[string]Chart{
		"kafbat":       {Namespace: "kafka-lab", Release: "ui", Values: map[string]any{"replicaCount": 2}},
		"nats":         {Chart: "nats", Repo: "https://nats-io.github.io/k8s/helm/charts/", RepoName: "nats", Namespace: "nats", CreateNamespace: boolPtr(true)},
		"cert-manager": {Values: map[string]any{"crds": map[string]any{"keep": true}}},
		"istio":        {Repo: "https://example.invalid/istio"},
	}
	list := ChartsList(MergeCharts(DefaultChartMap(), over))
	var names []string
	by := map[string]Chart{}
	for _, c := range list {
		names = append(names, c.Name)
		by[c.Name] = c
	}
	joined := strings.Join(names, ",")
	if !strings.HasPrefix(joined, "istio,sealed-secrets,metallb,argocd,cert-manager,metrics-server,kube-prometheus-stack,kafbat") {
		t.Fatalf("order %s", joined)
	}
	if names[len(names)-1] != "nats" {
		t.Fatalf("extra last %s", joined)
	}
	k := by["kafbat"]
	if k.Namespace != "kafka-lab" || k.HelmRelease() != "ui" || k.HelmChart() != "kafbat/kafka-ui" || k.Values["replicaCount"] != 2 {
		t.Fatalf("kafbat override %+v", k)
	}
	cm := by["cert-manager"]
	crds, _ := cm.Values["crds"].(map[string]any)
	if crds["enabled"] != true || crds["keep"] != true {
		t.Fatalf("deep values %+v", crds)
	}
	istio := by["istio"]
	if istio.Repo != "https://example.invalid/istio" || istio.InstallSteps()[0].Repo != "https://example.invalid/istio" {
		t.Fatalf("istio repo propagate %+v %+v", istio.Repo, istio.InstallSteps()[0].Repo)
	}
	if by["nats"].HelmChart() != "nats/nats" || !by["nats"].CreatesNamespace() {
		t.Fatalf("nats %+v", by["nats"])
	}
}

func TestLookupChartAndRepoArgs(t *testing.T) {
	list := Default().Charts()
	c, ok := LookupChart(list, "kafbat")
	if !ok || c.HelmChart() != "kafbat/kafka-ui" {
		t.Fatalf("by key %+v %v", c, ok)
	}
	c, ok = LookupChart(list, "kafbat/kafka-ui")
	if !ok || c.Name != "kafbat" {
		t.Fatalf("by chart %+v %v", c, ok)
	}
	c, ok = LookupChart(list, "istio/base")
	if !ok || c.HelmRelease() != "istio-base" {
		t.Fatalf("istio base %+v %v", c, ok)
	}
	c, ok = LookupChart(list, "istio")
	if !ok || len(c.InstallSteps()) != 2 {
		t.Fatalf("istio bundle %+v %v", c, ok)
	}
	if _, ok := LookupChart(list, "nope"); ok {
		t.Fatal("unknown")
	}
	add := strings.Join(HelmRepoAddArgs("kafbat", "https://ui.charts.kafbat.io"), " ")
	if add != "repo add kafbat https://ui.charts.kafbat.io --force-update" {
		t.Fatalf("add %s", add)
	}
	if strings.Join(HelmRepoUpdateArgs("kafbat"), " ") != "repo update kafbat" {
		t.Fatalf("update")
	}
}

func TestValuesFileYAML(t *testing.T) {
	text, err := ValuesFileYAML(map[string]any{"crds": map[string]any{"enabled": true}})
	if err != nil || !strings.Contains(text, "crds:") || !strings.Contains(text, "enabled: true") {
		t.Fatalf("%q %v", text, err)
	}
	if s, err := ValuesFileYAML(nil); err != nil || s != "" {
		t.Fatalf("empty %q %v", s, err)
	}
}

func TestConfigHelmChartsAndKind(t *testing.T) {
	c, w := Parse([]byte(`
helm:
  binary: /opt/helm
  charts:
    kafbat:
      namespace: kafka
      release: ui
    mine:
      chart: demo
      repo: https://example.invalid/charts
      repo_name: ex
      create_namespace: true
kind:
  binary: /usr/local/bin/kind
`))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	if c.Helm.Binary != "/opt/helm" || c.Kind.Binary != "/usr/local/bin/kind" {
		t.Fatalf("binaries %+v %+v", c.Helm, c.Kind)
	}
	var kafbat, mine Chart
	for _, ch := range c.Charts() {
		if ch.Name == "kafbat" {
			kafbat = ch
		}
		if ch.Name == "mine" {
			mine = ch
		}
	}
	if kafbat.Namespace != "kafka" || kafbat.HelmRelease() != "ui" || kafbat.HelmChart() != "kafbat/kafka-ui" {
		t.Fatalf("kafbat %+v", kafbat)
	}
	if mine.HelmChart() != "ex/demo" || !mine.CreatesNamespace() {
		t.Fatalf("mine %+v", mine)
	}
	c, w = Parse([]byte("kind: {binary: ~}"))
	if len(w) != 0 || c.Kind.Binary != "" {
		t.Fatalf("kind off %v %q", w, c.Kind.Binary)
	}
}

func TestConfigMasterKeyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, w := Parse([]byte(`
helm:
  charts:
    sealed-secrets:
      master_key_path: ~/secrets/master.yml
    kafbat:
      namespace: kafka
`))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	ss, ok := LookupChart(c.Charts(), "sealed-secrets")
	if !ok || ss.MasterKeyPath != "~/secrets/master.yml" {
		t.Fatalf("stored path %+v %v", ss, ok)
	}
	want := filepath.Join(home, "secrets", "master.yml")
	if ss.ExpandedMasterKeyPath() != want {
		t.Fatalf("expand %q want %q", ss.ExpandedMasterKeyPath(), want)
	}
	if !ss.IsSealedSecrets() {
		t.Fatal("IsSealedSecrets")
	}
	kafbat, ok := LookupChart(c.Charts(), "kafbat")
	if !ok || kafbat.MasterKeyPath != "" || kafbat.IsSealedSecrets() {
		t.Fatalf("kafbat %+v %v", kafbat, ok)
	}
	if !IsSealedSecretsName("sealed-secrets/sealed-secrets") || IsSealedSecretsName("metallb") {
		t.Fatal("name helper")
	}
}

func TestMergeChartsMasterKeyPath(t *testing.T) {
	over := map[string]Chart{"sealed-secrets": {MasterKeyPath: "~/keys/ss.yml"}}
	got := MergeCharts(DefaultChartMap(), over)["sealed-secrets"]
	if got.MasterKeyPath != "~/keys/ss.yml" || got.Namespace != "kube-system" || !got.IsSealedSecrets() {
		t.Fatalf("%+v", got)
	}
	if got.InstallSteps()[0].MasterKeyPath != "~/keys/ss.yml" {
		t.Fatalf("steps did not inherit %q", got.InstallSteps()[0].MasterKeyPath)
	}
}

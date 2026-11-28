package config

import (
	"bytes"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Chart struct {
	Name            string
	Release         string
	Chart           string
	Repo            string
	RepoName        string
	Namespace       string
	Version         string
	MasterKeyPath   string
	CreateNamespace *bool
	Values          map[string]any
	Steps           []Chart
}

func boolPtr(v bool) *bool { return &v }

func (c Chart) CreatesNamespace() bool {
	return c.CreateNamespace != nil && *c.CreateNamespace
}

func (c Chart) HelmRepo() string {
	if c.RepoName != "" {
		return c.RepoName
	}
	if i := strings.Index(c.Chart, "/"); i > 0 {
		return c.Chart[:i]
	}
	if c.Name != "" {
		return c.Name
	}
	return ""
}

func (c Chart) HelmChart() string {
	if strings.Contains(c.Chart, "/") {
		return c.Chart
	}
	repo := c.HelmRepo()
	if repo != "" && c.Chart != "" {
		return repo + "/" + c.Chart
	}
	return c.Chart
}

func (c Chart) HelmRelease() string {
	if c.Release != "" {
		return c.Release
	}
	if c.Name != "" {
		return c.Name
	}
	chart := c.HelmChart()
	if i := strings.LastIndex(chart, "/"); i >= 0 {
		return chart[i+1:]
	}
	return chart
}

func IsSealedSecretsName(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "sealed-secrets" || s == "sealed-secrets/sealed-secrets"
}

func (c Chart) IsSealedSecrets() bool {
	return IsSealedSecretsName(c.Name) || IsSealedSecretsName(c.Chart) || IsSealedSecretsName(c.HelmChart()) || IsSealedSecretsName(c.HelmRelease())
}

func (c Chart) ExpandedMasterKeyPath() string {
	return ExpandHome(strings.TrimSpace(c.MasterKeyPath))
}

func (c Chart) InstallSteps() []Chart {
	if len(c.Steps) == 0 {
		return []Chart{c}
	}
	out := make([]Chart, len(c.Steps))
	for i, s := range c.Steps {
		if s.Repo == "" {
			s.Repo = c.Repo
		}
		if s.RepoName == "" {
			s.RepoName = c.RepoName
		}
		if s.Namespace == "" {
			s.Namespace = c.Namespace
		}
		if s.Version == "" {
			s.Version = c.Version
		}
		if s.MasterKeyPath == "" {
			s.MasterKeyPath = c.MasterKeyPath
		}
		out[i] = s
	}
	return out
}

func DefaultChartOrder() []string {
	return []string{
		"istio", "sealed-secrets", "metallb", "argocd",
		"cert-manager", "metrics-server", "kube-prometheus-stack", "kafbat",
	}
}

func DefaultChartMap() map[string]Chart {
	istioRepo := "https://blob.istio.io/istio-release/charts"
	return map[string]Chart{
		"istio": {
			Name: "istio", Repo: istioRepo, RepoName: "istio", Namespace: "istio-system",
			CreateNamespace: boolPtr(true),
			Steps: []Chart{
				{
					Name: "istio-base", Release: "istio-base", Chart: "base",
					Repo: istioRepo, RepoName: "istio", Namespace: "istio-system",
					CreateNamespace: boolPtr(true),
					Values:          map[string]any{"defaultRevision": "default"},
				},
				{
					Name: "istiod", Release: "istiod", Chart: "istiod",
					Repo: istioRepo, RepoName: "istio", Namespace: "istio-system",
					CreateNamespace: boolPtr(false),
				},
			},
		},
		"sealed-secrets": {
			Name: "sealed-secrets", Release: "sealed-secrets", Chart: "sealed-secrets",
			Repo: "https://bitnami.github.io/sealed-secrets", RepoName: "sealed-secrets",
			Namespace: "kube-system", CreateNamespace: boolPtr(false),
		},
		"metallb": {
			Name: "metallb", Release: "metallb", Chart: "metallb",
			Repo: "https://metallb.github.io/metallb", RepoName: "metallb",
			Namespace: "metallb-system", CreateNamespace: boolPtr(true),
		},
		"argocd": {
			Name: "argocd", Release: "argocd", Chart: "argo-cd",
			Repo: "https://argoproj.github.io/argo-helm", RepoName: "argo",
			Namespace: "argocd", CreateNamespace: boolPtr(true),
		},
		"cert-manager": {
			Name: "cert-manager", Release: "cert-manager", Chart: "cert-manager",
			Repo: "https://charts.jetstack.io", RepoName: "jetstack",
			Namespace: "cert-manager", CreateNamespace: boolPtr(true),
			Values: map[string]any{"crds": map[string]any{"enabled": true}},
		},
		"metrics-server": {
			Name: "metrics-server", Release: "metrics-server", Chart: "metrics-server",
			Repo: "https://kubernetes-sigs.github.io/metrics-server/", RepoName: "metrics-server",
			Namespace: "kube-system", CreateNamespace: boolPtr(false),
			Values: map[string]any{"args": []any{"--kubelet-insecure-tls"}},
		},
		"kube-prometheus-stack": {
			Name: "kube-prometheus-stack", Release: "kube-prometheus-stack", Chart: "kube-prometheus-stack",
			Repo: "https://prometheus-community.github.io/helm-charts", RepoName: "prometheus-community",
			Namespace: "monitoring", CreateNamespace: boolPtr(true),
		},
		"kafbat": {
			Name: "kafbat", Release: "kafbat-ui", Chart: "kafka-ui",
			Repo: "https://ui.charts.kafbat.io", RepoName: "kafbat",
			Namespace: "kafbat", CreateNamespace: boolPtr(true),
		},
	}
}

func MergeCharts(base, over map[string]Chart) map[string]Chart {
	out := make(map[string]Chart, len(base)+len(over))
	for k, v := range base {
		v.Name = k
		out[k] = v
	}
	for k, v := range over {
		if cur, ok := out[k]; ok {
			out[k] = mergeChart(cur, v)
			continue
		}
		v.Name = k
		out[k] = v
	}
	return out
}

func mergeChart(base, over Chart) Chart {
	out := base
	if over.Release != "" {
		out.Release = over.Release
	}
	if over.Chart != "" {
		out.Chart = over.Chart
	}
	if over.Repo != "" {
		out.Repo = over.Repo
	}
	if over.RepoName != "" {
		out.RepoName = over.RepoName
	}
	if over.Namespace != "" {
		out.Namespace = over.Namespace
	}
	if over.Version != "" {
		out.Version = over.Version
	}
	if over.MasterKeyPath != "" {
		out.MasterKeyPath = over.MasterKeyPath
	}
	if over.CreateNamespace != nil {
		out.CreateNamespace = over.CreateNamespace
	}
	if len(over.Values) > 0 {
		out.Values = mergeValues(out.Values, over.Values)
	}
	if len(over.Steps) > 0 {
		out.Steps = over.Steps
	}
	out.Name = base.Name
	if out.Name == "" {
		out.Name = over.Name
	}
	if over.Repo != "" {
		for i := range out.Steps {
			out.Steps[i].Repo = out.Repo
		}
	}
	if over.RepoName != "" {
		for i := range out.Steps {
			out.Steps[i].RepoName = out.RepoName
		}
	}
	return out
}

func mergeValues(base, over map[string]any) map[string]any {
	if len(base) == 0 {
		return cloneValues(over)
	}
	out := cloneValues(base)
	for k, v := range over {
		if bm, ok := out[k].(map[string]any); ok {
			if om, ok := v.(map[string]any); ok {
				out[k] = mergeValues(bm, om)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func cloneValues(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]any); ok {
			out[k] = cloneValues(m)
			continue
		}
		out[k] = v
	}
	return out
}

func ChartsList(m map[string]Chart) []Chart {
	seen := map[string]bool{}
	var out []Chart
	for _, k := range DefaultChartOrder() {
		if c, ok := m[k]; ok {
			c.Name = k
			out = append(out, c)
			seen[k] = true
		}
	}
	var extra []string
	for k := range m {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		c := m[k]
		c.Name = k
		out = append(out, c)
	}
	return out
}

func (c Config) Charts() []Chart {
	return ChartsList(MergeCharts(DefaultChartMap(), c.Helm.Charts))
}

func LookupChart(charts []Chart, q string) (Chart, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return Chart{}, false
	}
	for _, c := range charts {
		if c.Name == q || c.HelmChart() == q || c.HelmRelease() == q || c.Chart == q {
			return c, true
		}
		for _, s := range c.InstallSteps() {
			if s.Name == q || s.HelmChart() == q || s.HelmRelease() == q || s.Chart == q {
				if s.Name == q || s.HelmChart() == q || s.Chart == q {
					return s, true
				}
			}
		}
	}
	return Chart{}, false
}

func HelmRepoAddArgs(name, url string) []string {
	return []string{"repo", "add", name, url, "--force-update"}
}

func HelmRepoUpdateArgs(name string) []string {
	return []string{"repo", "update", name}
}

func ValuesFileYAML(values map[string]any) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(values); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func WriteValuesFile(values map[string]any) (string, error) {
	text, err := ValuesFileYAML(values)
	if err != nil || text == "" {
		return "", err
	}
	f, err := os.CreateTemp("", "k2-values-*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		return "", err
	}
	return f.Name(), nil
}

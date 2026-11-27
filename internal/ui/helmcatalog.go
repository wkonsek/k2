package ui

import (
	"k2/internal/config"
	"k2/internal/kube"
)

func (m model) helmChartCandidates() []string {
	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	for _, c := range m.cfg.Charts() {
		add(c.Name)
	}
	for _, c := range m.cfg.Charts() {
		add(c.HelmChart())
	}
	if m.vs.hop != nil {
		for _, n := range kube.ChartNames(m.vs.hop.charts) {
			add(n)
		}
	}
	return names
}

func catalogChartField(c config.Chart) string {
	if len(c.Steps) > 0 {
		return c.Name
	}
	return c.HelmChart()
}

func (m *model) maybeFillCatalog() {
	if m.dlg == nil || m.vs.hop == nil || m.vs.hop.act != "install" {
		return
	}
	chart := helmFieldValue(m.dlg, "chart")
	c, ok := config.LookupChart(m.cfg.Charts(), chart)
	if !ok {
		return
	}
	op := *m.vs.hop
	op.catalog = c.Name
	if helmFieldValue(m.dlg, "name") == "" || op.autoName {
		setHelmField(m.dlg, "name", c.HelmRelease())
		op.autoName = true
	}
	setHelmField(m.dlg, "chart", catalogChartField(c))
	if c.Namespace != "" && (helmFieldValue(m.dlg, "ns") == "" || op.autoNS || helmFieldValue(m.dlg, "ns") == "default" || helmFieldValue(m.dlg, "ns") == m.namespace) {
		setHelmField(m.dlg, "ns", c.Namespace)
		op.autoNS = true
	}
	m.dlg.hcreate = c.CreatesNamespace()
	if (op.values == "" || op.autoValues) && len(c.Values) > 0 && len(c.Steps) == 0 {
		path, err := config.WriteValuesFile(c.Values)
		if err == nil {
			op.values = path
			op.autoValues = true
			setHelmField(m.dlg, "values", path)
		}
	}
	m.vs.hop = &op
}

func (m model) helmSteps(op *helmOp) []config.Chart {
	c, ok := config.LookupChart(m.cfg.Charts(), or(op.catalog, op.chart))
	if ok && (len(c.Steps) > 0 && (op.chart == c.Name || op.catalog == c.Name)) {
		return c.InstallSteps()
	}
	if ok {
		s := c
		s.Release = or(op.name, c.HelmRelease())
		s.Namespace = or(op.ns, c.Namespace)
		s.Version = op.version
		on := op.createNS
		s.CreateNamespace = &on
		return []config.Chart{s}
	}
	on := op.createNS
	return []config.Chart{{
		Release: op.name, Chart: op.chart, Version: op.version, Namespace: op.ns,
		CreateNamespace: &on,
	}}
}

func (m model) ensureHelmRepos(steps []config.Chart) error {
	return ensureRepos(m.k, m.cfg.Helm.Binary, steps, func(string) {})
}

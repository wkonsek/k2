package ui

import (
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
)

const kindNsLabel = "  namespace"

func kindChartDefaultNS(c config.Chart) string {
	if steps := c.InstallSteps(); len(steps) > 0 && steps[0].Namespace != "" {
		return steps[0].Namespace
	}
	return "default"
}

func (op kindOp) chartNS(c config.Chart) string {
	if op.namespaces != nil {
		if ns := strings.TrimSpace(op.namespaces[c.Name]); ns != "" {
			return ns
		}
	}
	return kindChartDefaultNS(c)
}

func (op kindOp) chartSteps(c config.Chart) []config.Chart {
	steps := c.InstallSteps()
	def, ns := kindChartDefaultNS(c), op.chartNS(c)
	if ns == def {
		return steps
	}
	on := true
	for i := range steps {
		if steps[i].Namespace == "" || steps[i].Namespace == def {
			steps[i].Namespace = ns
			steps[i].CreateNamespace = &on
		}
	}
	return steps
}

func (m model) kindNsField(op *kindOp, c config.Chart, locked bool) helmField {
	return helmField{key: "n:" + c.Name, label: kindNsLabel, input: helmFieldInput(op.chartNS(c), kindChartDefaultNS(c)), locked: locked}
}

func (op kindOp) isInstalled(name string) bool {
	for _, n := range op.installed {
		if n == name {
			return true
		}
	}
	return false
}

func (m *model) readKindNamespaces(op *kindOp) {
	if m.dlg == nil {
		return
	}
	for _, f := range m.dlg.hfields {
		if !strings.HasPrefix(f.key, "n:") || f.locked {
			continue
		}
		name := strings.TrimPrefix(f.key, "n:")
		c, ok := config.LookupChart(m.cfg.Charts(), name)
		if !ok {
			continue
		}
		if op.namespaces == nil {
			op.namespaces = map[string]string{}
		}
		ns := strings.TrimSpace(f.input.Value())
		if ns == "" || ns == kindChartDefaultNS(c) {
			delete(op.namespaces, name)
			continue
		}
		op.namespaces[name] = ns
	}
}

func (m *model) syncKindNsRows() {
	d := m.dlg
	if d == nil || m.vs.kop == nil {
		return
	}
	m.readKindNamespaces(m.vs.kop)
	cur := ""
	if d.idx >= 0 && d.idx < len(d.hfields) {
		cur = d.hfields[d.idx].key
	}
	old := map[string]helmField{}
	for _, f := range d.hfields {
		if strings.HasPrefix(f.key, "n:") {
			old[f.key] = f
		}
	}
	var out []helmField
	for _, f := range d.hfields {
		if strings.HasPrefix(f.key, "n:") {
			continue
		}
		out = append(out, f)
		if !strings.HasPrefix(f.key, "c:") || !f.on {
			continue
		}
		name := strings.TrimPrefix(f.key, "c:")
		if keep, ok := old["n:"+name]; ok {
			out = append(out, keep)
			continue
		}
		if c, ok := config.LookupChart(m.cfg.Charts(), name); ok {
			out = append(out, m.kindNsField(m.vs.kop, c, m.vs.kop.isInstalled(name)))
		}
	}
	d.hfields = out
	d.idx = kindFocusField(out, cur)
}

func kindChartsWithNS(names []string, op kindOp, charts []config.Chart) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if c, ok := config.LookupChart(charts, n); ok {
			parts = append(parts, n+" → "+op.chartNS(c))
			continue
		}
		parts = append(parts, n)
	}
	return strings.Join(parts, ", ")
}

func kindInstalledNamespaces(secrets []kube.Object, charts []config.Chart, selected []string) map[string]string {
	at := map[string]string{}
	for _, r := range kube.HelmLatest(secrets) {
		if r.Name != "" {
			at[r.Name] = r.Namespace
		}
	}
	out := map[string]string{}
	for _, name := range selected {
		c, ok := config.LookupChart(charts, name)
		if !ok {
			continue
		}
		for _, s := range c.InstallSteps() {
			if ns := at[s.HelmRelease()]; ns != "" {
				if ns != kindChartDefaultNS(c) {
					out[name] = ns
				}
				break
			}
		}
	}
	return out
}

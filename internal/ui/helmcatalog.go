package ui

import (
	"strings"

	"k2/internal/config"
	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) helmChartCandidates() []string {
	return m.helmChartSuggestions("")
}

func helmChartMatches(name, prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.HasPrefix(name, prefix) {
		return true
	}
	for _, sep := range []string{"-", "/", "."} {
		if strings.Contains(name, sep+prefix) {
			return true
		}
	}
	return false
}

func (m model) helmChartSuggestions(prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] || !helmChartMatches(s, prefix) {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	for _, c := range m.cfg.Charts() {
		add(c.Name)
	}
	if prefix != "" && m.vs.hop != nil {
		skip := map[string]bool{}
		for _, c := range m.cfg.Charts() {
			skip[c.Name] = true
			skip[c.HelmChart()] = true
		}
		for _, n := range kube.ChartNames(m.vs.hop.charts) {
			if skip[n] {
				continue
			}
			add(n)
		}
	}
	return names
}

func (m model) helmChartCandIndex(cands []string, value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	for i, c := range cands {
		if c == value {
			return i
		}
		if ch, ok := config.LookupChart(m.cfg.Charts(), c); ok && catalogChartField(ch) == value {
			return i
		}
		if ch, ok := config.LookupChart(m.cfg.Charts(), value); ok && (ch.Name == c || catalogChartField(ch) == c) {
			return i
		}
	}
	return -1
}

func (m *model) syncHelmChartSuggest() {
	m.refreshHelmChartSuggest(false)
}

func (m *model) refreshHelmChartSuggest(fromType bool) {
	if m.dlg == nil || m.dlg.idx < 0 || m.dlg.idx >= len(m.dlg.hfields) || m.dlg.hfields[m.dlg.idx].key != "chart" {
		return
	}
	typed := helmFieldValue(m.dlg, "chart")
	if fromType {
		m.dlg.candBase = typed
		m.dlg.cands = m.helmChartSuggestions(typed)
		m.dlg.candAt = m.helmChartCandIndex(m.dlg.cands, typed)
		return
	}
	if i := m.helmChartCandIndex(m.dlg.cands, typed); i >= 0 {
		m.dlg.candAt = i
		if m.dlg.candBase != "" {
			cur := m.dlg.cands[i]
			m.dlg.cands = m.helmChartSuggestions(m.dlg.candBase)
			m.dlg.candAt = m.helmChartCandIndex(m.dlg.cands, cur)
		}
		return
	}
	prefix := typed
	if _, ok := config.LookupChart(m.cfg.Charts(), typed); ok {
		prefix = ""
	}
	m.dlg.candBase = prefix
	m.dlg.cands = m.helmChartSuggestions(prefix)
	m.dlg.candAt = m.helmChartCandIndex(m.dlg.cands, typed)
}

func (m model) cycleHelmChart(delta int) (tea.Model, tea.Cmd) {
	d := m.dlg
	if d == nil || d.idx < 0 || d.idx >= len(d.hfields) {
		return m, nil
	}
	typed := strings.TrimSpace(d.hfields[d.idx].input.Value())
	rebuilt := false
	if len(d.cands) == 0 || (m.helmChartCandIndex(d.cands, typed) < 0 && typed != d.candBase) {
		d.candBase = typed
		d.cands = m.helmChartSuggestions(typed)
		rebuilt = true
	}
	if len(d.cands) == 0 {
		d.msg = "no match"
		return m, nil
	}
	at := m.helmChartCandIndex(d.cands, typed)
	if at < 0 {
		if delta < 0 {
			at = len(d.cands) - 1
		} else {
			at = 0
		}
	} else if !rebuilt {
		at = (at + delta) % len(d.cands)
		if at < 0 {
			at += len(d.cands)
		}
	}
	d.candAt = at
	d.msg = ""
	setHelmField(d, "chart", d.cands[at])
	m.maybeFillCatalog()
	return m, m.liveHelmVersions()
}

func (d *dialog) helmChartSuggestRows(inner, indent int) []string {
	if len(d.cands) == 0 {
		return nil
	}
	if indent < 0 {
		indent = 0
	}
	start, end := 0, len(d.cands)
	if end-start > maxCands {
		at := d.candAt
		if at < 0 {
			at = 0
		}
		start = at - maxCands/2
		if start < 0 {
			start = 0
		}
		if start+maxCands > len(d.cands) {
			start = len(d.cands) - maxCands
		}
		end = start + maxCands
	}
	pad := strings.Repeat(" ", indent)
	avail := max(8, inner-indent)
	var lines []string
	if start > 0 {
		lines = append(lines, dimStyle.Render(pad+fitWidth("…", avail)))
	}
	for i := start; i < end; i++ {
		mark, st := "  ", itemStyle
		if i == d.candAt || (d.candAt < 0 && i == start) {
			mark, st = kindIconSel+" ", selectedItemStyle
		}
		lines = append(lines, pad+st.Render(fitWidth(mark+d.cands[i], avail)))
	}
	if end < len(d.cands) {
		lines = append(lines, dimStyle.Render(pad+fitWidth("…", avail)))
	}
	return lines
}

func catalogChartField(c config.Chart) string {
	if len(c.Steps) > 0 {
		return c.Name
	}
	return c.HelmChart()
}

func helmChartReleaseName(chart string) string {
	chart = strings.TrimSpace(chart)
	if i := strings.LastIndex(chart, "/"); i >= 0 {
		return chart[i+1:]
	}
	return chart
}

func (m *model) applyHelmChartVersion(op *helmOp, pinned, chart string) {
	cur := helmFieldValue(m.dlg, "version")
	if cur == "" || op.autoVersion {
		setHelmField(m.dlg, "version", pinned)
		op.autoVersion = true
	}
	if op.chart != chart {
		op.versions = nil
	}
	op.chart = chart
}

func (m *model) maybeFillCatalog() {
	if m.dlg == nil || m.vs.hop == nil || m.vs.hop.act != "install" {
		return
	}
	chart := helmFieldValue(m.dlg, "chart")
	if chart == "" {
		return
	}
	op := *m.vs.hop
	c, ok := config.LookupChart(m.cfg.Charts(), chart)
	if !ok {
		op.catalog = ""
		if op.autoName {
			setHelmField(m.dlg, "name", helmChartReleaseName(chart))
		}
		if op.autoNS {
			ns := m.namespace
			if ns == "" {
				ns = "default"
			}
			setHelmField(m.dlg, "ns", ns)
		}
		if op.autoCreate && !op.manualCreate {
			m.dlg.hcreate = false
			op.autoCreate = false
		}
		if op.autoValues {
			op.values = ""
			setHelmField(m.dlg, "values", "")
		}
		m.applyHelmChartVersion(&op, "", chart)
		m.vs.hop = &op
		return
	}
	op.catalog = c.Name
	if helmFieldValue(m.dlg, "name") == "" || op.autoName {
		setHelmField(m.dlg, "name", c.HelmRelease())
		op.autoName = true
	}
	setHelmField(m.dlg, "chart", catalogChartField(c))
	chart = catalogChartField(c)
	if c.Namespace != "" && (helmFieldValue(m.dlg, "ns") == "" || op.autoNS || helmFieldValue(m.dlg, "ns") == "default" || helmFieldValue(m.dlg, "ns") == m.namespace) {
		setHelmField(m.dlg, "ns", c.Namespace)
		op.autoNS = true
	}
	if !op.manualCreate {
		m.dlg.hcreate = c.CreatesNamespace()
		op.autoCreate = true
	}
	if op.values == "" || op.autoValues {
		if len(c.Values) > 0 && len(c.Steps) == 0 {
			path, err := config.WriteValuesFile(c.Values)
			if err == nil {
				op.values = path
				op.autoValues = true
				setHelmField(m.dlg, "values", path)
			}
		} else if op.autoValues {
			op.values = ""
			setHelmField(m.dlg, "values", "")
		}
	}
	m.applyHelmChartVersion(&op, c.Version, chart)
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

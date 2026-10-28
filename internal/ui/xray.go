package ui

import (
	"strings"
	"time"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type xrayView struct {
	kind, name string
	roots      []*kube.Node
	collapsed  map[string]bool
	seen       map[string]bool
	filter     string
}

func (x xrayView) withDefaults() *xrayView {
	wide := x.name == "" && len(x.roots) > 1
	col := make(map[string]bool, len(x.collapsed)+len(x.roots))
	for k, v := range x.collapsed {
		col[k] = v
	}
	seen := make(map[string]bool, len(x.seen)+len(x.roots))
	for k, v := range x.seen {
		seen[k] = v
	}
	for _, r := range x.roots {
		id := r.ID()
		if seen[id] {
			continue
		}
		seen[id] = true
		if wide && len(r.Children) > 0 && !kube.Trouble(r) {
			col[id] = true
		}
	}
	x.collapsed, x.seen = col, seen
	return &x
}

type xrayMsg struct {
	pane       string
	token      int
	refresh    bool
	kind, name string
	ns         string
	roots      []*kube.Node
	err        error
}

var xrayResource = map[string]string{
	"deploy": "deployments", "rs": "replicasets", "pod": "pods", "svc": "services", "ing": "ingresses",
	"sts": "statefulsets", "ds": "daemonsets", "job": "jobs", "cronjob": "cronjobs",
	"cm": "configmaps", "secret": "secrets", "pvc": "persistentvolumeclaims",
}

func (m model) openXray(kind, name string) (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	root, ok := kube.XrayKind(kind)
	if !ok {
		m.status = "no owner tree for " + kind + " (have " + strings.Join(kube.XrayKindNames, ", ") + ")"
		return m, m.notify(toneWarn, "no owner tree for "+kind)
	}
	spin := m.startLoad("loading tree", true)
	return m, tea.Batch(spin, xrayCmd(m.k, m.logID, m.load.token, false, m.kubeconfig, m.context, m.namespace, root, name))
}

func xrayCmd(k kube.Client, pane string, token int, refresh bool, kc, ctx, ns, kind, name string) tea.Cmd {
	return func() tea.Msg {
		objs, err := k.XrayObjects(kc, ctx, ns)
		msg := xrayMsg{pane: pane, token: token, refresh: refresh, kind: kind, name: name, ns: ns, err: err}
		if err == nil {
			msg.roots = kube.BuildXray(objs, kind, name)
		}
		return msg
	}
}

func (m *model) applyXray(msg xrayMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if msg.refresh {
			delete(m.rowBusy, msg.pane)
			x := m.vs.xray
			if m.stage != "xray" || x == nil || x.kind != msg.kind || x.name != msg.name || msg.ns != m.namespace || msg.err != nil {
				return
			}
			if m.content.FilterState() != list.Unfiltered {
				return
			}
			next := *x
			next.roots = msg.roots
			m.vs.xray = next.withDefaults()
			m.showXray("")
			return
		}
		if m.load.token != msg.token || m.stage != m.load.from {
			return
		}
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "could not read the owner tree")
			return
		}
		if len(msg.roots) == 0 {
			what := msg.kind
			if msg.name != "" {
				what += "/" + msg.name
			}
			m.status = "no " + what + " in " + or(m.namespace, "all namespaces")
			if m.namespace != "" && msg.name == "" {
				m.status += " — press " + m.cfg.Keys.AllNamespaces.Label() + " for all namespaces"
			}
			cmd = m.notify(toneWarn, m.status)
			if m.stage == "xray" && m.vs.xray != nil {

				next := *m.vs.xray
				next.roots = nil
				m.vs.xray = &next
				m.vs.pick = ""
				m.showXray("")
			}
			return
		}
		pick := m.vs.pick
		m.vs.pick = ""
		if len(msg.roots) == 1 {
			pick = msg.roots[0].ID()
		}
		if m.stage != "xray" {
			sel := ""
			if it, ok := m.selectedItem(); ok {
				sel = it.id
			}
			m.vs.xrayFrom, m.vs.xrayKind, m.vs.xraySel = m.stage, m.kind, sel
		}
		m.vs.xray = xrayView{kind: msg.kind, name: msg.name, roots: msg.roots}.withDefaults()
		m.vs.sortCol, m.vs.sortDesc = "", false
		m.content.ResetFilter()
		m.content.SetItems(nil)
		m.stage = "xray"
		m.showXray(pick)
		m.status = m.xrayHint()
	})
	return cmd
}

func (m model) xrayHint() string {
	k := m.cfg.Keys
	return keyName(k.XrayToggle.Short()) + " fold · " + k.Select.Short() + " open · " + k.XrayFilter.Short() + " filter · " +
		k.XrayExpand.Short() + "/" + k.XrayCollapse.Short() + " expand/collapse all · tip: :xray deploy <name> or " + k.Xray.Short() + " on a row"
}

func (m model) xrayFoot() string {
	x := m.vs.xray
	if x != nil && x.filter != "" {
		return m.cfg.Keys.XrayFilter.Short() + x.filter + " · " + m.cfg.Keys.XrayFilter.Short() + " and empty clears · " + m.xrayHint()
	}
	return m.xrayHint()
}

func keyName(k string) string {
	if k == " " {
		return "space"
	}
	return k
}

func (m *model) showXray(pick string) {
	x := m.vs.xray
	if x == nil {
		return
	}
	header, items := xrayItems(x, m.tableWidth(), m.namespace)
	m.content.SetShowTitle(false)
	vs := m.vs
	vs.sortCol = ""
	setRows(&m.content, header, items, vs, pick)
}

func xrayItems(x *xrayView, width int, namespace string) (string, []list.Item) {
	roots, collapsed := x.roots, x.collapsed
	if x.filter != "" {

		roots, collapsed = kube.FilterXray(x.roots, x.filter), nil
	}
	rows := kube.Flatten(roots, collapsed)

	headers := []string{"NAME", "KIND", "STATUS", "READY", "RESTARTS", "AGE", "SUMMARY"}
	cells := make([][]string, len(rows))
	refs := make([][2]string, len(rows))
	born := make([]time.Time, len(rows))
	for i, r := range rows {
		n := r.Node
		mark := ""
		switch {
		case r.Kids && collapsed[n.ID()]:
			mark = "▸ "
		case r.Kids:
			mark = "▾ "
		}
		name := n.Name
		if n.Init {
			name += " (init)"
		}
		age := ""
		if !n.Born.IsZero() {
			age = kube.Age(n.Born)
		}
		summary := ""
		if r.Kids && collapsed[n.ID()] {
			summary = kube.Summary(n)
		}
		cells[i] = []string{r.Prefix + mark + name, n.Label(), n.Status, n.Ready, n.RestartsText(), age, summary}
		refs[i] = [2]string{n.Namespace, n.Name}
		born[i] = n.Born
	}

	anySummary := false
	for _, c := range cells {
		anySummary = anySummary || c[6] != ""
	}
	if !anySummary {
		headers = headers[:6]
		for i := range cells {
			cells[i] = cells[i][:6]
		}
	}

	header, items, _ := packTable(headers, cells, refs, born, 2, width, "x")
	for i, r := range rows {
		it := items[i].(item)
		n := r.Node
		it.id = n.ID()
		it.desc = n.Namespace
		it.extra = map[string]string{"node": n.Kind, "kind": xrayResource[n.Kind], "ns": n.Namespace, "name": n.Name, "pod": n.Pod}
		if n.Kind == "container" {
			it.extra["kind"], it.extra["name"], it.extra["container"] = "pods", n.Pod, n.Name
		}
		items[i] = it
	}
	_ = namespace
	return header, items
}

func (m model) xrayKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	it, sel := m.selectedItem()
	switch {
	case km.XrayToggle.Has(key):
		if !sel || m.vs.xray == nil {
			return m, nil, true
		}
		x := *m.vs.xray
		x.collapsed = make(map[string]bool, len(m.vs.xray.collapsed)+1)
		for k, v := range m.vs.xray.collapsed {
			x.collapsed[k] = v
		}
		if x.collapsed[it.id] {
			delete(x.collapsed, it.id)
		} else {
			x.collapsed[it.id] = true
		}
		m.vs.xray = &x
		m.showXray(it.id)
		return m, nil, true
	case km.XrayExpand.Has(key), km.XrayCollapse.Has(key):
		if m.vs.xray == nil {
			return m, nil, true
		}
		x := *m.vs.xray
		x.collapsed = map[string]bool{}
		if km.XrayCollapse.Has(key) {
			for _, r := range x.roots {
				if len(r.Children) > 0 {
					x.collapsed[r.ID()] = true
				}
			}
		}
		m.vs.xray = &x
		pick := ""
		if sel {
			pick = it.id
			if km.XrayCollapse.Has(key) {
				pick = xrayRootOf(&x, it.id)
			}
		}
		m.showXray(pick)
		return m, nil, true
	case km.XrayFilter.Has(key):
		x := m.vs.xray
		if x == nil {
			return m, nil, true
		}
		m.dlg = m.newDialog("xrayfilter", "⌕ filter the tree", "", m.namespace, "").
			withField(x.filter, "name contains", "matches with the path to them and all they hold; empty shows all")
		return m, nil, true
	case km.Yaml.Has(key):
		if !sel {
			return m, nil, true
		}
		next, cmd := m.yamlOf(it.extra["kind"], it.extra["ns"], it.extra["name"], "xray")
		return next, cmd, true
	case km.MultiLogs.Has(key):
		if !sel {
			return m, nil, true
		}
		next, cmd := m.xrayLogs(it)
		return next, cmd, true
	case km.CopyName.Has(key), km.CopyRef.Has(key):
		if !sel {
			return m, nil, true
		}
		text := it.extra["name"]
		if km.CopyRef.Has(key) {
			text = it.extra["kind"] + "/" + text
			if ns := it.extra["ns"]; ns != "" {
				text += " -n " + ns
			}
		}
		m.status = "copying " + text + "…"
		return m, m.copyCmd(text), true
	}
	return m, nil, false
}

func xrayRootOf(x *xrayView, id string) string {
	var has func(n *kube.Node) bool
	has = func(n *kube.Node) bool {
		if n.ID() == id {
			return true
		}
		for _, c := range n.Children {
			if has(c) {
				return true
			}
		}
		return false
	}
	for _, r := range x.roots {
		if has(r) {
			return r.ID()
		}
	}
	return id
}

func (m model) submitXrayFilter() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.dlg.input.Value())
	m.dlg = nil
	if m.vs.xray == nil {
		return m, nil
	}
	x := *m.vs.xray
	x.filter = text
	m.vs.xray = &x
	m.showXray("")
	switch {
	case text == "":
		m.status = "filter cleared · " + m.xrayHint()
	case len(kube.FilterXray(x.roots, text)) == 0:
		m.status = "nothing named like " + text
		return m, m.notify(toneWarn, m.status)
	default:
		m.status = "filter " + text
	}
	return m, nil
}

func (m model) xrayLogs(it item) (tea.Model, tea.Cmd) {
	var pods []string
	var walk func(n *kube.Node)
	walk = func(n *kube.Node) {
		if n.Kind == "pod" {
			pods = append(pods, n.Name)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if n := m.xrayNode(it.id); n != nil {
		walk(n)
	}
	q := kube.LogQuery{All: m.cfg.Logs.MultiAllContainers}
	if c := it.extra["container"]; c != "" {
		pods = []string{it.extra["pod"]}
		q.Container, q.All = c, false
	}
	if len(pods) == 0 {
		m.status = "no pods under " + it.extra["name"]
		return m, m.notify(toneWarn, m.status)
	}
	q.Regex = podsRegex(pods)
	return m.openMultiLogs(q, it.extra["node"]+"/"+it.extra["name"], "xray")
}

func (m model) xrayNode(id string) *kube.Node {
	if m.vs.xray == nil {
		return nil
	}
	var found *kube.Node
	var walk func(n *kube.Node)
	walk = func(n *kube.Node) {
		if found != nil {
			return
		}
		if n.ID() == id {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range m.vs.xray.roots {
		walk(r)
	}
	return found
}

func (m model) xrayEnter(it item) (tea.Model, tea.Cmd) {
	kind, ns, name := it.extra["kind"], it.extra["ns"], it.extra["name"]
	if c := it.extra["container"]; c != "" {
		m.vs.jumpFrom = "xray"
		return m.openLogs(ns+"/"+name, c, false)
	}
	if kind == "" {
		return m, nil
	}
	pick := name
	if m.namespace == "" && ns != "" {
		pick = ns + "/" + name
	}
	m.vs.jumpFrom, m.vs.jumpSel = "xray", it.id
	m.vs.pick = pick
	if kind == "pods" {
		m.vs.podSel, m.vs.podTitle = "", ""
		return m.openList("workpods", "pods")
	}
	return m.openList("res", kind)
}

func (m model) leaveXray() (tea.Model, tea.Cmd) {
	from, kind, sel := m.vs.xrayFrom, m.vs.xrayKind, m.vs.xraySel
	m.vs.xray = nil
	m.vs.xrayFrom, m.vs.xrayKind, m.vs.xraySel = "", "", ""
	m.vs.jumpFrom, m.vs.jumpSel = "", ""
	if (from == "workpods" || from == "res") && kind != "" {
		m.vs.pick = sel
		return m.openList(from, kind)
	}
	m.stage = "nav"
	m.status = m.context
	return m, nil
}

func (m model) backToXray() (tea.Model, tea.Cmd) {
	x, sel := m.vs.xray, m.vs.jumpSel
	m.vs.jumpFrom, m.vs.jumpSel, m.vs.pick = "", "", ""
	if x == nil {
		m.stage = "nav"
		return m, nil
	}
	m.stage = "xray"
	m.content.ResetFilter()
	m.content.SetItems(nil)
	m.showXray(sel)
	m.status = m.xrayHint()
	return m, xrayCmd(m.k, m.logID, 0, true, m.kubeconfig, m.context, m.namespace, x.kind, x.name)
}

func (m model) xraySelected() (tea.Model, tea.Cmd) {
	kind, _, name, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	if m.stage == "workpods" {
		kind = "pods"
	}
	if _, ok := kube.XrayKind(kind); !ok {
		m.status = "no owner tree for " + kind + " (have " + strings.Join(kube.XrayKindNames, ", ") + ")"
		return m, m.notify(toneWarn, "no owner tree for "+kube.BaseKind(kind))
	}
	return m.openXray(kind, name)
}

func (m model) yamlOf(kind, ns, name, from string) (tea.Model, tea.Cmd) {
	if kind == "" || name == "" {
		return m, nil
	}
	m.subject = name
	if ns != "" {
		m.subject = ns + "/" + name
	}
	m.vs.vback = from
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "yaml"
	m.armWatch(kind, ns, name)
	m.status = m.subject
	pane, kc, ctx, k := m.logID, m.kubeconfig, m.context, m.k
	fetch := func() tea.Msg {
		text, err := k.YAML(kc, ctx, kind, name, ns)
		return yamlMsg{id: pane, text: text, err: err}
	}
	return m, tea.Batch(m.startLoad("loading yaml", false), fetch)
}

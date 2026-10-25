package ui

import (
	"strconv"

	"k2/internal/kube"
	"k2/internal/layout"
	"k2/internal/state"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func readNamespaceCmd(k kube.Client, kubeconfig, context string) tea.Cmd {
	return func() tea.Msg {
		if kubeconfig == "" || context == "" {
			return nsMsg{}
		}
		return nsMsg{ctx: context, ns: k.NamespaceOf(kubeconfig, context)}
	}
}

func (m *model) save() {
	m.commit()
	if m.tab >= 0 && m.tab < len(m.tabs) {
		m.tabs[m.tab].focus = m.logID
	}
	st := state.State{
		Kubeconfig: m.kubeconfig,
		Context:    m.context,
		Namespace:  m.namespace,
		KustomRoot: m.kroot,
		Focus:      m.focus,
		Tab:        m.tab,
		Forwards:   m.forwards,
		Favorites:  m.favs,
	}
	index := map[string]int{}
	for i, p := range m.panes {
		index[p.logID] = i
		stage := p.stage

		// stern keeps the query, not the lines. xray/helm/pulse just come back as home.
		logs, logsTitle := "", ""
		if stage == "mlogs" && p.vs.ml != nil {
			logs = logQueryArgs(p.vs.ml.q)
			logsTitle = p.vs.ml.title
		}
		if stage == "describe" {
			stage = or(p.dback, "res")
		}
		if stage == "yaml" {
			stage = "res"
		}
		if logs == "" {
			switch {
			case p.vs.vback != "", p.vs.jumpFrom != "", stage == "xray", stage == "helm", stage == "pulse", stage == "mlogs",
				stage == "helmhist", stage == "hdiff":
				stage = "nav"
			}
		}
		st.Panes = append(st.Panes, state.Pane{
			Stage: stage, Kind: p.kind, LogPod: p.logPod, Pod: p.pod,
			Kubeconfig: p.kubeconfig, Context: p.context, Namespace: p.namespace,
			NsPrev: p.vs.nsPrev, Logs: logs, LogsTitle: logsTitle,
		})
	}
	for _, t := range m.tabs {
		st.Tabs = append(st.Tabs, state.Tab{Layout: saveNode(t.root, index), Focus: index[t.focus], Name: t.name})
	}
	state.Save(m.cfg.StateDir, st)
}

func saveNode(n *layout.Node, index map[string]int) state.Node {
	switch n.Dir {
	case layout.Row, layout.Col:
		out := state.Node{Split: map[layout.Dir]string{layout.Row: "row", layout.Col: "col"}[n.Dir]}
		for _, k := range n.Kids {
			out.Kids = append(out.Kids, saveNode(k, index))
		}
		return out
	}
	return state.Node{Pane: index[n.Pane]}
}

func loadNode(n state.Node, panes int, used map[int]bool) *layout.Node {
	switch n.Split {
	case "":
		if n.Pane < 0 || n.Pane >= panes || used[n.Pane] || len(n.Kids) > 0 {
			return nil
		}
		used[n.Pane] = true
		return layout.New(strconv.Itoa(n.Pane))
	case "row", "col":
		out := &layout.Node{Dir: layout.Row}
		if n.Split == "col" {
			out.Dir = layout.Col
		}
		for _, k := range n.Kids {
			kid := loadNode(k, panes, used)
			if kid == nil {
				return nil
			}
			out.Kids = append(out.Kids, kid)
		}
		if !out.Valid() {
			return nil
		}
		return out
	}
	return nil
}

func relabel(n *layout.Node, ids map[string]string) {
	if n.Dir == layout.Leaf {
		n.Pane = ids[n.Pane]
		return
	}
	for _, k := range n.Kids {
		relabel(k, ids)
	}
}

func (m *model) savedTabs(st state.State) (roots []*layout.Node, focus []string, ok bool) {
	used := map[int]bool{}
	for i, t := range st.Tabs {
		if i == maxTabs {
			break
		}
		root := loadNode(t.Layout, len(st.Panes), used)
		if root == nil {
			return nil, nil, false
		}
		for ids := root.Leaves(); len(ids) > m.cfg.MaxPanes; ids = root.Leaves() {
			root = root.Remove(ids[len(ids)-1])
		}
		f := strconv.Itoa(t.Focus)
		if !root.Has(f) {
			f = root.Leaves()[0]
		}
		roots, focus = append(roots, root), append(focus, f)
	}
	return roots, focus, len(roots) > 0
}

func (m *model) restorePanes(st state.State) {
	if len(st.Panes) == 0 || st.Kubeconfig == "" || st.Kubeconfig != m.kubeconfig {
		return
	}
	roots, focus, ok := m.savedTabs(st)
	if !ok {
		roots, focus, ok = m.savedTabs(state.Migrate(state.State{Panes: st.Panes, Focus: st.Focus}))
		if !ok {
			return
		}
	}

	ids := map[string]string{}
	var snaps []paneSnap
	for i, sp := range st.Panes {
		key := strconv.Itoa(i)
		for _, r := range roots {
			if r.Has(key) {
				snap := m.hydrate(sp)
				ids[key] = snap.logID
				snaps = append(snaps, snap)
				break
			}
		}
	}
	m.panes = snaps
	m.tabs = nil
	for i, r := range roots {
		relabel(r, ids)
		name := ""
		if i < len(st.Tabs) {
			name = cleanTabName(st.Tabs[i].Name)
		}
		m.tabs = append(m.tabs, tab{root: r, focus: ids[focus[i]], name: name})
	}
	m.tab = min(max(st.Tab, 0), len(m.tabs)-1)
	m.focus = -1
	m.focus = m.paneIdx(m.tabs[m.tab].focus)
	m.restored = true
	m.restore(m.panes[m.focus])
	m.refit()
}

func (m *model) hydrate(sp state.Pane) paneSnap {
	base := freshPane(m.cfg.Keys)
	alt := *m
	alt.content = base.content
	alt.portInput = base.portInput
	alt.logID = base.logID
	alt.logText = ""
	alt.logBusy = false
	alt.load, alt.ran = load{}, runInfo{}
	alt.kind = sp.Kind
	alt.logPod = sp.LogPod
	alt.pod = sp.Pod
	alt.kubeconfig = sp.Kubeconfig
	alt.context = sp.Context
	alt.namespace = sp.Namespace
	if alt.kubeconfig == "" {
		alt.kubeconfig = m.kubeconfig
		alt.context = m.context
		alt.namespace = m.namespace
	}
	alt.vs.nsPrev = sp.NsPrev
	alt.stage = "nav"
	alt.status = alt.context
	kc, ctx, ns := alt.kubeconfig, alt.context, alt.namespace
	fail := func(err error) {
		alt.stage = "nav"
		if err != nil {
			alt.status = m.errLine(err)
		}
	}
	switch sp.Stage {
	case "workpods":
		header, items, err := getPodItems(m.k, kc, ctx, ns, "", m.tableWidth())
		if err != nil || len(items) == 0 {
			fail(err)
			break
		}
		alt.content.Title = header
		alt.content.SetShowTitle(false)
		alt.content.SetItems(items)
		alt.kind = "pods"
		alt.stage = "workpods"
		alt.status = "pods"
	case "res":
		if sp.Kind == "" {
			break
		}
		header, items, err := getResourceItems(m.k, kc, ctx, ns, sp.Kind, m.tableWidth())
		if err != nil || len(items) == 0 {
			fail(err)
			break
		}
		alt.content.Title = header
		alt.content.SetShowTitle(false)
		alt.content.SetItems(items)
		alt.kind = sp.Kind
		alt.stage = "res"
		alt.status = sp.Kind
	case "logs":
		if sp.LogPod == "" {
			break
		}
		alt.stage = "logs"
		alt.logPod = sp.LogPod
		alt.logStick = true
		alt.status = "logs"
	case "mlogs":
		if sp.Logs == "" {
			break
		}
		q, err := kube.ParseLogQuery(sp.Logs, m.cfg.Logs.MultiAllContainers)
		if err != nil {
			fail(err)
			break
		}
		title := sp.LogsTitle
		if title == "" {
			title = q.String()
		}

		alt.vs.ml = &mlogView{q: q, title: title, from: "nav", cur: kube.LogCursor{}, colors: map[string]color{}}
		alt.stage = "mlogs"
		alt.logStick = true
		alt.status = "logs of " + title
	case "fwds":
		if len(m.forwards) == 0 {
			break
		}
		alt.content.Title = "Forwards"
		alt.content.SetItems(m.forwardItems())
		alt.stage = "fwds"
		alt.status = "forwards"
	case "projects":
		projects, err := m.k.Projects(kc, ctx)
		if err != nil {
			fail(err)
			break
		}
		items := []list.Item{item{title: "all namespaces", id: "*"}}
		for _, p := range projects {
			items = append(items, item{title: p, id: p})
		}
		alt.content.Title = "Namespace"
		alt.content.SetItems(items)
		alt.stage = "projects"
	case "contexts":
		contexts, err := m.k.Contexts(kc)
		if err != nil {
			fail(err)
			break
		}
		items := make([]list.Item, len(contexts))
		for i, c := range contexts {
			items[i] = item{title: c}
		}
		alt.content.Title = "Context"
		alt.content.SetItems(items)
		alt.stage = "contexts"
	case "files":
		files, err := getKubeFiles()
		if err != nil {
			fail(err)
			break
		}
		alt.content.Title = "Kubeconfig"
		alt.content.SetItems(files)
		alt.stage = "files"
	}
	return alt.snap()
}

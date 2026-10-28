package ui

import (
	"strconv"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/kustomize"
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
		kind := p.kind
		target := ""
		logs, logsTitle := "", ""
		if stage == "mlogs" && p.vs.ml != nil {
			logs = logQueryArgs(p.vs.ml.q)
			logsTitle = p.vs.ml.title
		}
		switch stage {
		case "describe", "yaml", "view":
			stage = or(p.dback, "res")
			if stage == "" || stage == "describe" || stage == "yaml" || stage == "view" || stage == "nav" {
				stage = "res"
			}
		case "hdiff":
			stage = "helm"
			if p.vs.hist != nil && p.vs.hist.name != "" {
				stage = "helmhist"
				target = p.vs.hist.name
			}
		case "kout":
			if m.kroot != "" {
				stage, target = "kdir", m.kroot
			} else {
				stage = "nav"
			}
		case "helmhist":
			if p.vs.hist != nil {
				target = p.vs.hist.name
			}
			if target == "" {
				stage = "helm"
			}
		case "xray":
			if p.vs.xray != nil {
				kind = p.vs.xray.kind
				target = p.vs.xray.name
			}
		case "kust":
			stage = "kdir"
			target = m.kroot
		case "kdir":
			target = m.kroot
		case "kcmd":
			if p.logID == m.logID {
				target = m.sh.Value()
			}
		}
		if stage == "mlogs" && logs == "" {
			stage = "nav"
		}
		st.Panes = append(st.Panes, state.Pane{
			Stage: stage, Kind: kind, LogPod: p.logPod, Pod: p.pod,
			Kubeconfig: p.kubeconfig, Context: p.context, Namespace: p.namespace,
			NsPrev: p.vs.nsPrev, Logs: logs, LogsTitle: logsTitle,
			HelmAll: p.vs.helmAll, Target: target, PodSel: p.vs.podSel, PodTitle: p.vs.podTitle,
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
	if m.stage == "kcmd" && m.subject != "" {
		m.sh.SetValue(m.subject)
	}
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
		header, items, err := getPodItems(m.k, kc, ctx, ns, sp.PodSel, m.tableWidth())
		if err != nil {
			fail(err)
			break
		}
		alt.content.Title = header
		alt.content.SetShowTitle(false)
		alt.content.SetItems(items)
		alt.kind = "pods"
		alt.stage = "workpods"
		alt.vs.podSel, alt.vs.podTitle = sp.PodSel, sp.PodTitle
		alt.status = "pods"
		if len(items) == 0 {
			alt.status = "no pods in " + or(ns, "all namespaces")
		}
	case "res":
		if sp.Kind == "" {
			break
		}
		header, items, err := getResourceItems(m.k, kc, ctx, ns, sp.Kind, m.tableWidth())
		if err != nil {
			fail(err)
			break
		}
		alt.content.Title = header
		alt.content.SetShowTitle(false)
		alt.content.SetItems(items)
		alt.kind = sp.Kind
		alt.stage = "res"
		alt.status = sp.Kind
		if len(items) == 0 {
			alt.status = "no " + sp.Kind + " in " + or(ns, "all namespaces")
		}
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
	case "pulse":
		if ns == "" {
			break
		}
		alt.stage = "pulse"
		alt.subject = "pulse " + ns
		alt.status = "pulse of " + ns
	case "helm":
		alt.stage = "helm"
		alt.vs.helmAll = sp.HelmAll
		alt.status = "helm"
	case "helmhist":
		if sp.Target == "" {
			alt.stage = "helm"
			alt.vs.helmAll = sp.HelmAll
			break
		}
		alt.stage = "helmhist"
		alt.vs.hist = &helmHist{ns: ns, name: sp.Target}
		alt.status = "history of " + ns + "/" + sp.Target
	case "xray":
		kind := sp.Kind
		if kind == "" {
			kind = "pods"
		}
		alt.kind = kind
		alt.stage = "xray"
		alt.vs.xray = &xrayView{kind: kind, name: sp.Target}
		alt.status = "xray " + kind
		if sp.Target != "" {
			alt.status += "/" + sp.Target
		}
	case "kdir":
		dir := sp.Target
		if dir == "" {
			dir = m.kroot
		}
		if dir == "" {
			break
		}
		entries, err := kustomize.List(dir)
		if err != nil {
			fail(err)
			break
		}
		items := make([]list.Item, 0, len(entries))
		for _, e := range entries {
			desc := e.Kind
			switch e.Kind {
			case "parent":
				desc = ""
			case "file":
				desc = "kust-file"
			case "other":
				desc = "plain-file"
			}
			items = append(items, item{title: e.Name, id: e.Path, desc: desc})
		}
		alt.stage = "kdir"
		alt.content.Title = config.HomePath(dir)
		alt.content.SetShowTitle(true)
		alt.content.SetItems(items)
		alt.status = "kustomize"
	case "kcmd":
		alt.stage = "kcmd"
		alt.subject = sp.Target
		alt.status = "kubectl"
	}
	return alt.snap()
}

func (m *model) resumeViews() []tea.Cmd {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	var cmds []tea.Cmd
	start := func(id, stage, kind, target, kc, ctx, ns string, helmAll bool) {
		if kc == "" || ctx == "" {
			return
		}
		switch stage {
		case "pulse":
			if ns == "" || m.rowBusy["pu"+id] {
				return
			}
			m.rowBusy["pu"+id] = true
			cmds = append(cmds, pulseCmd(m.k, id, 0, true, kc, ctx, ns))
		case "helm":
			if m.rowBusy[id] {
				return
			}
			m.rowBusy[id] = true
			hns := ns
			if helmAll {
				hns = ""
			}
			cmds = append(cmds, helmCmd(m.k, id, 0, true, helmAll, kc, ctx, hns))
		case "helmhist":
			if target == "" || m.rowBusy[id] {
				return
			}
			m.rowBusy[id] = true
			name, pane := target, id
			cmds = append(cmds, func() tea.Msg {
				secrets, err := m.k.ObjectsWhere(kc, ctx, "secrets", ns, "owner=helm,name="+name)
				msg := helmHistMsg{pane: pane, ns: ns, name: name, err: err}
				if err == nil {
					msg.revs = kube.HelmHistory(secrets, ns, name)
				}
				return msg
			})
		case "xray":
			if m.rowBusy[id] {
				return
			}
			m.rowBusy[id] = true
			cmds = append(cmds, xrayCmd(m.k, id, 0, true, kc, ctx, ns, kind, target))
		}
	}
	start(m.logID, m.stage, m.kind, xrayOrHist(m.vs), m.kubeconfig, m.context, m.namespace, m.vs.helmAll)
	for i := range m.panes {
		if i == m.focus {
			continue
		}
		p := m.panes[i]
		start(p.logID, p.stage, p.kind, xrayOrHist(p.vs), p.kubeconfig, p.context, p.namespace, p.vs.helmAll)
	}
	return cmds
}

func xrayOrHist(vs viewState) string {
	if vs.xray != nil {
		return vs.xray.name
	}
	if vs.hist != nil {
		return vs.hist.name
	}
	return ""
}

package ui

import (
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type kindUsageMsg struct {
	usage map[string]kube.KindUsage
}

type kindCatalogMsg struct {
	charts map[string][]string
}

const kindExtrasEvery = 15 * time.Second

type kindSnap struct {
	names []string
	nodes map[string]kube.KindNodes
	kc    map[string]string
	sel   int
}

func (m model) kindVisible() bool {
	if m.stage == "kind" || m.stage == "kindlog" {
		return true
	}
	for i := range m.panes {
		if m.panes[i].stage == "kind" || m.panes[i].stage == "kindlog" {
			return true
		}
	}
	return false
}

func cloneKindNodes(in map[string]kube.KindNodes) map[string]kube.KindNodes {
	if in == nil {
		return nil
	}
	out := make(map[string]kube.KindNodes, len(in))
	for k, v := range in {
		v.Containers = append([]string{}, v.Containers...)
		out[k] = v
	}
	return out
}

func cloneKindKC(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *model) storeKindSnap() {
	m.kindSnap.names = append([]string{}, m.vs.kindNames...)
	m.kindSnap.nodes = cloneKindNodes(m.vs.kindNodes)
	m.kindSnap.sel = m.vs.kindSel
	if m.kindSnap.kc == nil {
		m.kindSnap.kc = map[string]string{}
	}
}

func (m *model) restoreKindSnap() {
	if m.kindSnap.names == nil && m.kindSnap.nodes == nil {
		return
	}
	m.vs.kindNames = append([]string{}, m.kindSnap.names...)
	m.vs.kindNodes = cloneKindNodes(m.kindSnap.nodes)
	if m.kindSnap.sel >= 0 {
		m.vs.kindSel = m.kindSnap.sel
	}
}

func (m model) kindExtrasDue() bool {
	if m.kindExtrasForce {
		return true
	}
	if !m.kindVisible() {
		return false
	}
	if m.kindExtrasAt.IsZero() {
		return true
	}
	return now().Sub(m.kindExtrasAt) >= kindExtrasEvery
}

func (m *model) kindExtrasCmds() tea.Cmd {
	if !m.kindExtrasDue() {
		return nil
	}
	var cmds []tea.Cmd
	if !m.kindUsageBusy {
		m.kindUsageBusy = true
		cmds = append(cmds, m.kindUsageCmd())
	}
	if !m.kindCatalogBusy {
		m.kindCatalogBusy = true
		cmds = append(cmds, m.kindCatalogCmd())
	}
	if !m.kindPodsBusy {
		m.kindPodsBusy = true
		cmds = append(cmds, m.kindPodsCmd())
	}
	if proxy := m.kindProxyCmd(); proxy != nil {
		cmds = append(cmds, proxy)
	}
	if len(cmds) == 0 {
		return nil
	}
	m.kindExtrasAt = now()
	m.kindExtrasForce = false
	return tea.Batch(cmds...)
}

func (m model) kindUsageCmd() tea.Cmd {
	docker := m.dockerCLI()
	run := m.run
	nodes := m.vs.kindNodes
	ids := runningKindIDs(nodes)
	return func() tea.Msg {
		if docker == "" || len(ids) == 0 {
			return kindUsageMsg{usage: map[string]kube.KindUsage{}}
		}
		out, err := shell.Combined(run, docker, kube.DockerStatsArgs(ids)...)
		if err != nil {
			return kindUsageMsg{}
		}
		return kindUsageMsg{usage: kube.UsageByCluster(kube.ParseDockerStats(string(out)), nodes)}
	}
}

func runningKindIDs(nodes map[string]kube.KindNodes) []string {
	var ids []string
	seen := map[string]bool{}
	for _, n := range nodes {
		for _, id := range n.Containers {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *model) applyKindUsage(msg kindUsageMsg) {
	m.kindUsageBusy = false
	if msg.usage == nil {
		return
	}
	if m.kindUsage == nil {
		m.kindUsage = map[string]kube.KindUsage{}
	}
	for name, u := range msg.usage {
		m.kindUsage[name] = u
	}
	for name, n := range m.vs.kindNodes {
		if n.Running == 0 {
			delete(m.kindUsage, name)
		}
	}
}

type kindCatalogTarget struct {
	name, kubeconfig string
}

func (m model) kindCatalogCmd() tea.Cmd {
	k := m.k
	charts := m.cfg.Charts()
	var targets []kindCatalogTarget
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		t := m.kindTileFor(name)
		if t.status != "running" || !t.kcOK {
			return
		}
		seen[name] = true
		targets = append(targets, kindCatalogTarget{name: name, kubeconfig: t.kubeconfig})
	}
	for _, name := range m.vs.kindNames {
		add(name)
	}
	return func() tea.Msg {
		out := map[string][]string{}
		for _, t := range targets {
			secrets, err := k.HelmSecrets(t.kubeconfig, kube.KindContext(t.name), "")
			if err != nil {
				continue
			}
			out[t.name] = installedCatalogNames(charts, secrets)
		}
		return kindCatalogMsg{charts: out}
	}
}

func installedCatalogNames(charts []config.Chart, secrets []kube.Object) []string {
	have := kindReleaseSet(secrets)
	var names []string
	for _, c := range charts {
		if catalogChartOn(have, c) {
			names = append(names, c.Name)
		}
	}
	return names
}

func (m *model) applyKindCatalog(msg kindCatalogMsg) {
	m.kindCatalogBusy = false
	if msg.charts == nil {
		return
	}
	if m.kindCatalog == nil {
		m.kindCatalog = map[string][]string{}
	}
	for name, charts := range msg.charts {
		m.kindCatalog[name] = charts
	}
}

type kindPodsMsg struct {
	pods map[string]kube.KindPods
}

func (m model) kindPodsCmd() tea.Cmd {
	k := m.k
	var targets []kindCatalogTarget
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		t := m.kindTileFor(name)
		if t.status != "running" || !t.kcOK {
			return
		}
		seen[name] = true
		targets = append(targets, kindCatalogTarget{name: name, kubeconfig: t.kubeconfig})
	}
	for _, name := range m.vs.kindNames {
		add(name)
	}
	return func() tea.Msg {
		out := map[string]kube.KindPods{}
		for _, t := range targets {
			raw, err := shell.Combined(k.Run, "kubectl", kube.KubectlPodCountArgs(t.kubeconfig, kube.KindContext(t.name))...)
			if err != nil {
				continue
			}
			out[t.name] = kube.ParsePodCounts(string(raw))
		}
		return kindPodsMsg{pods: out}
	}
}

func (m *model) applyKindPods(msg kindPodsMsg) {
	m.kindPodsBusy = false
	if msg.pods == nil {
		return
	}
	if m.kindPods == nil {
		m.kindPods = map[string]kube.KindPods{}
	}
	keep := map[string]bool{}
	for name, p := range msg.pods {
		m.kindPods[name] = p
		keep[name] = true
	}
	for name := range m.kindPods {
		t := m.kindTileFor(name)
		if t.status != "running" || !t.kcOK || !keep[name] {
			delete(m.kindPods, name)
		}
	}
}

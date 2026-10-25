package ui

import (
	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// xray, helm, stern, pulse. update.go was already a long switch.
func (m *model) screenTick() []tea.Cmd {
	cmds := m.mlogTick()
	if m.beat%max(1, m.cfg.UI.RefreshEvery) != 0 {
		return cmds
	}
	cmds = append(cmds, m.pulseTick()...)
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	shown := map[string]bool{}
	for _, id := range m.tabIDs() {
		shown[id] = true
	}
	try := func(id, stage, kc, ctx, ns string, vs viewState, l load, lst list.Model) {
		if !shown[id] || m.rowBusy[id] || l.active() || kc == "" || ctx == "" || lst.FilterState() != list.Unfiltered {
			return
		}
		switch {
		case stage == "xray" && vs.xray != nil:
			m.rowBusy[id] = true
			cmds = append(cmds, xrayCmd(m.k, id, 0, true, kc, ctx, ns, vs.xray.kind, vs.xray.name))
		case stage == "helm":
			m.rowBusy[id] = true
			hns := ns
			if vs.helmAll {
				hns = ""
			}
			cmds = append(cmds, helmCmd(m.k, id, 0, true, vs.helmAll, kc, ctx, hns))
		}
	}
	try(m.logID, m.stage, m.kubeconfig, m.context, m.namespace, m.vs, m.load, m.content)
	for i := range m.panes {
		if i != m.focus {
			p := m.panes[i]
			try(p.logID, p.stage, p.kubeconfig, p.context, p.namespace, p.vs, p.load, p.content)
		}
	}
	return cmds
}

func (m model) screenMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	switch msg := msg.(type) {
	case xrayMsg:
		cmd := m.applyXray(msg)
		return m, cmd, true
	case helmMsg:
		cmd := m.applyHelm(msg)
		return m, cmd, true
	case helmHistMsg:
		cmd := m.applyHelmHist(msg)
		return m, cmd, true
	case mlogMsg:
		cmd := m.applyMlog(msg)
		return m, cmd, true
	case pulseMsg:
		cmd := m.applyPulse(msg)
		return m, cmd, true
	case selectorMsg:
		next, cmd := m.applySelector(msg)
		return next, cmd, true
	}
	return m, nil, false
}

func (m model) screenKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	table := m.stage == "workpods" || m.stage == "res"
	switch {
	case km.Favorites.Has(key):
		next, cmd := m.openFavorites()
		return next, cmd, true
	case km.AllNamespaces.Has(key) && (table || m.stage == "xray" || m.stage == "mlogs" || m.stage == "pulse"):
		next, cmd := m.toggleAllNS()
		return next, cmd, true
	case table && km.Xray.Has(key):
		next, cmd := m.xraySelected()
		return next, cmd, true
	case table && km.MultiLogs.Has(key):
		next, cmd := m.multiLogsSelected()
		return next, cmd, true
	case m.stage == "xray":
		if km.Select.Has(key) {
			if it, ok := m.selectedItem(); ok {
				next, cmd := m.xrayEnter(it)
				return next, cmd, true
			}
			return m, nil, true
		}
		if km.Describe.Has(key) {
			next, cmd := m.describeSelected()
			return next, cmd, true
		}
		return m.xrayKey(key)
	case m.stage == "helm":
		return m.helmKey(key)
	case m.stage == "helmhist":
		return m.histKey(key)
	case m.viewing() && m.stage != "mlogs" && km.Search.Has(key):
		next, cmd := m.openSearch()
		return next, cmd, true
	case m.stage == "mlogs" && km.LogFilter.Has(key):
		next, cmd := m.openLogFilter()
		return next, cmd, true
	}
	return m, nil, false
}

func (m model) screenBack() (tea.Model, tea.Cmd, bool) {

	if m.vs.jumpFrom == "xray" && m.vs.parent == "" && (m.stage == "workpods" || m.stage == "res" || m.stage == "logs") {
		next, cmd := m.backToXray()
		return next, cmd, true
	}
	if m.vs.vback != "" && (m.stage == "yaml" || m.stage == "view" || m.stage == "describe" || m.stage == "hdiff") {
		back := m.vs.vback
		m.vs.vback = ""
		m.stage = back
		switch back {
		case "xray":
			m.status = m.xrayHint()
		case "helm":
			m.status = m.helmHint()
		case "helmhist":

			m.vs.vback = ""
			m.showHist("")
			m.status = m.histHint()
		}
		return m, nil, true
	}
	if m.stage == "helmhist" {

		pick := ""
		if h := m.vs.hist; h != nil {
			pick = h.ns + "/" + h.name
			if m.helmNS() != "" {
				pick = h.name
			}
		}
		m.stage = "helm"
		m.content.ResetFilter()
		m.content.SetItems(nil)
		m.showHelm(pick)
		m.status = m.helmHint()
		return m, nil, true
	}
	if m.stage == "mlogs" && m.vs.ml != nil {
		from := m.vs.ml.from
		m.vs.ml = nil
		m.logText = ""
		switch from {
		case "xray":
			m.stage = "xray"
			m.status = m.xrayHint()
			return m, nil, true
		case "workpods", "res":

			m.stage = from
			m.status = or(m.kind, "pods")
			if from == "workpods" {
				m.status = m.podsHint()
			}
			return m, nil, true
		}
		m.stage = "nav"
		m.status = m.context
		return m, nil, true
	}
	switch m.stage {
	case "xray", "helm", "pulse":
		m.stage = "nav"
		m.status = m.context
		return m, nil, true
	}
	return m, nil, false
}

func (m *model) helmRefreshNow() {
	secrets, err := m.k.HelmSecrets(m.kubeconfig, m.context, m.helmNS())
	if err != nil {
		m.status = m.errLine(err)
		return
	}
	m.vs.helm = kube.HelmLatest(secrets)
	m.showHelm("")
}

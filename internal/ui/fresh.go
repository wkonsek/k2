package ui

import (
	"k2/internal/layout"

	tea "github.com/charmbracelet/bubbletea"
)

// freshSession drops every tab and pane and leaves the home screen.
// The focused cluster, port-forwards and favorites stay.
func (m model) freshSession() (tea.Model, tea.Cmd) {
	m.killEditors()
	for _, p := range m.panes {
		m.dropVis(p.logID)
	}
	kc, ctx, ns := m.kubeconfig, m.context, m.namespace
	p := freshPane(m.cfg.Keys)
	p.kubeconfig, p.context, p.namespace = kc, ctx, ns
	if ctx != "" {
		p.status = ctx
	} else {
		p.status = "pick an action"
	}
	m.panes = []paneSnap{p}
	m.tabs = []tab{{root: layout.New(p.logID), focus: p.logID}}
	m.tab = 0
	m.focus = 0
	m.restore(p)
	m.zoom = false
	m.prefixOn = false
	m.copy = nil
	m.dlg = nil
	m.pfOn = false
	m.cmdOn = false
	m.stage = "nav"
	m.status = "fresh session"
	m.save()
	return m, m.relayout()
}

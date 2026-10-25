package ui

import (
	"fmt"
	"strings"

	"k2/internal/layout"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const maxTabs = 9

func (m model) newTab() (tea.Model, tea.Cmd) {
	if len(m.tabs) >= maxTabs {
		m.status = fmt.Sprintf("%d tabs max", maxTabs)
		return m, m.notify(toneWarn, m.status)
	}
	m.commit()
	m.tabs[m.tab].focus = m.logID
	p := freshPane(m.cfg.Keys)
	p.kubeconfig, p.context, p.namespace = m.kubeconfig, m.context, m.namespace
	m.panes = append(m.panes, p)
	m.tabs = append(m.tabs, tab{root: layout.New(p.logID), focus: p.logID})
	m.tab = len(m.tabs) - 1
	m.focus = len(m.panes) - 1
	m.restore(p)
	m.refit()
	m.status = fmt.Sprintf("tab %d/%d", m.tab+1, len(m.tabs))
	return m, nil
}

func (m *model) switchTab(t int) {
	if t < 0 || t >= len(m.tabs) {
		m.status = fmt.Sprintf("no tab %d", t+1)
		return
	}
	if t != m.tab {
		m.commit()
		m.tabs[m.tab].focus = m.logID
		m.tab = t
		id := m.tabs[t].focus
		if !m.tabs[t].root.Has(id) {
			id = m.tabs[t].root.Leaves()[0]
		}
		m.focus = -1
		m.focus = m.paneIdx(id)
		m.restore(m.panes[m.focus])
		m.refit()
	}
	m.status = fmt.Sprintf("tab %d/%d", m.tab+1, len(m.tabs))
}

func (m *model) stepTab(step int) {
	n := len(m.tabs)
	m.switchTab((m.tab + step + n) % n)
}

func (m *model) closeTab() {
	if len(m.tabs) <= 1 {
		m.status = "last tab"
		return
	}
	m.commit()
	gone := map[string]bool{}
	for _, id := range m.tabs[m.tab].root.Leaves() {
		gone[id] = true
		if e := m.paneEditor(id); e.live() {
			e.t.Kill()
		}
	}
	kept := m.panes[:0]
	for _, p := range m.panes {
		if !gone[p.logID] {
			kept = append(kept, p)
		}
	}
	m.panes = kept
	m.tabs = append(m.tabs[:m.tab], m.tabs[m.tab+1:]...)
	t := min(m.tab, len(m.tabs)-1)
	m.tab = t
	id := m.tabs[t].focus
	m.focus = -1
	m.focus = m.paneIdx(id)
	if m.focus < 0 {
		m.focus = m.paneIdx(m.tabs[t].root.Leaves()[0])
	}
	m.restore(m.panes[m.focus])
	m.refit()
	m.status = fmt.Sprintf("tab %d/%d", m.tab+1, len(m.tabs))
}

func (m model) showTabBar() bool {
	switch m.cfg.UI.Tabbar {
	case "always":
		return true
	case "never":
		return false
	}
	return len(m.tabs) > 1
}

func (m model) tabBarRows() int {
	if m.showTabBar() {
		return 1
	}
	return 0
}

func (m model) withTabBar(view string) string {
	if !m.showTabBar() {
		return view
	}
	return m.tabBar(max(1, m.width)) + "\n" + view
}

func (m model) tabSnap(t int) paneSnap {
	if t == m.tab {
		return m.snap()
	}
	if i := m.paneIdx(m.tabs[t].focus); i >= 0 {
		return m.panes[i]
	}
	return paneSnap{}
}

func (m model) tabHot(t int) bool {
	for _, id := range m.tabs[t].root.Leaves() {
		ctx := m.context
		if id != m.logID {
			if i := m.paneIdx(id); i >= 0 {
				ctx = m.panes[i].context
			}
		}
		if m.cfg.IsProd(ctx) {
			return true
		}
	}
	return false
}

func (m model) tabLabel(t int) string { return m.tabLabelMax(t, 0) }

func (m model) tabAutoName(t int) string {
	s := m.tabSnap(t)
	return or(s.context, or(s.namespace, "home"))
}

func (m model) tabName(t int) string {
	return or(m.tabs[t].name, m.tabAutoName(t))
}

func (m model) tabLabelMax(t, limit int) string {
	name := m.tabName(t)
	if limit > 0 && ansi.StringWidth(name) > limit {
		name = ansi.Truncate(name, limit, "…")
	}
	if m.tabHot(t) {
		return fmt.Sprintf("%d %s %s", t+1, skull, name)
	}
	return fmt.Sprintf("%d %s", t+1, name)
}

const maxTabName = 40

func cleanTabName(s string) string {
	s = strings.Join(strings.Fields(ansi.Strip(sanitizeLine(s))), " ")
	if r := []rune(s); len(r) > maxTabName {
		s = string(r[:maxTabName])
	}
	return s
}

func (m model) openRenameTab() model {
	d := m.newDialog("rename_tab", fmt.Sprintf("rename tab %d", m.tab+1), "", "", "")
	d.where = "automatic name: " + m.tabAutoName(m.tab)
	d = d.withField(m.tabName(m.tab), m.tabAutoName(m.tab), "enter saves · empty = automatic name · esc cancels")
	d.input.CharLimit = maxTabName
	m.dlg = d
	return m
}

func (m model) submitRenameTab() (tea.Model, tea.Cmd) {
	name := cleanTabName(m.dlg.input.Value())
	m.dlg = nil
	if name == m.tabAutoName(m.tab) && m.tabs[m.tab].name == "" {
		name = ""
	}
	m.tabs[m.tab].name = name
	if name == "" {
		m.status = fmt.Sprintf("tab %d: automatic name", m.tab+1)
	} else {
		m.status = fmt.Sprintf("tab %d renamed to %s", m.tab+1, name)
	}
	m.save()
	return m, nil
}

func (m model) tabBar(width int) string {

	limit := 0
	for l := 24; l >= 4 && m.tabBarWidth(limit) > width; l-- {
		limit = l
	}
	var parts []string
	for t := range m.tabs {
		label := " " + m.tabLabelMax(t, limit) + " "
		hot := m.tabHot(t)
		var st lipgloss.Style
		switch {
		case t == m.tab && hot:
			st = lipgloss.NewStyle().Foreground(th.Bg).Background(th.Danger).Bold(true)
		case t == m.tab:
			st = selectedItemStyle
		case hot:
			st = lipgloss.NewStyle().Foreground(th.Danger)
		default:
			st = dimStyle
		}
		parts = append(parts, st.Render(label))
	}
	line := " " + strings.Join(parts, " ")
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width-1, "…")
	}
	return line
}

func (m model) tabBarWidth(limit int) int {
	w := 1
	for t := range m.tabs {
		w += ansi.StringWidth(m.tabLabelMax(t, limit)) + 3
	}
	return w - 1
}

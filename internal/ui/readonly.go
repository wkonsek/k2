package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) readOnlyBlock(what string) (tea.Model, tea.Cmd, bool) {
	if !m.readOnly {
		return m, nil, false
	}
	msg := "view-only: " + what + " blocked"
	m.status = msg
	return m, m.notify(toneWarn, msg), true
}

func (m model) toggleReadOnly() (tea.Model, tea.Cmd) {
	m.readOnly = !m.readOnly
	if m.readOnly {
		if m.dlg != nil && (m.dlg.ask != "" || m.dlg.pending != nil || m.dlg.onYes != nil) {
			m.dlg = nil
		}
		m.status = "view-only on · " + m.cfg.Keys.ReadOnly.Label() + " to leave"
		return m, m.notify(toneWarn, "view-only on")
	}
	m.status = "view-only off"
	return m, m.notify(toneOK, "view-only off")
}

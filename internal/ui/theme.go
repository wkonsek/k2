package ui

import (
	"strings"

	"k2/internal/config"
	"k2/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) openTheme() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.context) == "" {
		m.status = "select a context first"
		return m, m.notify(toneWarn, m.status)
	}
	names := theme.Names()
	if m.cfg.Custom.Name == "custom" {
		names = append(names, "custom")
	}
	cur := m.cfg.ThemeFor(m.kubeconfig, m.context)
	d := m.newDialog("theme", "theme", "", "", "")
	where := m.context
	if home := config.HomePath(m.kubeconfig); home != "" && home != "no kubeconfig" {
		where += " · " + home
	}
	d.where = where
	d.details = []string{"saved to ui.context_themes"}
	for _, name := range names {
		d.options = append(d.options, name)
		if name == cur {
			d.idx = len(d.options) - 1
		}
	}
	m.dlg = d
	return m, nil
}

func (m model) applyThemePick(name string) (tea.Model, tea.Cmd) {
	m.dlg = nil
	name = strings.TrimSpace(name)
	if name == "" {
		m.status = "cancelled"
		return m, nil
	}
	next, err := config.WriteContextTheme(config.FilePath(), m.cfg.ContextThemes, m.context, m.kubeconfig, name)
	m.cfg.ContextThemes = next
	m.syncTheme()
	if err != nil {
		m.status = "theme " + name + " · not saved"
		return m, m.notify(toneWarn, m.status+" ("+oneLine(err.Error())+")")
	}
	m.status = "theme " + name
	return m, m.notify(toneOK, "theme "+name+" · "+m.context)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:79] + "…"
	}
	return s
}

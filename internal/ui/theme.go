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

func (m *model) previewTheme() {
	if m.dlg != nil && m.dlg.act == "theme" {
		m.retheme(m.cfg.PaletteNamed(m.dlg.picked()))
	}
}

func (m model) bgPaint(light bool) bool {
	if m.dlg != nil && m.dlg.act == "theme" && m.dlg.bg != "" {
		return m.dlg.bg == "theme"
	}
	return m.cfg.UI.PaintBackground(light)
}

func (m model) bgMode() string {
	if m.paintBg {
		return "theme"
	}
	return "transparent"
}

func (m *model) toggleBackground() {
	if m.dlg == nil || m.dlg.act != "theme" {
		return
	}
	m.dlg.bg = "theme"
	if m.paintBg {
		m.dlg.bg = "transparent"
	}
	m.paintBg = m.dlg.bg == "theme"
}

func (m model) applyThemePick(name, bg string) (tea.Model, tea.Cmd) {
	m.dlg = nil
	name = strings.TrimSpace(name)
	if name == "" {
		m.status = "cancelled"
		return m, nil
	}
	next, err := config.WriteThemeChoice(config.FilePath(), m.cfg.ContextThemes, m.context, m.kubeconfig, name, bg)
	m.cfg.ContextThemes = next
	if bg != "" {
		m.cfg.UI.Background, m.cfg.UI.BackgroundSet = bg, true
	}
	m.syncTheme()
	if err != nil {
		m.status = "theme " + name + " · not saved"
		return m, m.notify(toneWarn, m.status+" ("+oneLine(err.Error())+")")
	}
	m.status = "theme " + name
	note := "theme " + name + " · " + m.context
	if bg != "" {
		note += " · background " + bg
	}
	return m, m.notify(toneOK, note)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:79] + "…"
	}
	return s
}

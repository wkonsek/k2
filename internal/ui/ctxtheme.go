package ui

import (
	"k2/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) syncTheme() {
	name := m.cfg.ThemeFor(m.kubeconfig, m.context)
	if name == m.themeName {
		return
	}
	pal := m.cfg.PaletteNamed(name)
	m.retheme(pal)
	m.themeName = name
}

func (m *model) retheme(pal theme.Palette) {
	useTheme(pal)
	m.themeName = pal.Name
	m.paintBg = m.cfg.UI.PaintBackground(pal.Light)
	for _, in := range []*struct{ p, t *lipgloss.Style }{
		{&m.portInput.PromptStyle, &m.portInput.TextStyle},
		{&m.cmd.PromptStyle, &m.cmd.TextStyle},
		{&m.sh.PromptStyle, &m.sh.TextStyle},
		{&m.jq.PromptStyle, &m.jq.TextStyle},
	} {
		*in.p, *in.t = keyStyle, itemStyle
	}
	m.content.Styles.Title = titleStyle
	for i := range m.panes {
		m.panes[i].content.Styles.Title = titleStyle
		m.panes[i].portInput.PromptStyle, m.panes[i].portInput.TextStyle = keyStyle, itemStyle
	}
	m.spin.Style = lipgloss.NewStyle().Foreground(th.Accent)
	clear(m.mlogStyles)
	clear(m.mlogPaint)
	m.fitContent(0, 0)
}

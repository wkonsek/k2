package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) openSearch() (tea.Model, tea.Cmd) {
	m.dlg = m.newDialog("search", "⌕ search", "", m.namespace, "").
		withField(m.vs.search, "text", "the next line containing it; again for the next one")
	return m, nil
}

func (m model) submitSearch() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.dlg.input.Value())
	m.dlg = nil
	if text == "" {
		m.vs.search = ""
		m.status = "search cleared"
		return m, nil
	}
	again := strings.EqualFold(text, m.vs.search)
	m.vs.search = text
	lines := m.visualLines(m.viewerWidth())
	low := strings.ToLower(text)
	var hits []int
	for i, l := range lines {
		if strings.Contains(strings.ToLower(ansi.Strip(l.text)), low) {
			hits = append(hits, i)
		}
	}
	if len(hits) == 0 {
		m.status = "not found: " + text
		return m, m.notify(toneWarn, m.status)
	}

	from := m.viewTop(len(lines), m.textRows())
	if again {
		from = m.vs.searchAt + 1
	}
	at := 0
	for i, h := range hits {
		if h >= from {
			at = i
			break
		}
	}
	m.vs.searchAt = hits[at]
	m.logStick = false
	m.logAt = hits[at]
	if maxTop := len(lines) - m.textRows(); m.logAt > maxTop && maxTop > 0 {
		m.logAt = maxTop
	}
	m.pinLog(lines, m.logAt)
	m.status = fmt.Sprintf("%s: match %d of %d · %s again for the next", text, at+1, len(hits), m.cfg.Keys.Search.Short())
	return m, nil
}

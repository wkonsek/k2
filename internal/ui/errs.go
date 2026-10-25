package ui

import (
	"strings"
	"time"

	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type lastErr struct {
	text string
	ctx  string
	at   time.Time
}

func (m *model) errLine(err error) string {
	if err == nil {
		return ""
	}
	m.noteErr(err.Error())
	return shell.OneLine(err.Error())
}

func (m *model) noteErr(full string) {
	full = strings.TrimSpace(sanitizeText(full))
	if full == "" {
		return
	}
	m.lastErr = lastErr{text: full, ctx: m.context, at: now()}
}

func (m model) freshErr() bool {
	return m.lastErr.text != "" && now().Sub(m.lastErr.at) < 2*time.Second
}

func (m model) openLastErr() (tea.Model, tea.Cmd) {
	if m.lastErr.text == "" {
		m.status = "no error so far"
		return m, m.notify(toneOK, "no error so far")
	}
	d := m.newDialog("error", "✗ last error", "", "", "")
	d.where = or(m.lastErr.ctx, "no context") + " · " + elapsed(now().Sub(m.lastErr.at)) + " ago"
	d.details = strings.Split(m.lastErr.text, "\n")
	d.danger = true
	m.dlg = d
	return m, nil
}

func (m model) errorKey(key string) (tea.Model, tea.Cmd) {
	km := m.cfg.Keys
	switch {
	case km.CopyYank.Has(key) || km.CopyAll.Has(key):
		m.dlg = nil
		m.status = "copying the error…"
		return m, m.copyCmdAs(m.lastErr.text, "the error")
	case km.Cancel.Has(key) || km.Back.Has(key) || km.Submit.Has(key) || km.LastError.Has(key):
		m.dlg = nil
		return m, nil
	}
	return m, nil
}

func moreRows(rows int) string {
	return "… " + plural(rows, "more row", "more rows") + " · y copies all"
}

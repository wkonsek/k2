package ui

import (
	"fmt"
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
	const max = 256 * 1024
	if len(full) > max {
		full = full[:max] + "\n… truncated"
	}
	e := lastErr{text: full, ctx: m.context, at: now()}
	m.lastErr = e
	m.errLog = append(m.errLog, e)
	if len(m.errLog) > 40 {
		m.errLog = append([]lastErr(nil), m.errLog[len(m.errLog)-40:]...)
	}
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
	d.errAt = len(m.errLog) - 1
	m.dlg = d
	m = m.fillErr()
	return m, nil
}

func (m model) fillErr() model {
	d := m.dlg
	if d == nil || len(m.errLog) == 0 {
		return m
	}
	if d.errAt < 0 {
		d.errAt = 0
	}
	if d.errAt >= len(m.errLog) {
		d.errAt = len(m.errLog) - 1
	}
	e := m.errLog[d.errAt]
	d.where = or(e.ctx, "no context") + " · " + elapsed(now().Sub(e.at)) + " ago · " + fmt.Sprintf("%d/%d", d.errAt+1, len(m.errLog))
	d.details = strings.Split(e.text, "\n")
	d.idx = 0
	d.danger = true
	return m
}

func (m model) currentErr() lastErr {
	if m.dlg != nil && m.dlg.errAt >= 0 && m.dlg.errAt < len(m.errLog) {
		return m.errLog[m.dlg.errAt]
	}
	return m.lastErr
}

func (m model) errBody(all bool) string {
	if !all {
		return m.currentErr().text
	}
	var b strings.Builder
	for i, e := range m.errLog {
		fmt.Fprintf(&b, "── %d/%d · %s · %s ago ──\n%s\n\n", i+1, len(m.errLog), or(e.ctx, "no context"), elapsed(now().Sub(e.at)), e.text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) openErrLog(all bool) (tea.Model, tea.Cmd) {
	text := m.errBody(all)
	if strings.TrimSpace(text) == "" {
		m.status = "no error so far"
		return m, nil
	}
	back := m.stage
	if back == "" || back == "view" {
		back = or(m.vs.vback, "nav")
	}
	m.dlg = nil
	m.vs.vback = back
	m.subject = "errors"
	if !all {
		m.subject = "error"
	}
	m.logText = text
	m.logStick = false
	m.logAt = 0
	m.clearLogHold()
	m.stage = "view"
	m.status = m.subject + " · " + m.cfg.Keys.Back.Label() + " back"
	return m, nil
}

func (m model) errorKey(key string) (tea.Model, tea.Cmd) {
	km := m.cfg.Keys
	d := m.dlg
	switch {
	case km.CopyYank.Has(key) || km.CopyAll.Has(key):
		text := m.currentErr().text
		m.dlg = nil
		m.status = "copying the error…"
		return m, m.copyCmdAs(text, "the error")
	case km.Submit.Has(key):
		return m.openErrLog(false)
	case key == "a":
		return m.openErrLog(true)
	case key == "[":
		d.errAt--
		return m.fillErr(), nil
	case key == "]":
		d.errAt++
		return m.fillErr(), nil
	case km.ScrollDown.Has(key) || km.PageDown.Has(key):
		d.idx++
		return m, nil
	case km.ScrollUp.Has(key) || km.PageUp.Has(key):
		if d.idx > 0 {
			d.idx--
		}
		return m, nil
	case km.Top.Has(key):
		d.idx = 0
		return m, nil
	case km.Bottom.Has(key):
		d.idx = 1 << 20
		return m, nil
	case km.Cancel.Has(key) || km.Back.Has(key) || km.LastError.Has(key):
		m.dlg = nil
		return m, nil
	}
	return m, nil
}

func moreRows(rows int) string {
	return "… " + plural(rows, "more row", "more rows") + " · y copies all"
}

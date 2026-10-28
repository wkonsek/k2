package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type copyState struct {
	cur    int
	top    int
	anchor int
}

type copyRow struct {
	text string
	src  int
}

func (m model) copyRows(width int) []copyRow {
	lines := m.textLines()
	out := make([]copyRow, 0, len(lines))
	for i, l := range lines {
		l = ansi.Strip(sanitizeLine(l))
		if !m.wrapsLine(l) {
			out = append(out, copyRow{text: ansi.Truncate(l, width, ""), src: i})
			continue
		}
		for _, r := range wrapLine(l, width) {
			out = append(out, copyRow{text: r, src: i})
		}
	}
	return out
}

func (m model) copyHeight() int {
	if m.cfg.UI.CopyModeHint {
		return max(1, m.height-1)
	}
	return max(1, m.height)
}

func (m model) enterCopy() (tea.Model, tea.Cmd) {
	rows := m.copyRows(m.width)

	vis := m.visualLines(m.viewerWidth())
	topLine := 0
	if len(vis) > 0 {
		at := m.viewTop(len(vis), m.textRows())
		for at > 0 && vis[at].num < 0 {
			at--
		}
		topLine = max(vis[at].num, 0)
	}
	cs := &copyState{anchor: -1}
	for i, r := range rows {
		if r.src == topLine {
			cs.cur, cs.top = i, i
			break
		}
	}
	m.copy = cs
	m.clampCopy(len(rows))
	return m, tea.DisableMouse
}

func (m *model) clampCopy(total int) {
	cs := *m.copy
	h := m.copyHeight()
	cs.cur = max(0, min(cs.cur, total-1))
	if cs.cur < cs.top {
		cs.top = cs.cur
	}
	if cs.cur >= cs.top+h {
		cs.top = cs.cur - h + 1
	}
	cs.top = max(0, min(cs.top, max(0, total-h)))
	if cs.anchor >= total {
		cs.anchor = total - 1
	}
	m.copy = &cs
}

func (m model) copyKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km := m.cfg.Keys
	key := msg.String()
	rows := m.copyRows(m.width)
	cs := *m.copy
	h := m.copyHeight()
	switch {
	case km.Back.Has(key):
		m.copy = nil
		m.shellYank = nil
		return m, m.mouseForShells()
	case km.Quit.Has(key) && key == "ctrl+c":
		m.copy = nil
		m.shellYank = nil
		m.killEditors()
		m.quitting = true
		return m, tea.Quit
	case km.ScrollDown.Has(key):
		cs.cur++
	case km.ScrollUp.Has(key):
		cs.cur--
	case km.PageDown.Has(key):
		cs.cur += h - 1
	case km.PageUp.Has(key):
		cs.cur -= h - 1
	case km.Top.Has(key):
		cs.cur = 0
	case km.Bottom.Has(key):
		cs.cur = len(rows) - 1
	case km.Wrap.Has(key):

		line := 0
		if cs.cur < len(rows) {
			line = rows[cs.cur].src
		}
		anchorLine := -1
		if cs.anchor >= 0 && cs.anchor < len(rows) {
			anchorLine = rows[cs.anchor].src
		}
		m.vs.wrapFlip = !m.vs.wrapFlip
		rows = m.copyRows(m.width)
		cs.cur, cs.anchor = firstRow(rows, line), -1
		if anchorLine >= 0 {
			cs.anchor = firstRow(rows, anchorLine)
		}
	case km.CopySelect.Has(key):
		if cs.anchor >= 0 {
			cs.anchor = -1
		} else {
			cs.anchor = cs.cur
		}
	case km.CopyYank.Has(key):
		if len(rows) == 0 {
			return m, nil
		}
		a, b := cs.cur, cs.cur
		if cs.anchor >= 0 {
			a, b = min(cs.anchor, cs.cur), max(cs.anchor, cs.cur)
		}
		text, n := m.copyText(rows[a].src, rows[b].src)
		cs.anchor = -1
		m.copy = &cs
		return m, m.copyCmdAs(text, plural(n, "line", "lines"))
	case km.CopyAll.Has(key):
		lines := m.textLines()
		if len(lines) == 0 {
			return m, nil
		}
		text, n := m.copyText(0, len(lines)-1)
		return m, m.copyCmdAs(text, "all "+plural(n, "line", "lines"))
	}
	m.copy = &cs
	m.clampCopy(len(rows))
	return m, nil
}

func firstRow(rows []copyRow, src int) int {
	for i, r := range rows {
		if r.src >= src {
			return i
		}
	}
	return max(0, len(rows)-1)
}

func (m model) copyText(a, b int) (string, int) {
	lines := m.textLines()
	a, b = max(0, a), min(b, len(lines)-1)
	out := make([]string, 0, b-a+1)
	for _, l := range lines[a : b+1] {
		out = append(out, strings.TrimRight(ansi.Strip(sanitizeLine(l)), " "))
	}
	return strings.Join(out, "\n"), len(out)
}

func (m model) copyView() string {
	rows := m.copyRows(m.width)
	cs := m.copy
	h := m.copyHeight()
	sel := lipgloss.NewStyle().Reverse(true)
	cursor := lipgloss.NewStyle().Underline(true)
	a, b := -1, -1
	if cs.anchor >= 0 {
		a, b = min(cs.anchor, cs.cur), max(cs.anchor, cs.cur)
	}
	var out []string
	for i := cs.top; i < min(len(rows), cs.top+h); i++ {
		t := rows[i].text
		switch {
		case i >= a && i <= b && a >= 0:
			t = sel.Render(t + " ")
		case i == cs.cur:
			t = cursor.Render(t + " ")
		}
		out = append(out, t)
	}
	for len(out) < h {
		out = append(out, "")
	}
	if m.cfg.UI.CopyModeHint {
		out = append(out, m.copyHint())
	}
	return strings.Join(out, "\n")
}

func (m model) copyHint() string {
	k := m.cfg.Keys
	hint := fmt.Sprintf("COPY · %s select · %s yank · %s all · %s wrap · %s/%s exit",
		k.CopySelect.Short(), k.CopyYank.Short(), k.CopyAll.Short(), k.Wrap.Short(), k.Back.Short(), "esc")
	if m.copy != nil && m.copy.anchor >= 0 {
		hint = "COPY · selecting · " + k.CopyYank.Short() + " yank · " + k.CopySelect.Short() + " cancel"
	}
	if strings.HasPrefix(m.status, "copied ") || strings.HasPrefix(m.status, "copy failed") {
		hint += "    " + m.status
	}
	return dimStyle.Render(ansi.Truncate(hint, m.width, "…"))
}

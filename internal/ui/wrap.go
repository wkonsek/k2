package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type vline struct {
	text string
	num  int
}

func (m model) wrapping() bool { return m.cfg.UI.Wrap != m.vs.wrapFlip }

func (m model) wrapNote() string {
	state := "off"
	if m.wrapping() {
		state = "on"
	}
	return m.cfg.Keys.Wrap.Short() + " wrap " + state
}

func logGutter(lines int) int { return max(3, len(strconv.Itoa(lines))) }

func logRoom(innerW, lines int) int { return max(8, innerW-logGutter(lines)-1) }

func (m model) viewerWidth() int {
	w, _ := m.paneBox(m.logID)
	return max(w-4, 4)
}

func (m model) visualLines(innerW int) []vline {
	logical := m.textLines()
	colored := logical
	switch m.stage {
	case "describe":
		colored = colorDescribe(logical)
	case "yaml":
		colored = make([]string, len(logical))
		for i, l := range logical {
			colored[i] = colorYAML(l)
		}
	case "kout":
		colored = make([]string, len(logical))
		for i, l := range logical {
			colored[i] = colorOut(l, m.ran.mode)
		}
	case "mlogs":
		colored = make([]string, len(logical))
		for i, l := range logical {
			colored[i] = m.colorMlog(l)
		}
	case "pulse":
		colored = make([]string, len(logical))
		for i, l := range logical {
			colored[i] = colorPulse(l)
		}
	case "hdiff":
		colored = make([]string, len(logical))
		for i, l := range logical {
			colored[i] = colorHelmDiff(l)
		}
	}
	width := innerW
	if m.stage == "logs" {
		width = logRoom(innerW, len(logical))
	}
	out := make([]vline, 0, len(colored))
	for i, l := range colored {
		if !m.wrapsLine(logical[i]) {
			out = append(out, vline{text: l, num: i})
			continue
		}
		for j, row := range wrapLine(l, width) {
			n := i
			if j > 0 {
				n = -1
			}
			out = append(out, vline{text: row, num: n})
		}
	}
	if m.stage == "logs" && len(m.vs.logMarks) > 0 {
		out = insertLogMarks(out, m.vs.logMarks)
	}
	if m.stage == "mlogs" && m.vs.ml != nil {
		if marks := m.vs.ml.renderedMarks(); len(marks) > 0 {
			out = insertLogMarks(out, marks)
		}
	}
	return out
}

func wrapLine(line string, width int) []string {
	line = sanitizeLine(line)
	width = max(width, 2)
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}
	plain := ansi.Strip(line)
	indent := len(plain) - len(strings.TrimLeft(plain, " "))
	indent = min(indent, width/2)
	pad := strings.Repeat(" ", indent)
	var rows []string
	rest := line
	for first := true; ; first = false {
		lim := width
		if !first {
			lim = width - indent
		}
		if ansi.StringWidth(rest) <= lim {
			if !first {
				rest = pad + rest
			}
			rows = append(rows, rest)
			return rows
		}
		cut := breakAt(ansi.Strip(rest), lim)
		row := ansi.Truncate(rest, cut, "")
		rest = ansi.TruncateLeft(rest, cut, "")

		if trimmed := strings.TrimLeft(ansi.Strip(rest), " "); trimmed != ansi.Strip(rest) {
			rest = ansi.TruncateLeft(rest, ansi.StringWidth(ansi.Strip(rest))-ansi.StringWidth(trimmed), "")
		}
		if !first {
			row = pad + row
		}
		rows = append(rows, strings.TrimRight(row, " "))
		if ansi.StringWidth(ansi.Strip(rest)) == 0 {
			return rows
		}
	}
}

func breakAt(plain string, lim int) int {
	cells, lastSpace := 0, -1
	for _, r := range plain {
		w := ansi.StringWidth(string(r))
		if cells+w > lim {
			break
		}
		cells += w
		if r == ' ' {
			lastSpace = cells
		}
	}
	if lastSpace > lim/2 {
		return lastSpace
	}
	return max(cells, 1)
}

func (m *model) toggleWrap() {
	w := m.viewerWidth()
	before := m.visualLines(w)
	topLine := 0
	if !m.logStick && len(before) > 0 {
		at := m.viewTop(len(before), m.textRows())
		for at > 0 && before[at].num < 0 {
			at--
		}
		topLine = max(before[at].num, 0)
	}
	m.vs.wrapFlip = !m.vs.wrapFlip
	state := "off"
	if m.wrapping() {
		state = "on"
	}
	m.status = "wrap " + state
	if m.logStick {
		return
	}
	for i, l := range m.visualLines(w) {
		if l.num == topLine {
			m.logAt = i
			return
		}
	}
	m.logAt = 0
}

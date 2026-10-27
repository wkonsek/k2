package ui

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

type vline struct {
	text string
	num  int
}

// visCache is one pane's wrapped rows. View, scroll and unfocused panes all
// call visualLines; without this every frame rewalks the whole buffer.
// Keyed by logID on the model (the map is shared across snap copies).
type visCache struct {
	text  string
	w     int
	wrap  bool
	stage string
	theme string
	mode  string
	extra string
	marks []int
	lines []vline
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

func (m model) visExtra() string {
	if m.stage == "mlogs" && m.vs.ml != nil {
		return m.vs.ml.filter
	}
	return ""
}

func (m model) visMarks() []int {
	switch m.stage {
	case "logs":
		return m.vs.logMarks
	case "mlogs":
		if m.vs.ml != nil {
			return m.vs.ml.marks
		}
	}
	return nil
}

func (m model) visualLines(innerW int) []vline {
	if m.vis == nil {
		return m.buildVisual(innerW)
	}
	wrap := m.wrapping()
	marks := m.visMarks()
	extra := m.visExtra()
	if c := m.vis[m.logID]; c != nil &&
		c.w == innerW && c.wrap == wrap && c.stage == m.stage &&
		c.theme == m.themeName && c.mode == m.ran.mode && c.extra == extra &&
		c.text == m.logText && slices.Equal(c.marks, marks) {
		return c.lines
	}
	lines := m.buildVisual(innerW)
	m.vis[m.logID] = &visCache{
		text: m.logText, w: innerW, wrap: wrap, stage: m.stage,
		theme: m.themeName, mode: m.ran.mode, extra: extra,
		marks: cloneMarks(marks), lines: lines,
	}
	return lines
}

func (m model) dropVis(id string) {
	if m.vis != nil {
		delete(m.vis, id)
	}
}

func cloneMarks(s []int) []int {
	if len(s) == 0 {
		return nil
	}
	out := make([]int, len(s))
	copy(out, s)
	return out
}

func (m model) buildVisual(innerW int) []vline {
	logical := m.textLines()
	width := innerW
	if m.stage == "logs" {
		width = logRoom(innerW, len(logical))
	}
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
	out := make([]vline, 0, len(logical))
	for i, raw := range logical {
		line := raw
		if m.stage != "mlogs" {
			line = colored[i]
		}
		if !m.wrapsLine(raw) {
			if m.stage == "mlogs" {
				line = m.colorMlog(raw)
			}
			out = append(out, vline{text: line, num: i})
			continue
		}
		parts := wrapLine(line, width)
		if m.stage == "mlogs" {
			m.paintMlogRow(raw, parts)
		}
		for j, row := range parts {
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
	if strings.IndexByte(line, 0x1b) >= 0 {
		return wrapANSI(line, width)
	}
	if isASCII(line) {
		return wrapASCII(line, width)
	}
	return wrapUnicode(line, width)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func wrapASCII(line string, width int) []string {
	if len(line) <= width {
		return []string{line}
	}
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > width/2 {
		indent = width / 2
	}
	pad := line[:indent]
	rows := make([]string, 0, len(line)/max(width-indent, 1)+1)
	rest := line
	first := true
	for {
		lim := width
		if !first {
			lim = width - indent
		}
		if len(rest) <= lim {
			if !first && indent > 0 {
				rest = pad + rest
			}
			rows = append(rows, rest)
			return rows
		}
		cut := breakASCII(rest, lim)
		row := rest[:cut]
		rest = rest[cut:]
		i := 0
		for i < len(rest) && rest[i] == ' ' {
			i++
		}
		rest = rest[i:]
		if !first && indent > 0 {
			row = pad + row
		}
		rows = append(rows, trimRightSpace(row))
		if len(rest) == 0 {
			return rows
		}
		first = false
	}
}

func breakASCII(plain string, lim int) int {
	n := lim
	if n > len(plain) {
		n = len(plain)
	}
	if n < 1 {
		return 1
	}
	lastSpace := -1
	for i := 0; i < n; i++ {
		if plain[i] == ' ' {
			lastSpace = i + 1
		}
	}
	if lastSpace > lim/2 {
		return lastSpace
	}
	return n
}

func trimRightSpace(s string) string {
	i := len(s)
	for i > 0 && s[i-1] == ' ' {
		i--
	}
	return s[:i]
}

func wrapUnicode(line string, width int) []string {
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > width/2 {
		indent = width / 2
	}
	pad := line[:indent]
	var rows []string
	rest := line
	first := true
	for {
		lim := width
		if !first {
			lim = width - indent
		}
		if ansi.StringWidth(rest) <= lim {
			if !first && indent > 0 {
				rest = pad + rest
			}
			rows = append(rows, rest)
			return rows
		}
		cut := breakUnicode(rest, lim)
		if cut <= 0 || cut > len(rest) {
			cut = len(rest)
		}
		row := rest[:cut]
		rest = rest[cut:]
		i := 0
		for i < len(rest) && rest[i] == ' ' {
			i++
		}
		rest = rest[i:]
		if !first && indent > 0 {
			row = pad + row
		}
		rows = append(rows, trimRightSpace(row))
		if len(rest) == 0 {
			return rows
		}
		first = false
	}
}

func breakUnicode(plain string, lim int) int {
	cells, lastSpace, i := 0, -1, 0
	lastBytes := 0
	for i < len(plain) {
		n, w := seqWidth(plain[i:])
		if n < 1 {
			n = 1
		}
		if cells+w > lim {
			break
		}
		cells += w
		i += n
		if n == 1 && plain[i-1] == ' ' {
			lastSpace = cells
			lastBytes = i
		}
	}
	if lastSpace > lim/2 {
		return lastBytes
	}
	if i == 0 {
		n, _ := seqWidth(plain)
		if n < 1 {
			n = 1
		}
		return n
	}
	return i
}

func seqWidth(s string) (n, w int) {
	if s == "" {
		return 0, 0
	}
	if s[0] < 0x80 {
		return 1, 1
	}
	_, size := utf8.DecodeRuneInString(s)
	if size < 1 {
		return 1, 1
	}
	cluster, gw := ansi.FirstGraphemeCluster(s[:size], ansi.GraphemeWidth)
	if len(cluster) == 0 {
		return size, 1
	}
	return size, gw
}

func wrapANSI(line string, width int) []string {
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
		w := 1
		if r >= 0x80 {
			w = ansi.StringWidth(string(r))
		}
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

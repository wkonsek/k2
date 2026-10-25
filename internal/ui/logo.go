package ui

import (
	"strings"

	"k2/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type logo struct {
	rows  []string
	masks []string
}

// default is the block mountain, simple is the smaller ascii one.
var logos = []logo{
	{
		rows:  []string{`       ▄`, `      ▟█▙`, `    ▄▟███▙▄`, `  ▄▟███████▙▄`},
		masks: []string{`       S`, `      SSS`, `    BAAAAAB`, `  BBAAAAAAABB`},
	},
	{
		rows:  []string{`       /\`, `  /\  /  \  /\`, ` /  \/    \/  \`},
		masks: []string{`       SS`, `  BB  A  A  BB`, ` B  BA    AB  B`},
	},
}

func init() {
	for i := range logos {
		logos[i] = logos[i].trimmed()
	}
}

func (l logo) trimmed() logo {
	indent := -1
	for _, row := range l.rows {
		r := []rune(strings.TrimRight(row, " "))
		n := 0
		for n < len(r) && r[n] == ' ' {
			n++
		}
		if n < len(r) && (indent < 0 || n < indent) {
			indent = n
		}
	}
	indent = max(indent, 0)
	out := logo{rows: make([]string, len(l.rows)), masks: make([]string, len(l.masks))}
	for i, row := range l.rows {
		r := []rune(strings.TrimRight(row, " "))
		out.rows[i] = string(r[min(indent, len(r)):])
	}
	for i, mask := range l.masks {
		r := []rune(strings.TrimRight(mask, " "))
		out.masks[i] = string(r[min(indent, len(r)):])
	}
	return out
}

func logoFor(name string) logo {
	if name == "simple" && len(logos) > 1 {
		return logos[1]
	}
	return logos[0]
}

func (l logo) axis2() int {
	for _, mask := range l.masks {
		r := []rune(mask)
		first, last := -1, -1
		for i, c := range r {
			if c == 'S' {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first >= 0 {
			return first + last + 1
		}
	}
	return l.width()
}

func floorDiv2(n int) int {
	if n < 0 {
		return -((1 - n) / 2)
	}
	return n / 2
}

func startOn(ax2, w, inner int) int {
	return min(max(floorDiv2(ax2-w), 0), max(inner-w, 0))
}

func maskStyle(c rune) (lipgloss.Style, bool) {
	switch c {
	case 'S':
		return lipgloss.NewStyle().Foreground(th.Bright), true
	case 'A':
		return lipgloss.NewStyle().Foreground(th.Accent), true
	case 'B':
		return lipgloss.NewStyle().Foreground(th.Accent2), true
	}
	return lipgloss.Style{}, false
}

func paint(row, mask string) string {
	r, mk := []rune(strings.TrimRight(row, " ")), []rune(mask)
	var b strings.Builder
	for i := 0; i < len(r); {
		c := ' '
		if i < len(mk) {
			c = mk[i]
		}
		j := i + 1
		for j < len(r) && (j < len(mk) && mk[j] == c || j >= len(mk) && c == ' ') {
			j++
		}
		run := string(r[i:j])
		if st, ok := maskStyle(c); ok {
			b.WriteString(st.Render(run))
		} else {
			b.WriteString(run)
		}
		i = j
	}
	return b.String()
}

func (l logo) width() int {
	w := 0
	for _, row := range l.rows {
		w = max(w, ansi.StringWidth(strings.TrimRight(row, " ")))
	}
	return w
}

func (l logo) render(inner int) ([]string, int) {
	pad := min(max(floorDiv2(inner-l.axis2()+1), 0), max(inner-l.width(), 0))
	out := make([]string, len(l.rows))
	for i, row := range l.rows {
		mask := ""
		if i < len(l.masks) {
			mask = l.masks[i]
		}
		out[i] = strings.Repeat(" ", pad) + paint(row, mask)
	}
	return out, 2*pad + l.axis2()
}

func taglineRows(text string, inner int) []string {
	text = strings.Join(strings.Fields(sanitizeLine(text)), " ")
	switch {
	case text == "":
		return nil
	case ansi.StringWidth(text) <= inner:
		return []string{text}
	}
	best, bestW := -1, 1<<30
	for i, r := range text {
		if r != ' ' {
			continue
		}
		a, b := text[:i], text[i+1:]
		wa, wb := ansi.StringWidth(a), ansi.StringWidth(b)
		if wa <= inner && wb <= inner && max(wa, wb) < bestW {
			best, bestW = i, max(wa, wb)
		}
	}
	if best >= 0 {
		return []string{text[:best], text[best+1:]}
	}
	return wrapLine(text, max(inner, 2))
}

func logoTitle(color string) lipgloss.Style {
	if color == "accent" {
		return lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(th.OK).Bold(true)
}

func head(cfg config.UI, inner int, withLogo bool) []string {
	var rows []string
	ax2 := inner
	if l := logoFor(cfg.Logo); withLogo && l.width() <= inner {
		rows, ax2 = l.render(inner)

		rows = append(rows, "")
	}
	at := func(s string, w int) string {
		return strings.Repeat(" ", startOn(ax2, w, inner)) + s
	}
	rows = append(rows, at(logoTitle(cfg.LogoTitleColor).Render("k2"), 2))
	for _, t := range taglineRows(cfg.Tagline, max(inner-1, 1)) {
		rows = append(rows, at(dimStyle.Render(t), ansi.StringWidth(t)))
	}
	return rows
}

func (m model) mark(inner int, withLogo bool) string {
	return strings.Join(head(m.cfg.UI, inner, withLogo), "\n") + "\n\n"
}

package ui

import (
	"fmt"
	"strings"

	"k2/internal/config"
	"k2/internal/forward"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	stripRows = 5
	stripMidW = 24
	stripLogo = 17
	stripInfo = 26
	stripFwds = 30
)

func (m model) stripped() bool {
	p := m.cfg.UI.SidebarPosition
	return !m.zoom && (p == "top" || p == "bottom")
}

func (m model) stripH() int {
	if !m.stripped() {
		return 0
	}
	h := m.height
	if h < 1 {
		h = 24
	}
	room := h - 3 - m.tabBarRows() - 8
	return min(stripRows+2, max(room, 3))
}

func (m model) mainOrigin() (x, y int) {
	if m.zoom {
		return 0, 0
	}
	switch m.cfg.UI.SidebarPosition {
	case "right", "bottom":
		return 0, 0
	case "top":
		return 0, m.stripH()
	}
	sideW, _, _ := m.layout()
	return sideW + 2, 0
}

func (m model) sideStrip(w, h int) string {
	inner, rows := max(w-4, 1), max(h-2, 1)
	return lipgloss.NewStyle().
		Width(max(w-2, 1)).
		Height(rows).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Muted).
		Render(crop(m.stripBody(inner, rows), inner, rows))
}

type stripCol struct {
	w     int
	lines []string
}

func (m model) stripBody(inner, rows int) string {
	sep := dimStyle.Render(" │ ")
	info := m.stripInfo(stripInfo, rows)
	fwds := m.stripFwds(stripFwds, rows)
	logo, showInfo, showFwds := inner >= 80, len(info) > 0, len(fwds) > 0
	midW := func() int {
		w := inner
		if logo {
			w -= stripLogo + 3
		}
		if showInfo {
			w -= stripInfo + 3
		}
		if showFwds {
			w -= stripFwds + 3
		}
		return w
	}
	for _, drop := range []*bool{&logo, &showInfo, &showFwds} {
		if midW() >= stripMidW {
			break
		}
		*drop = false
	}
	var cols []stripCol
	if logo {
		cols = append(cols, stripCol{stripLogo, brandRows(m.cfg.UI, stripLogo, rows >= 3)})
	}
	mid := max(midW(), 1)
	cols = append(cols, stripCol{mid, m.stripMiddle(mid, rows)})
	if showInfo {
		cols = append(cols, stripCol{stripInfo, info})
	}
	if showFwds {
		cols = append(cols, stripCol{stripFwds, fwds})
	}
	out := make([]string, rows)
	for r := range out {
		parts := make([]string, len(cols))
		for i, c := range cols {
			line := ""
			if r < len(c.lines) {
				line = c.lines[r]
			}
			parts[i] = padPlain(line, c.w)
		}
		out[r] = strings.TrimRight(strings.Join(parts, sep), " ")
	}
	return strings.Join(out, "\n")
}

func (m model) stripMiddle(w, rows int) []string {
	ctx, style := m.ctxLabel()
	ident := []string{style.Render(ctx) + dimStyle.Render(" · ") + itemStyle.Render(or(m.namespace, "all namespaces"))}
	menu := m.stripMenu(w)
	if m.kubeconfig != "" && len(menu)+2 <= rows {
		ident = append(ident, dimStyle.Render(config.HomePath(m.kubeconfig)))
	}
	lines := append(ident, menu...)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return lines
}

func (m model) stripMenu(w int) []string {
	var lines []string
	cur, curW := "", 0
	for _, raw := range m.nav.Items() {
		n := raw.(navItem)
		k := strings.TrimSpace(n.key)
		if k == "" {
			k = "·"
		}
		item := keyStyle.Render(k) + itemStyle.Render(" "+n.title)
		iw := ansi.StringWidth(k) + 1 + ansi.StringWidth(n.title)
		if cur != "" && curW+2+iw > w {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		if cur != "" {
			cur += "  "
			curW += 2
		}
		cur += item
		curW += iw
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func (m model) stripInfo(w, rows int) []string {
	alt := m
	alt.cfg.UI.TokenBar = false
	vers := splitRows(alt.infoBlock(w))
	tok := splitRows(m.tokenRows(w))
	if len(vers)+len(tok) > rows {
		vers = vers[:max(0, rows-len(tok))]
	}
	lines := append(vers, tok...)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return lines
}

func splitRows(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (m model) stripFwds(w, rows int) []string {
	n := len(m.forwards)
	if n == 0 {
		return nil
	}
	room := rows - 1
	if room < 1 {
		return []string{dimStyle.Render(fmt.Sprintf("forwards %d", n))}
	}
	show := min(n, m.cfg.UI.ForwardDockRows, room)
	if show < n && show >= room {
		show = room - 1
	}
	lines := []string{dimStyle.Render("forwards")}
	for _, f := range m.forwards[:show] {
		dot := okStyle.Render(markSquare)
		if !forward.Up(f) {
			dot = errStyle.Render(markSquare)
		}
		name := f.Pod
		if f.Namespace != "" {
			name = f.Namespace + "/" + f.Pod
		}
		lines = append(lines, clipLine(dot+itemStyle.Render(" "+f.Local+" → "+f.Remote)+dimStyle.Render("  "+name), w))
	}
	if show < n {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d", n-show)))
	}
	return lines
}

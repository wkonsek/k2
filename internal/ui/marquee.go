package ui

import (
	"strings"
	"time"

	"k2/internal/forward"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	marqueeGap   = 4
	marqueePause = 8
	marqueeStep  = 200 * time.Millisecond
	stripNameMin = 6
)

type marqueeMsg struct{ seq int }

func marqueeTick(seq int) tea.Cmd {
	return tea.Tick(marqueeStep, func(time.Time) tea.Msg { return marqueeMsg{seq} })
}

type tickerCell struct {
	s string
	w int
}

func tickerCells(s string) []tickerCell {
	var out []tickerCell
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if w == 0 {
			if len(out) > 0 {
				out[len(out)-1].s += string(r)
			}
			continue
		}
		out = append(out, tickerCell{string(r), w})
	}
	return out
}

func marquee(s string, width, frame int) string {
	if width < 1 {
		return ""
	}
	s = sanitizeLine(s)
	full := ansi.StringWidth(s)
	if full <= width {
		return s
	}
	span := full + marqueeGap
	off := max(0, frame%(span+marqueePause)-marqueePause)
	var b strings.Builder
	at, used := 0, 0
	for _, g := range tickerCells(s + strings.Repeat(" ", marqueeGap) + s) {
		if used >= width {
			break
		}
		end := at + g.w
		switch {
		case end <= off:
		case at < off:
			n := min(end-off, width-used)
			b.WriteString(strings.Repeat(" ", n))
			used += n
		case used+g.w > width:
			b.WriteString(strings.Repeat(" ", width-used))
			used = width
		default:
			b.WriteString(g.s)
			used += g.w
		}
		at = end
	}
	if used < width {
		b.WriteString(strings.Repeat(" ", width-used))
	}
	return b.String()
}

func (m model) scrollOn() bool {
	return m.cfg.UI.Animations && m.cfg.UI.ForwardScroll
}

func (m model) fitFwd(s string, width int) string {
	if m.scrollOn() {
		return marquee(s, width, m.ticker)
	}
	return clipLine(s, width)
}

func fwdName(f forward.Forward) string {
	if f.Namespace != "" {
		return f.Namespace + "/" + f.Pod
	}
	return f.Pod
}

func fwdPorts(f forward.Forward) string {
	return f.Local + " → " + f.Remote
}

func stripNameRoom(w int, ports string) int {
	return w - ansi.StringWidth(ports) - 4
}

func (m model) fwdOverflow() bool {
	if !m.scrollOn() || m.zoom || len(m.forwards) == 0 {
		return false
	}
	shown := m.forwards[:min(len(m.forwards), m.cfg.UI.ForwardDockRows)]
	if m.stripped() {
		_, mainW, _ := m.layout()
		rows := max(m.stripH()-2, 1)
		lines := m.stripFwds(stripFwds, rows)
		_, _, fwds, _ := stripCols(max(mainW-2, 1), len(m.stripInfo(stripInfo, rows)) > 0, len(lines) > 0)
		if !fwds {
			return false
		}
		for _, f := range shown[:min(len(shown), len(lines)-1)] {
			if room := stripNameRoom(stripFwds, fwdPorts(f)); room >= stripNameMin && ansi.StringWidth(fwdName(f)) > room {
				return true
			}
		}
		return false
	}
	sideW, _, _ := m.layout()
	room := m.dockRoom(sideW - 2)
	for _, f := range shown {
		if ansi.StringWidth(fwdPorts(f)) > room || ansi.StringWidth(fwdName(f)) > room {
			return true
		}
	}
	return false
}

func (m model) dockRoom(boxW int) int {
	return max(1, max(boxW, 10)-2)
}

func (m *model) marqueeStep(msg marqueeMsg) tea.Cmd {
	if msg.seq != m.tickerSeq {
		return nil
	}
	if !m.fwdOverflow() {
		m.tickerAt = time.Time{}
		m.ticker = 0
		return nil
	}
	m.ticker++
	m.tickerAt = now()
	return marqueeTick(m.tickerSeq)
}

func (m *model) marqueeStart() tea.Cmd {
	if now().Sub(m.tickerAt) < time.Second || !m.fwdOverflow() {
		return nil
	}
	m.tickerSeq++
	m.tickerAt = now()
	return marqueeTick(m.tickerSeq)
}

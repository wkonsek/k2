package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var now = time.Now

type load struct {
	token  int
	what   string
	from   string
	cancel bool
	start  time.Time
	done   int
	total  int
	lines  []string
}

func (l load) active() bool { return l.token != 0 }

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(th.Accent)))
}

func (m *model) startLoad(what string, cancel bool) tea.Cmd {
	m.loadSeq++
	m.load = load{token: m.loadSeq, what: what, from: m.stage, cancel: cancel, start: now()}
	if m.spinning || !m.cfg.UI.Animations {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

func (m model) anyLoading() bool {
	if m.load.active() {
		return true
	}
	for i, p := range m.panes {
		if i != m.focus && p.load.active() {
			return true
		}
	}
	return false
}

func (m *model) spinTick(msg spinner.TickMsg) tea.Cmd {
	if !m.anyLoading() || !m.cfg.UI.Animations {
		m.spinning = false
		return nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return cmd
}

func elapsed(d time.Duration) string {
	switch {
	case d < 0:
		d = 0
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d >= 10*time.Second:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func (m model) spinGlyph() string {
	if !m.cfg.UI.Animations {
		return "•"
	}
	return m.spin.View()
}

func loadText(l load) string {
	took := elapsed(now().Sub(l.start))
	if l.total > 0 {
		return fmt.Sprintf("%s %d/%d · %s", l.what, min(l.done, l.total), l.total, took)
	}
	return l.what + "… " + took
}

func (m model) loadLabel(l load) string {
	if !l.active() {
		return ""
	}
	return m.spinGlyph() + " " + loadText(l)
}

func (m model) loadingBlock(width int) string {
	lines := []string{"", " " + m.spinGlyph() + " " + itemStyle.Render(loadText(m.load))}
	if m.load.total > 0 {
		pct := 100 * min(m.load.done, m.load.total) / m.load.total
		lines = append(lines, " "+progressBar(m.load.done, m.load.total, min(64, width-8))+dimStyle.Render(fmt.Sprintf(" %3d%%", pct)))
	}
	if len(m.load.lines) == 0 {
		return strings.Join(lines, "\n")
	}
	var out []string
	for _, l := range m.load.lines {
		out = append(out, wrapLine(" "+colorResult(l), width)...)
	}
	room := max(1, m.textRows()-len(lines))
	if len(out) > room {
		out = out[len(out)-room:]
	}
	lines = append(lines, "")
	return strings.Join(append(lines, out...), "\n")
}

func progressBar(done, total, width int) string {
	if width < 4 || total <= 0 {
		return ""
	}
	done = max(0, min(done, total))
	fill := width * done / total
	return lipgloss.NewStyle().Foreground(th.Accent).Render(strings.Repeat("█", fill)) +
		dimStyle.Render(strings.Repeat("░", width-fill))
}

func (m *model) withPane(id string, fn func()) bool {
	if m.logID == id {
		fn()
		return true
	}
	for i := range m.panes {
		if i != m.focus && m.panes[i].logID == id {
			back := m.focus
			m.focusPane(i)
			fn()
			m.focusPane(back)
			return true
		}
	}
	return false
}

type openedMsg struct {
	pane, stage, kind, header string
	token                     int
	items                     []list.Item
	names                     []string
	toggled                   bool
	err                       error
}

func (m model) openList(stage, kind string) (tea.Model, tea.Cmd) {
	return m.openListAs(stage, kind, false)
}

func (m model) openListToggled(stage, kind string) (tea.Model, tea.Cmd) {
	return m.openListAs(stage, kind, true)
}

func (m model) openListAs(stage, kind string, toggled bool) (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	what := "loading " + kind
	if stage == "projects" {
		what = "loading namespaces"
	}
	spin := m.startLoad(what, true)
	k, kc, ctx, ns, sel, width := m.k, m.kubeconfig, m.context, m.namespace, m.vs.podSel, m.tableWidth()
	msg := openedMsg{pane: m.logID, stage: stage, kind: kind, token: m.load.token, toggled: toggled}
	fetch := func() tea.Msg {
		switch stage {
		case "workpods":
			msg.header, msg.items, msg.err = getPodItems(k, kc, ctx, ns, sel, width)
		case "res":
			msg.header, msg.items, msg.err = getResourceItems(k, kc, ctx, ns, kind, width)
		case "projects":
			msg.names, msg.err = k.Projects(kc, ctx)
		}
		return msg
	}
	return m, tea.Batch(spin, fetch)
}

func (m *model) applyOpened(msg openedMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if m.load.token != msg.token || m.stage != m.load.from {
			return
		}
		m.load = load{}
		switch msg.stage {
		case "projects":
			cmd = m.showProjects(msg.names, msg.err)
		default:
			cmd = m.showRows(msg)
		}
	})
	return cmd
}

func (m *model) showRows(msg openedMsg) tea.Cmd {
	label := m.namespace
	if label == "" {
		label = "all namespaces"
	}
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		return m.notify(toneErr, "could not list "+msg.kind)
	}
	if len(msg.items) == 0 {
		if msg.stage == "workpods" {
			m.status = "no pods in " + label
		} else {
			m.status = "no " + msg.kind
			if m.namespace != "" {
				m.status += " in " + label
			}
		}
		if m.namespace != "" && m.vs.podSel == "" {

			m.status += " — press " + m.cfg.Keys.AllNamespaces.Label() + " for all namespaces"
		}
		m.content.ResetFilter()
		m.content.SetShowTitle(false)
		m.content.Title = msg.header
		m.content.SetItems(nil)
		m.vs.pick = ""
		m.vs.marks = nil
		m.kind = msg.kind
		m.stage = msg.stage
		return m.notify(toneWarn, m.status)
	}
	m.content.ResetFilter()
	m.content.SetShowTitle(false)
	if msg.kind != m.kind || msg.stage != m.stage {
		m.vs.sortCol, m.vs.sortDesc = "", false
		m.vs.marks = nil
		m.content.SetItems(nil)
	}
	m.content.Select(0)
	m.kind = msg.kind
	m.stage = msg.stage
	header, items := m.withMetrics(msg.header, msg.items, m.kubeconfig, m.context, m.namespace, m.tableWidth())
	setRows(&m.content, header, items, m.vs, m.vs.pick)
	m.vs.pick = ""
	if msg.stage == "workpods" {
		m.status = m.podsHint()
	} else {
		m.status = msg.kind
	}
	return nil
}

func (m *model) cancelLoad() bool {
	if !m.load.active() || !m.load.cancel {
		return false
	}
	m.status = m.load.what + " cancelled"
	m.load = load{}
	return true
}

func (m *model) applyEditDone(msg editDoneMsg) {
	if msg.token != 0 && m.load.token == msg.token {
		m.load = load{}
	}
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		return
	}
	m.status = "done"
	if m.stage == "helm" {
		m.helmRefreshNow()
		return
	}
	if m.stage == "helmhist" {
		m.histRefreshNow()
		return
	}
	if m.stage != "workpods" && m.stage != "res" {
		return
	}
	var items []list.Item
	var header string
	var err error
	label := m.namespace
	if label == "" {
		label = "all namespaces"
	}
	if m.stage == "workpods" {
		header, items, err = getPodItems(m.k, m.kubeconfig, m.context, m.namespace, m.vs.podSel, m.tableWidth())
	} else {
		header, items, err = getResourceItems(m.k, m.kubeconfig, m.context, m.namespace, m.kind, m.tableWidth())
		label = m.kind
	}
	if err != nil {
		m.status = m.errLine(err)
		return
	}
	if len(items) == 0 {
		m.content.Title = header
		m.content.SetItems(nil)
		m.status = "none in " + label
		return
	}
	header, items = m.withMetrics(header, items, m.kubeconfig, m.context, m.namespace, m.tableWidth())
	setRows(&m.content, header, items, m.vs, "")
}

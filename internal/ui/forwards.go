package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k2/internal/forward"
	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) showForwards() tea.Cmd {
	keep := m.status
	m.content.ResetFilter()
	m.stage = "fwds"
	m.content.Title = "Forwards"
	m.content.SetShowTitle(false)
	m.content.SetDelegate(contentDelegate(m.tableWidth()))
	var cmd tea.Cmd
	k := m.cfg.Keys
	if len(m.forwards) == 0 {
		cmd = m.content.SetItems([]list.Item{item{title: "no forwards · " + k.PortForward.Label() + " on a pod or service starts one"}})
	} else {
		cmd = m.content.SetItems(m.forwardItems())
		m.content.Select(len(m.forwards) - 1)
	}
	if keep != "" && keep != "forwards" {
		m.status = keep
		return cmd
	}
	m.status = k.Delete.Label() + " delete    " + k.DeleteAll.Label() + " all    " + k.PfRestart.Label() + " restart    " + k.PfAuto.Label() + " auto-restart"
	return cmd
}

func (m *model) stopSelected() {
	sel := m.content.SelectedItem()
	if sel == nil {
		m.status = "nothing selected"
		return
	}
	it := sel.(item)
	pid, _ := strconv.Atoi(it.id)
	forward.Stop(pid)
	var keep []forward.Forward
	for _, f := range m.forwards {
		if f.PID != pid {
			keep = append(keep, f)
		}
	}
	m.forwards = keep
	m.save()
	m.status = "forward stopped"
	_ = m.showForwards()
}

func (m *model) stopAll() {
	for _, f := range m.forwards {
		forward.Stop(f.PID)
	}
	m.forwards = nil
	m.save()
	m.status = "all forwards stopped"
	_ = m.showForwards()
}

var forwardCols = []string{"", "STATUS", "PORTS", "TARGET", "NAMESPACE", "CONTEXT", "AGE", "REASON"}

func (m model) forwardCells(f forward.Forward) []string {
	mark, state, age := "○", m.pfStatus(f), "-"
	if forward.Up(f) {
		mark, age = "●", forward.Age(f.Started)
	}
	if m.autoOn(f) {
		state += " ↻"
	}
	return []string{mark, state, f.Local + "→" + f.Remote, forward.Display(f), or(f.Namespace, "default"), forward.Where(f), age, fwdReason(f)}
}

func fwdReason(f forward.Forward) string {
	if forward.Up(f) {
		return f.Note
	}
	return f.Reason
}

func (m model) forwardWidths(fs []forward.Forward) []int {
	w := make([]int, len(forwardCols))
	for i, h := range forwardCols {
		w[i] = ansi.StringWidth(h)
	}
	for _, f := range fs {
		for i, c := range m.forwardCells(f) {
			w[i] = max(w[i], ansi.StringWidth(c))
		}
	}
	return w
}

func forwardRow(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = fitCell(c, widths[i])
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

func (m model) forwardHeader() string {
	return forwardRow(forwardCols, m.forwardWidths(m.forwards))
}

func (m model) forwardItems() []list.Item {
	fs := m.forwards
	widths := m.forwardWidths(fs)
	items := make([]list.Item, len(fs))
	for i, f := range fs {
		items[i] = item{title: forwardRow(m.forwardCells(f), widths), id: strconv.Itoa(f.PID)}
	}
	return items
}

func spawnForwardCmd(k kube.Client, logDir, kc, ctx, ns, kind, name, local, remote string) tea.Cmd {
	return func() tea.Msg {
		f, err := forward.SpawnTarget(k.Run, logDir, kc, ctx, ns, kind, name, local, remote)
		return fwdMsg{f: f, err: err}
	}
}

func replaceForwardCmd(k kube.Client, logDir, kc, ctx, ns, kind, name, local, remote string) tea.Cmd {
	return func() tea.Msg {
		forward.KillPort(k.Run, local)
		time.Sleep(300 * time.Millisecond)
		f, err := forward.SpawnTarget(k.Run, logDir, kc, ctx, ns, kind, name, local, remote)
		return fwdMsg{f: f, err: err, retried: true}
	}
}

func (m *model) dropLocal(port string) {
	var keep []forward.Forward
	for _, f := range m.forwards {
		if f.Local == port {
			forward.Stop(f.PID)
			continue
		}
		keep = append(keep, f)
	}
	m.forwards = keep
}

func resumeForwards(k kube.Client, logDir string, fs []forward.Forward) tea.Cmd {
	cp := append([]forward.Forward(nil), fs...)
	return func() tea.Msg {
		out, note := forward.Resume(k.Run, logDir, cp)
		return resumedMsg{forwards: out, note: note}
	}
}

func (m model) logDir() string { return filepath.Join(m.cfg.StateDir, "logs") }

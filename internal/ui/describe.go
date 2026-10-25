package ui

import (
	"strings"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type describeMsg struct {
	id, text string
	err      error
}

func (m model) selectedRef() (kind, ns, name string, ok bool) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return "", "", "", false
	}
	it, isItem := sel.(item)
	if !isItem || it.id == "" {
		return "", "", "", false
	}
	switch m.stage {
	case "workpods":
		ns, name = kube.SplitRef(it.id, m.namespace)
		return "pod", ns, name, true
	case "res":
		ns, name = kube.SplitRef(it.id, it.desc)
		if ns == "" {
			ns = m.namespace
		}
		return m.kind, ns, name, m.kind != ""
	case "xray":

		if k := it.extra["kind"]; k != "" {
			return k, it.extra["ns"], it.extra["name"], true
		}
	}
	return "", "", "", false
}

func (m model) describeSelected() (tea.Model, tea.Cmd) {
	kind, ns, name, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	m.dback = m.stage
	m.vs.vback = ""
	m.subject = name
	if ns != "" {
		m.subject = ns + "/" + name
	}
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "describe"
	m.status = "describe " + m.subject
	kc, ctx, pane, k := m.kubeconfig, m.context, m.logID, m.k
	fetch := func() tea.Msg {
		text, err := k.Describe(kc, ctx, kind, name, ns)
		return describeMsg{id: pane, text: text, err: err}
	}
	return m, tea.Batch(m.startLoad("describing "+name, false), fetch)
}

func (m *model) applyDescribe(msg describeMsg) {
	set := func(stage, back, text, status *string, l *load) {
		*l = load{}
		if *stage != "describe" {
			return
		}
		if msg.err != nil {
			*stage = or(*back, "res")
			*status = m.errLine(msg.err)
			return
		}
		*text = msg.text
	}
	if m.logID == msg.id {
		set(&m.stage, &m.dback, &m.logText, &m.status, &m.load)
		return
	}
	for i := range m.panes {
		p := &m.panes[i]
		if p.logID == msg.id {
			set(&p.stage, &p.dback, &p.logText, &p.status, &p.load)
			return
		}
	}
}

var errorReasons = []string{"Failed", "BackOff", "ErrImage", "InvalidImage", "Evicted", "OOM", "Killing"}

func colorDescribe(lines []string) []string {
	out := make([]string, len(lines))
	inEvents := false
	last := itemStyle
	for i, l := range lines {
		t := strings.TrimSpace(l)
		top := l != "" && l[0] != ' ' && l[0] != '\t'
		if top {
			inEvents = strings.HasPrefix(l, "Events:")
		}
		switch {
		case top && inEvents:
			out[i] = titleStyle.Render(l)
		case inEvents && t != "":
			f := strings.Fields(t)
			switch {
			case f[0] == "Type" || strings.HasPrefix(f[0], "----"):
				last = dimStyle.Bold(true)
			case f[0] == "Warning":
				last = warnStyle
				if len(f) > 1 && eventIsError(f[1]) {
					last = errStyle
				}
			case f[0] == "Normal":
				last = dimStyle
			}
			out[i] = last.Render(l)
		case top:
			out[i] = describeKey(l)
		default:
			out[i] = itemStyle.Render(l)
		}
	}
	return out
}

func eventIsError(reason string) bool {
	for _, r := range errorReasons {
		if strings.Contains(reason, r) {
			return true
		}
	}
	return false
}

func describeKey(l string) string {
	k, v, ok := strings.Cut(l, ":")
	if !ok {
		return itemStyle.Render(l)
	}
	return keyStyle.Render(k+":") + lipgloss.NewStyle().Foreground(th.Text).Render(v)
}

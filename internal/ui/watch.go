package ui

import (
	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

const watchEvery = 2

type watchMsg struct {
	id, stage, text string
	err             error
}

func (m *model) armWatch(kind, ns, name string) {
	m.vs.watchOn = false
	m.vs.watchBusy = false
	m.vs.watchKind = kind
	m.vs.watchNS = ns
	m.vs.watchName = name
}

func (m model) toggleWatch() (tea.Model, tea.Cmd) {
	if m.vs.watchName == "" || (m.stage != "yaml" && m.stage != "describe") {
		return m, nil
	}
	m.vs.watchOn = !m.vs.watchOn
	if !m.vs.watchOn {
		m.vs.watchBusy = false
		m.status = "watch off"
		return m, nil
	}
	m.status = m.watchStatus()
	return m, m.issueWatch()
}

func (m model) watchStatus() string {
	what := m.subject
	if m.stage == "describe" {
		what = "describe " + what
	}
	return "watching " + what + " · every 2s · " + m.cfg.Keys.Watch.Label() + " stops"
}

func (m *model) watchDue() []tea.Cmd {
	if m.beat%watchEvery != 0 {
		return nil
	}
	var cmds []tea.Cmd
	if cmd := m.issueWatch(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	for i := range m.panes {
		if i == m.focus {
			continue
		}
		if cmd := issueSnapWatch(&m.panes[i], m.k); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (m *model) issueWatch() tea.Cmd {
	if !m.canWatch() {
		return nil
	}
	m.vs.watchBusy = true
	return watchCmd(m.k, m.logID, m.stage, m.vs.watchKind, m.vs.watchNS, m.vs.watchName, m.kubeconfig, m.context)
}

func (m *model) canWatch() bool {
	return m.vs.watchOn && !m.vs.watchBusy && m.vs.watchName != "" && (m.stage == "yaml" || m.stage == "describe")
}

func issueSnapWatch(p *paneSnap, k kube.Client) tea.Cmd {
	if !p.vs.watchOn || p.vs.watchBusy || p.vs.watchName == "" {
		return nil
	}
	if p.stage != "yaml" && p.stage != "describe" {
		return nil
	}
	p.vs.watchBusy = true
	return watchCmd(k, p.logID, p.stage, p.vs.watchKind, p.vs.watchNS, p.vs.watchName, p.kubeconfig, p.context)
}

func watchCmd(k kube.Client, pane, stage, kind, ns, name, kc, ctx string) tea.Cmd {
	return func() tea.Msg {
		var text string
		var err error
		if stage == "describe" {
			text, err = k.Describe(kc, ctx, kind, name, ns)
		} else {
			text, err = k.YAML(kc, ctx, kind, name, ns)
		}
		return watchMsg{id: pane, stage: stage, text: text, err: err}
	}
}

func (m *model) applyWatch(msg watchMsg) {
	set := func(stage, text, status *string, vs *viewState) {
		vs.watchBusy = false
		if !vs.watchOn || *stage != msg.stage || (*stage != "yaml" && *stage != "describe") {
			return
		}
		if msg.err != nil {
			*status = m.errLine(msg.err)
			return
		}
		*text = msg.text
	}
	if m.logID == msg.id {
		set(&m.stage, &m.logText, &m.status, &m.vs)
		if msg.err == nil && m.vs.watchOn && (m.stage == "yaml" || m.stage == "describe") {
			m.status = m.watchStatus()
		}
		return
	}
	for i := range m.panes {
		p := &m.panes[i]
		if p.logID != msg.id {
			continue
		}
		set(&p.stage, &p.logText, &p.status, &p.vs)
		if msg.err == nil && p.vs.watchOn && (p.stage == "yaml" || p.stage == "describe") {
			what := p.subject
			if p.stage == "describe" {
				what = "describe " + what
			}
			p.status = "watching " + what + " · every 2s · " + m.cfg.Keys.Watch.Label() + " stops"
		}
		return
	}
}

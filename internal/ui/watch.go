package ui

import (
	"strconv"

	"k2/internal/kube"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

type watchMsg struct {
	id, stage, text string
	alt             string
	certs           bool
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
	if m.vs.watchName == "" || !watchable(m.stage, m.vs) {
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

func (m model) watchEvery() int {
	return max(1, m.cfg.UI.WatchEvery)
}

func (m model) watchStatus() string {
	return m.watchStatusFor(m.stage, m.subject)
}

func (m model) watchStatusFor(stage, subject string) string {
	what := subject
	if stage == "describe" {
		what = "describe " + what
	}
	return "watching " + what + " · every " + strconv.Itoa(m.watchEvery()) + "s · " + m.cfg.Keys.Watch.Label() + " stops"
}

func (m *model) watchDue() []tea.Cmd {
	if m.beat%m.watchEvery() != 0 {
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
	return m.vs.watchOn && !m.vs.watchBusy && m.vs.watchName != "" && watchable(m.stage, m.vs)
}

func issueSnapWatch(p *paneSnap, k kube.Client) tea.Cmd {
	if !p.vs.watchOn || p.vs.watchBusy || p.vs.watchName == "" {
		return nil
	}
	if !watchable(p.stage, p.vs) {
		return nil
	}
	p.vs.watchBusy = true
	return watchCmd(k, p.logID, p.stage, p.vs.watchKind, p.vs.watchNS, p.vs.watchName, p.kubeconfig, p.context)
}

func watchCmd(k kube.Client, pane, stage, kind, ns, name, kc, ctx string) tea.Cmd {
	return func() tea.Msg {
		switch stage {
		case "describe":
			text, err := k.Describe(kc, ctx, kind, name, ns)
			return watchMsg{id: pane, stage: stage, text: text, err: err}
		case "view":
			v, err := k.SecretViews(kc, ctx, ns, name)
			return watchMsg{id: pane, stage: stage, text: v.Decoded, alt: v.Certs, certs: v.HasCerts, err: err}
		}
		text, err := k.YAML(kc, ctx, kind, name, ns)
		return watchMsg{id: pane, stage: stage, text: text, err: err}
	}
}

func (m *model) applyWatch(msg watchMsg) {
	set := func(stage, text, status *string, vs *viewState) {
		vs.watchBusy = false
		if !vs.watchOn || *stage != msg.stage || !watchable(*stage, *vs) || helmOwnedYAML(*vs) {
			return
		}
		if msg.err != nil {
			*status = m.errLine(msg.err)
			return
		}
		*text = msg.text
		if msg.stage == "view" {
			vs.secretAlt, vs.secretHas = msg.alt, msg.certs
			if vs.secret == "cert" {
				*text = msg.alt
			}
		}
	}
	if m.logID == msg.id {
		set(&m.stage, &m.logText, &m.status, &m.vs)
		if msg.err == nil && m.vs.watchOn && watchable(m.stage, m.vs) {
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
		if msg.err == nil && p.vs.watchOn && watchable(p.stage, p.vs) {
			p.status = m.watchStatusFor(p.stage, p.subject)
		}
		return
	}
}

func (m model) paneWatchOn(i int) bool {
	vs, stage := m.vs, m.stage
	if i != m.focus && i < len(m.panes) {
		vs, stage = m.panes[i].vs, m.panes[i].stage
	}
	return vs.watchOn && watchable(stage, vs)
}

func watchable(stage string, vs viewState) bool {
	return stage == "yaml" || stage == "describe" || (stage == "view" && vs.secret != "")
}

func (m model) paneWatchTitle(i, width int, frame lipgloss.Style) (label, badge string) {
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	label = m.title(i).render(max(1, innerW-2), frame)
	if !m.paneWatchOn(i) {
		return label, ""
	}
	badge = pickWatchBadge(innerW, ansi.StringWidth(label), m.watchEvery())
	if badge == "" {
		reserve := ansi.StringWidth("⟳") + 2
		budget := innerW - 2 - reserve
		if budget < 1 {
			return label, ""
		}
		label = m.title(i).render(budget, frame)
		badge = "⟳"
	}
	style := okStyle
	if m.title(i).muted {
		style = dimStyle
	}
	return label, style.Render(badge)
}

func pickWatchBadge(innerW, labelW, every int) string {
	if every < 1 {
		every = 1
	}
	sec := strconv.Itoa(every) + "s"
	for _, b := range []string{"⟳ watch " + sec, "⟳ " + sec, "⟳"} {
		if labelW+2+ansi.StringWidth(b)+2 <= innerW {
			return b
		}
	}
	return ""
}

package ui

import (
	"strconv"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type kindToggleState struct {
	op string
	at time.Time
}

type kindToggleMsg struct {
	pane, name, op, text string
	err                  error
}

type kindNodesMsg struct {
	nodes map[string]kube.KindNodes
}

func (m model) kindToggleKey() (tea.Model, tea.Cmd) {
	if m.kindProxySelected() {
		return m.toggleKindProxy()
	}
	name, ok := m.selectedKind()
	if !ok {
		return m, nil
	}
	t := m.kindTileFor(name)
	switch t.status {
	case "running":
		return m.askKindStop(name)
	case "stopped":
		return m.startKindCluster(name)
	case "stopping", "starting":
		m.status = name + " is already " + t.status
		return m, m.notify(toneWarn, m.status)
	}
	m.status = name + " is " + t.status + ", nothing to stop or start"
	return m, m.notify(toneWarn, m.status)
}

func (m model) askKindStop(name string) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("stop cluster"); blocked {
		return next, cmd
	}
	if _, next, cmd, ok := m.needDockerCLI("stop cluster"); !ok {
		return next, cmd
	}
	n := m.vs.kindNodes[name]
	details := []string{
		kindDetail("cluster", name),
		kindDetail("nodes", strconv.Itoa(max(n.Total, 1))+" container(s), docker stop"),
		kindDetail("keeps", "everything; "+m.cfg.Keys.KindToggle.Label()+" starts it again"),
	}
	return m.ask(askSpec{
		title: "■ kind stop", question: "stop kind cluster " + name + "?", details: details,
		kind: "kind", name: name, running: "stopping " + name + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			return m.runKindToggle(name, "stopping")
		},
	}), nil
}

func (m model) startKindCluster(name string) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("start cluster"); blocked {
		return next, cmd
	}
	if _, next, cmd, ok := m.needDockerCLI("start cluster"); !ok {
		return next, cmd
	}
	return m.runKindToggle(name, "starting")
}

func (m model) runKindToggle(name, op string) (tea.Model, tea.Cmd) {
	docker := m.dockerCLI()
	if m.kindToggle == nil {
		m.kindToggle = map[string]kindToggleState{}
	}
	m.kindToggle[name] = kindToggleState{op: op, at: now()}
	m.stage = "kind"
	m.selectKind(name)
	m.status = op + " " + name
	run, pane := m.run, m.logID
	start := op == "starting"
	return m, func() tea.Msg {
		text, err := kube.ToggleKindCluster(run, docker, name, start)
		return kindToggleMsg{pane: pane, name: name, op: op, text: text, err: err}
	}
}

func (m *model) applyKindToggle(msg kindToggleMsg) tea.Cmd {
	delete(m.kindToggle, msg.name)
	verb := "stopped"
	if msg.op == "starting" {
		verb = "started"
	}
	var cmd tea.Cmd
	apply := func() {
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "could not "+kindToggleVerb(msg.op)+" "+msg.name)
		} else {
			if msg.op == "stopping" {
				delete(m.kindUsage, msg.name)
				delete(m.kindPods, msg.name)
			}
			m.status = verb + " " + msg.name
			cmd = m.notify(toneOK, m.status)
		}
		if m.stage == "kind" {
			m.showKind(msg.name)
		}
		m.kindExtrasForce = true
		cmd = tea.Batch(cmd, m.kindRefreshCmd())
	}
	if !m.withPane(msg.pane, apply) {
		apply()
	}
	return cmd
}

func kindToggleVerb(op string) string {
	if op == "starting" {
		return "start"
	}
	return "stop"
}

func (m model) kindToggleHint() string {
	if m.kindProxySelected() {
		if m.kindProxy.Running() {
			return "stop"
		}
		return "start"
	}
	if name, ok := m.selectedKind(); ok && m.kindTileFor(name).status == "stopped" {
		return "start"
	}
	return "stop"
}

func (m model) toggleKindProxy() (tea.Model, tea.Cmd) {
	if m.kindProxyOp != "" {
		return m.kindProxyBusyMsg()
	}
	if !m.kindProxy.Running() {
		if next, cmd, blocked := m.readOnlyBlock("start registry proxy"); blocked {
			return next, cmd
		}
		docker, next, cmd, ok := m.needDockerCLI("registry proxy")
		if !ok {
			return next, cmd
		}
		p, run := m.registryProxy(), m.run
		m.kindProxyOp, m.kindProxyOpAt = "starting", now()
		m.selectKindProxy()
		m.status = "starting registry proxy " + p.Container
		return m, func() tea.Msg {
			var log strings.Builder
			_, err := kube.EnsureRegistryProxy(run, docker, p, func(line string) {
				log.WriteString(line)
				log.WriteByte('\n')
			})
			return kindProxyDoneMsg{op: "start", text: log.String(), err: err}
		}
	}
	if next, cmd, blocked := m.readOnlyBlock("stop registry proxy"); blocked {
		return next, cmd
	}
	docker, next, cmd, ok := m.needDockerCLI("registry proxy")
	if !ok {
		return next, cmd
	}
	p, run := m.registryProxy(), m.run
	details := []string{
		kindDetail("container", p.Container),
		kindDetail("keeps", config.HomePath(p.CacheDir)+" (cached layers and CA stay on disk)"),
		kindDetail("clusters", "wired clusters pull directly until it runs again"),
	}
	return m.ask(askSpec{
		title: "■ registry proxy", question: "stop registry proxy " + p.Container + "?", details: details,
		kind: "kind", name: p.Container, running: "stopping " + p.Container + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.kindProxyOp, m.kindProxyOpAt = "stopping", now()
			m.stage = "kind"
			m.selectKindProxy()
			m.status = "stopping registry proxy " + p.Container
			return m, func() tea.Msg {
				res := run.Run(shell.Request{Name: docker, Args: kube.RegistryProxyStopArgs(p.Container), Combined: true})
				text := strings.TrimSpace(string(res.Stdout) + string(res.Stderr))
				var err error
				if res.Err != nil {
					err = errorText(text, res.Err)
				}
				return kindProxyDoneMsg{op: "stop", text: text, err: err}
			}
		},
	}), nil
}

func (m model) kindNodesCmd() tea.Cmd {
	docker := m.dockerCLI()
	if docker == "" {
		return nil
	}
	run := m.run
	return func() tea.Msg {
		out, err := shell.Combined(run, docker, kube.DockerKindNodesArgs()...)
		if err != nil {
			return kindNodesMsg{}
		}
		return kindNodesMsg{nodes: kube.ParseKindNodesAt(string(out), now())}
	}
}

func (m *model) applyKindNodes(msg kindNodesMsg) {
	if msg.nodes == nil {
		return
	}
	apply := func() {
		if m.stage != "kind" && m.stage != "kindlog" {
			return
		}
		next := cloneKindNodes(msg.nodes)
		kube.KeepKindStartedAts(m.vs.kindNodes, next, now())
		m.vs.kindNodes = next
		if m.stage == "kind" {
			m.showKind("")
		}
	}
	ids := []string{m.logID}
	for i := range m.panes {
		if i != m.focus && (m.panes[i].stage == "kind" || m.panes[i].stage == "kindlog") {
			ids = append(ids, m.panes[i].logID)
		}
	}
	for _, id := range ids {
		m.withPane(id, apply)
	}
	if m.kindSnap.names != nil || m.kindSnap.nodes != nil {
		snap := cloneKindNodes(msg.nodes)
		kube.KeepKindStartedAts(m.kindSnap.nodes, snap, now())
		m.kindSnap.nodes = snap
	}
	for name, n := range msg.nodes {
		if n.Running == 0 {
			delete(m.kindUsage, name)
			delete(m.kindPods, name)
		}
	}
}

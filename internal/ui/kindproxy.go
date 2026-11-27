package ui

import (
	"context"
	"errors"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	kindIconProxy  = "⇄"
	kindProxyBadge = "registry proxy"
	kindProxyLog   = "registry-proxy"
)

type kindProxyMsg struct {
	status kube.ProxyStatus
}

type kindProxyDoneMsg struct {
	op   string
	text string
	err  error
}

type kindWiredMsg struct {
	pane string
	op   kindOp
	text string
	err  error
}

func (m model) registryProxy() kube.RegistryProxy {
	r := m.cfg.Kind.RegistryProxy
	def := config.DefaultRegistryProxy()
	p := kube.RegistryProxy{Image: r.Image, Container: r.Container, CacheDir: r.CacheDir, Registries: append([]string{}, r.Registries...)}
	if p.Image == "" {
		p.Image = def.Image
	}
	if p.Container == "" {
		p.Container = def.Container
	}
	if p.CacheDir == "" {
		p.CacheDir = def.CacheDir
	}
	return p
}

func (m model) kindProxyShown() bool {
	return m.kindProxy.Exists() || m.kindProxyOp != ""
}

func (m model) kindCardCount() int {
	n := len(m.kindTiles())
	if m.kindProxyShown() {
		n++
	}
	return n
}

func (m model) kindProxySelected() bool {
	if !m.kindProxyShown() {
		return false
	}
	return m.vs.kindSel == len(m.kindTiles())
}

func (m *model) selectKindProxy() {
	if !m.kindProxyShown() {
		return
	}
	m.vs.kindSel = len(m.kindTiles())
	m.ensureKindVisible()
}

func (m model) kindProxyCmd() tea.Cmd {
	docker := m.dockerCLI()
	if docker == "" {
		return nil
	}
	run, p := m.run, m.registryProxy()
	return func() tea.Msg {
		return kindProxyMsg{status: kube.InspectRegistryProxy(run, docker, p)}
	}
}

func (m *model) applyKindProxy(msg kindProxyMsg) {
	wasSel := m.kindProxySelected()
	m.kindProxy = msg.status
	if wasSel && m.kindProxyShown() {
		m.selectKindProxy()
	}
	if m.stage == "kind" {
		m.showKind("")
	}
}

func (m model) kindProxyRegistries() []string {
	regs := m.kindProxy.Registries
	if !m.kindProxy.Exists() || regs == nil {
		regs = m.registryProxy().Registries
	}
	out := []string{"docker.io"}
	for _, r := range regs {
		r = strings.TrimSpace(r)
		if r == "" || r == "docker.io" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (m model) kindProxyStatusLine() string {
	if m.kindProxyOp != "" {
		return warnStyle.Render(kindLabel(m.kindStatusIcon("deploying"), m.kindProxyOp+"  "+elapsed(now().Sub(m.kindProxyOpAt))))
	}
	s := m.kindProxy
	switch {
	case s.Running():
		label := "running"
		if !s.StartedAt.IsZero() {
			label += " · " + kube.FormatKindUptime(now().Sub(s.StartedAt))
		}
		return okStyle.Render(kindLabel(kindIconRunning, label))
	case s.State == "":
		return dimStyle.Render(kindLabel(kindIconStopped, "—"))
	default:
		return dimStyle.Render(kindLabel(kindIconStopped, s.State))
	}
}

func (m model) kindProxyContent(selected bool, inner int) []string {
	p := m.registryProxy()
	name := p.Container
	if selected {
		name = kindLabel(kindIconSel, name)
	}
	title := titleStyle.Render(name)
	badge := warnStyle.Render(kindIconProxy + " " + kindProxyBadge)
	title = kindJoinRight(title, badge, inner, lipgloss.Width(badge)+kindTileSafe)
	cache := "—"
	if m.kindProxy.CacheOK {
		cache = kube.FormatBytes(m.kindProxy.CacheBytes)
	}
	lines := []string{
		title,
		m.kindProxyStatusLine(),
		kindStatLine("cache", cache+" · "+config.HomePath(p.CacheDir), inner),
	}
	prefix := dimStyle.Render("registries" + strings.Repeat(" ", len("control-plane")+2-len("registries")))
	padW := len("control-plane") + 2
	for i, part := range wrapCSV(m.kindProxyRegistries(), max(8, inner-padW)) {
		head := prefix
		if i > 0 {
			head = strings.Repeat(" ", padW)
		}
		lines = append(lines, clipLine(head+itemStyle.Render(part), inner))
	}
	lines = append(lines,
		kindStatLine("proxy", p.URL(), inner),
		clipLine(dimStyle.Render(kindLabel(kindIconStep, "image cache for kind nodes, not a cluster")), inner),
	)
	if m.kindProxy.Err != nil {
		lines = append(lines, clipLine(errStyle.Render(kindLabel(kindIconStep, m.kindProxy.Err.Error())), inner))
	}
	return lines
}

func (m model) renderKindProxy(selected bool, width int) string {
	inner := max(8, width-2)
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Warn).
		Width(inner)
	if selected {
		border = border.BorderForeground(th.Accent)
	}
	return border.Render(strings.Join(m.kindProxyContent(selected, inner), "\n"))
}

func (m model) kindProxyBusyMsg() (tea.Model, tea.Cmd) {
	m.status = "registry proxy is still " + m.kindProxyOp
	return m, m.notify(toneWarn, m.status)
}

func (m model) askKindProxyStart() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("start registry proxy"); blocked {
		return next, cmd
	}
	docker, next, cmd, ok := m.needDockerCLI("registry proxy")
	if !ok {
		return next, cmd
	}
	if m.kindProxyOp != "" {
		return m.kindProxyBusyMsg()
	}
	p := m.registryProxy()
	if m.kindProxy.Running() {
		m.selectKindProxy()
		m.status = "registry proxy " + p.Container + " is already running"
		return m, m.notify(toneOK, m.status)
	}
	verb := "start"
	if !m.kindProxy.Exists() {
		verb = "set up"
	}
	details := []string{
		kindDetail("container", p.Container),
		kindDetail("image", p.Image),
		kindDetail("cache", config.HomePath(p.CacheDir)),
		kindDetail("registries", strings.Join(m.kindProxyRegistries(), ", ")),
		kindDetail("network", kube.KindNetwork),
	}
	run := m.run
	return m.ask(askSpec{
		title: kindIconProxy + " registry proxy", question: verb + " registry proxy " + p.Container + "?", details: details,
		kind: "kind", name: p.Container, running: "starting " + p.Container + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.kindProxyOp, m.kindProxyOpAt = "starting", now()
			m.stage = "kind"
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
		},
	}), nil
}

func (m model) askKindProxyRemove() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("remove registry proxy"); blocked {
		return next, cmd
	}
	docker, next, cmd, ok := m.needDockerCLI("registry proxy")
	if !ok {
		return next, cmd
	}
	if m.kindProxyOp != "" {
		return m.kindProxyBusyMsg()
	}
	p := m.registryProxy()
	details := []string{
		kindDetail("container", p.Container),
		kindDetail("keeps", config.HomePath(p.CacheDir)+" (cached layers and CA stay on disk)"),
		kindDetail("clusters", "wired clusters lose the cache until it runs again"),
	}
	run := m.run
	return m.ask(askSpec{
		title: "✗ registry proxy", question: "remove registry proxy " + p.Container + "?", details: details, danger: true,
		kind: "kind", name: p.Container, running: "removing " + p.Container + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.kindProxyOp, m.kindProxyOpAt = "removing", now()
			m.stage = "kind"
			m.selectKindProxy()
			m.status = "removing registry proxy " + p.Container
			return m, func() tea.Msg {
				res := run.Run(shell.Request{Name: docker, Args: kube.RegistryProxyRemoveArgs(p.Container), Combined: true})
				text := strings.TrimSpace(string(res.Stdout) + string(res.Stderr))
				var err error
				if res.Err != nil {
					err = errorText(text, res.Err)
				}
				return kindProxyDoneMsg{op: "remove", text: text, err: err}
			}
		},
	}), nil
}

func errorText(text string, err error) error {
	if l := lastNonEmpty(text); l != "" {
		return errors.New(l)
	}
	return err
}

func (m *model) applyKindProxyDone(msg kindProxyDoneMsg) tea.Cmd {
	m.kindProxyOp = ""
	p := m.registryProxy()
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		return tea.Batch(m.notify(toneErr, "registry proxy "+msg.op+" failed"), m.kindProxyCmd())
	}
	if msg.op == "remove" {
		m.kindProxy = kube.ProxyStatus{State: "absent"}
		m.status = "removed " + p.Container + " · cache kept in " + config.HomePath(p.CacheDir)
		if m.stage == "kind" {
			m.showKind("")
		}
		return tea.Batch(m.notify(toneOK, "removed "+p.Container), m.kindProxyCmd())
	}
	m.kindProxy.State = "running"
	if m.kindProxy.StartedAt.IsZero() {
		m.kindProxy.StartedAt = now()
	}
	m.status = "registry proxy " + p.Container + " running"
	if m.stage == "kind" {
		m.selectKindProxy()
	}
	return tea.Batch(m.notify(toneOK, m.status), m.kindProxyCmd())
}

func kindProxyLogSubject(name string) string {
	if name == kindProxyLog {
		return kindProxyBadge
	}
	return "kind " + name
}

func (m model) openKindProxyLogs() (tea.Model, tea.Cmd) {
	docker, next, cmd, ok := m.needDockerCLI("registry proxy logs")
	if !ok {
		return next, cmd
	}
	if !m.kindProxy.Exists() {
		m.status = "registry proxy is not there yet"
		return m, m.notify(toneWarn, m.status)
	}
	p := m.registryProxy()
	m.stopKindLiveLog()
	ctx, cancel := context.WithCancel(context.Background())
	m.kindLogCancel = cancel
	j := m.ensureKindJob(kindProxyLog, "logs", "registry proxy")
	j.status, j.step, j.log, j.last, j.err, j.started = "logs", "registry proxy", "", "", nil, now()
	if m.stage != "kindlog" {
		m.rememberView()
	}
	m.stage = "kindlog"
	m.vs.kindLog = kindProxyLog
	m.vs.vback = "kind"
	m.subject = kindProxyLogSubject(kindProxyLog)
	m.logText = ""
	m.logStick, m.logAt = true, 0
	m.status = p.Container + " logs · " + m.cfg.Keys.Back.Label() + " back to tiles"
	return m, m.kindStreamCtx(kindProxyLog, m.run, docker, kube.RegistryProxyLogsArgs(p.Container), "", ctx, func(text string, err error) tea.Msg {
		return kindLiveLogDoneMsg{name: kindProxyLog, text: text, err: err}
	})
}

func (m model) kindProxyKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	switch {
	case km.Delete.Has(key):
		if m.kindProxyOp != "" {
			next, cmd := m.kindProxyBusyMsg()
			return next, cmd, true
		}
		next, cmd := m.askKindProxyRemove()
		return next, cmd, true
	case key == " " || km.Edit.Has(key):
		m.status = "that is the registry proxy, not a cluster · " + km.Select.Label() + " logs, " + km.Delete.Label() + " remove"
		return m, m.notify(toneWarn, "registry proxy is not a cluster"), true
	case km.Select.Has(key) && key != "l" && key != "right":
		next, cmd := m.openKindProxyLogs()
		return next, cmd, true
	}
	return m, nil, false
}

func (m model) kindWireStep(op kindOp) tea.Cmd {
	docker := m.dockerCLI()
	kind := m.cfg.Kind.Binary
	run, pane, p := m.run, m.logID, m.registryProxy()
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			var log strings.Builder
			emit := func(line string) {
				if strings.TrimSpace(line) == "" {
					return
				}
				log.WriteString(line)
				log.WriteByte('\n')
				ch <- kindLineMsg{pane: pane, name: op.name, line: line, step: "wiring registry cache", ch: ch}
			}
			var err error
			if docker == "" {
				err = errors.New("registry cache needs docker in PATH")
			} else {
				err = kube.WireKindClusterProxy(run, docker, kind, op.name, p, emit)
			}
			ch <- kindWiredMsg{pane: pane, op: op, text: log.String(), err: err}
		}()
		return <-ch
	}
}

func (m *model) applyKindWired(msg kindWiredMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		j := m.ensureKindJob(msg.op.name, "", "")
		note := "registry cache: " + m.registryProxy().Container
		if msg.err != nil {
			note = "registry cache skipped: " + msg.err.Error()
		}
		if j.log != "" && !strings.HasSuffix(j.log, "\n") {
			j.log += "\n"
		}
		j.log += note + "\n"
		m.syncKindLog(msg.op.name)
		var warn tea.Cmd
		if msg.err != nil {
			warn = m.notify(toneWarn, "registry cache not wired on "+msg.op.name)
		}
		cmd = tea.Batch(warn, m.kindProxyCmd(), m.finishKindCreate(msg.op))
	})
	return cmd
}

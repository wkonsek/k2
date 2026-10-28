package ui

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pulseMsg struct {
	pane    string
	token   int
	refresh bool
	ns      string
	pulse   kube.Pulse
	at      time.Time
	err     error
}

func (m model) openPulse() (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	if m.namespace == "" {
		m.status = "pulse needs a namespace (pick one with " + m.cfg.Keys.Namespace.Label() + ")"
		return m, m.notify(toneWarn, "pulse needs a namespace")
	}
	m.vs.vback = ""
	m.subject = "pulse " + m.namespace
	m.logText = ""
	m.logStick, m.logAt = false, 0
	m.stage = "pulse"
	m.status = "pulse of " + m.namespace + " · refreshed every " + plural(max(1, m.cfg.UI.RefreshEvery), "second", "seconds")
	spin := m.startLoad("reading the namespace", false)
	return m, tea.Batch(spin, pulseCmd(m.k, m.logID, m.load.token, false, m.kubeconfig, m.context, m.namespace))
}

func pulseCmd(k kube.Client, pane string, token int, refresh bool, kc, ctx, ns string) tea.Cmd {
	return func() tea.Msg {
		msg := pulseMsg{pane: pane, token: token, refresh: refresh, ns: ns, at: now()}
		objs, err := k.PulseObjects(kc, ctx, ns)
		if err != nil {
			msg.err = err
			return msg
		}
		events, _ := k.WarningEvents(kc, ctx, ns)
		usage, err := k.TopPods(kc, ctx, ns)
		if err != nil {
			usage = nil
		}
		msg.pulse = kube.BuildPulse(objs, events, usage)
		return msg
	}
}

func (m *model) applyPulse(msg pulseMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if msg.refresh {
			delete(m.rowBusy, "pu"+msg.pane)
		} else if m.load.token == msg.token {
			m.load = load{}
		} else {
			return
		}
		if m.stage != "pulse" || m.namespace != msg.ns {
			return
		}
		if msg.err != nil {
			m.noteErr(msg.err.Error())
			if !msg.refresh || m.logText == "" {
				m.logText = "error: " + strings.TrimSpace(msg.err.Error())
				cmd = m.notify(toneErr, "could not read the namespace")
			}
			return
		}
		m.logText = m.pulseText(msg.pulse, msg.ns, msg.at)
	})
	return cmd
}

func (m *model) pulseTick() []tea.Cmd {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	shown := map[string]bool{}
	for _, id := range m.tabIDs() {
		shown[id] = true
	}
	var cmds []tea.Cmd
	try := func(id, stage, kc, ctx, ns string, l load) {
		if stage != "pulse" || !shown[id] || l.active() || m.rowBusy["pu"+id] || ns == "" {
			return
		}
		m.rowBusy["pu"+id] = true
		cmds = append(cmds, pulseCmd(m.k, id, 0, true, kc, ctx, ns))
	}
	try(m.logID, m.stage, m.kubeconfig, m.context, m.namespace, m.load)
	for i := range m.panes {
		if i != m.focus {
			p := m.panes[i]
			try(p.logID, p.stage, p.kubeconfig, p.context, p.namespace, p.load)
		}
	}
	return cmds
}

const (
	markOK   = markSquare + " "
	markWarn = "▲ "
	markErr  = "✖ "
	markDim  = "· "
)

func (m model) pulseText(p kube.Pulse, ns string, at time.Time) string {
	var b strings.Builder
	line := func(indent int, mark, text string) {
		b.WriteString(strings.Repeat(" ", indent) + mark + text + "\n")
	}
	count := func(n int, bad string) string {
		if n > 0 {
			return bad
		}
		return markOK
	}
	b.WriteString(fmt.Sprintf("PULSE  %s · %s · read %s\n\n", ns, m.context, at.Format("15:04:05")))

	b.WriteString(fmt.Sprintf("PODS  %d\n", p.Pods))
	line(2, markOK, fmt.Sprintf("%-10s %d", "running", p.Running))
	line(2, count(p.Pending, markWarn), fmt.Sprintf("%-10s %d", "pending", p.Pending))
	line(2, count(p.Failed, markErr), fmt.Sprintf("%-10s %d", "failed", p.Failed))
	line(2, count(p.Crashing, markErr), fmt.Sprintf("%-10s %d", "crashing", p.Crashing))
	if p.Succeeded > 0 {
		line(2, markDim, fmt.Sprintf("%-10s %d", "completed", p.Succeeded))
	}
	line(2, count(p.Restarts, markWarn), fmt.Sprintf("%-10s %d", "restarts", p.Restarts))

	b.WriteString("\nWORKLOADS\n")
	for _, w := range p.Workloads {
		mark := markOK
		switch {
		case w.Total == 0:
			mark = markDim
		case w.Ready < w.Total:
			mark = markWarn
		}
		line(2, mark, fmt.Sprintf("%-13s %d/%d ready", strings.ToLower(w.Kind), w.Ready, w.Total))
	}
	jobs := fmt.Sprintf("%-13s %d active, %d done, %d failed", "jobs", p.JobsActive, p.JobsDone, p.JobsFailed)
	line(2, count(p.JobsFailed, markErr), jobs)
	pvc := fmt.Sprintf("%-13s %d bound, %d pending, %d lost", "pvcs", p.PVCBound, p.PVCPending, p.PVCLost)
	switch {
	case p.PVCLost > 0:
		line(2, markErr, pvc)
	case p.PVCPending > 0:
		line(2, markWarn, pvc)
	default:
		line(2, markOK, pvc)
	}

	b.WriteString("\nUSAGE\n")
	if !p.Metrics {
		line(2, markDim, "no metrics (metrics-server not available)")
	} else {
		b.WriteString(m.usageLine("cpu", p.CPUUsed, p.CPUOf, p.CPUApprox, cpuText))
		b.WriteString(m.usageLine("memory", p.MemUsed, p.MemOf, p.MemApprox, memText))
	}

	if len(p.Problems) > 0 {
		b.WriteString("\nTROUBLE\n")
		for _, pr := range p.Problems {
			line(2, markErr, pr)
		}
	}

	b.WriteString("\nWARNINGS")
	if len(p.Warnings) == 0 {
		b.WriteString("\n")
		line(2, markOK, "no warning events")
	} else {
		b.WriteString(fmt.Sprintf("  %d\n", len(p.Warnings)))
		for i, e := range p.Warnings {
			if i == 10 {
				line(2, markDim, fmt.Sprintf("+%d more (kubectl get events --field-selector type=Warning)", len(p.Warnings)-10))
				break
			}
			when := "-"
			if !e.At.IsZero() {
				when = kube.Age(e.At)
			}
			cnt := ""
			if e.Count > 1 {
				cnt = fmt.Sprintf(" ×%d", e.Count)
			}
			line(2, markWarn, fmt.Sprintf("%-5s %s %s%s: %s", when, e.Object, e.Reason, cnt, e.Message))
		}
	}
	return b.String()
}

func (m model) usageLine(what string, used, of float64, approx bool, text func(float64) string) string {
	const width = 24
	if of <= 0 {
		return "  " + markDim + fmt.Sprintf("%-7s %s used (no limits or requests to compare with)\n", what, text(used))
	}
	pct := int(used * 100 / of)
	fill := min(width, max(0, width*pct/100))
	mark := markOK
	switch {
	case pct >= m.cfg.UI.MetricsCrit:
		mark = markErr
	case pct >= m.cfg.UI.MetricsWarn:
		mark = markWarn
	}
	basis := "limits"
	if approx {
		basis = "requests ~"
	}
	return "  " + mark + fmt.Sprintf("%-7s %s%s %3d%%  %s of %s (%s)\n", what,
		strings.Repeat("━", fill), strings.Repeat("─", width-fill), pct, text(used), text(of), basis)
}

func cpuText(milli float64) string {
	if milli < 1000 {
		return fmt.Sprintf("%.0fm", milli)
	}
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", milli/1000), "0"), ".")
}

func memText(b float64) string {
	const mi = 1024 * 1024
	if b >= 1024*mi {
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", b/(1024*mi)), "0"), ".") + "Gi"
	}
	return fmt.Sprintf("%.0fMi", b/mi)
}

func colorPulse(line string) string {
	trim := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trim)]
	st := lipgloss.Style{}
	switch {
	case strings.HasPrefix(trim, markOK):
		st = okStyle
	case strings.HasPrefix(trim, markWarn):
		st = warnStyle
	case strings.HasPrefix(trim, markErr):
		st = errStyle
	case strings.HasPrefix(trim, markDim):
		st = dimStyle
	case trim != "" && indent == "":
		return titleStyle.Render(line)
	default:
		return line
	}
	return indent + st.Render(trim)
}

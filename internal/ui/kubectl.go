package ui

import (
	"sort"
	"strings"

	"k2/internal/config"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) shellBody(innerW int) string {
	fmts := config.OutputFormats
	if m.shFmt < 0 || m.shFmt >= len(fmts) {
		m.shFmt = 0
	}
	var chips []string
	for i, name := range fmts {
		if i == m.shFmt {
			chips = append(chips, keyStyle.Render(name))
		} else {
			chips = append(chips, dimStyle.Render(name))
		}
	}
	outLine := dimStyle.Render("output ") + strings.Join(chips, dimStyle.Render("  "))
	hint := m.shHint
	if hint == "" {
		k := m.cfg.Keys
		hint = k.Complete.Label() + " completes    " + k.Output.Label() + " output    " + k.JQ.Label() + " jq"
	}
	jq := m.jq.View()
	if !m.jqOn && strings.TrimSpace(m.jq.Value()) == "" {
		jq = dimStyle.Render("jq  (" + m.cfg.Keys.JQ.Label() + ")")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("kubectl"),
		"",
		m.kubectlPrompt(innerW),
		jq,
		"",
		clipLine(outLine, innerW),
		"",
		dimStyle.Render(clipLine(hint, innerW)),
	)
}

func (m model) inputFocused() bool {
	if m.cmdOn {
		return true
	}
	return m.pfOn || m.dlg != nil || (m.stage == "kcmd" || m.listFiltering())
}

func (m model) listFiltering() bool {
	return m.stage != "nav" && m.stage != "yaml" && m.stage != "kcmd" && m.content.FilterState() == list.Filtering
}

func (m model) typeInto(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.cmdOn:
		m.cmdCycle = nil
		m.cmdCycleAt = -1
		m.cmdBase = ""
		m.cmd, cmd = m.cmd.Update(msg)

		fetch := m.liveHint()
		cmd = tea.Batch(cmd, fetch)
	case m.stage == "kcmd" && m.jqOn:
		m.jq, cmd = m.jq.Update(msg)
	case m.stage == "kcmd":
		m.shCycle = nil
		m.shCycleAt = -1
		m.sh, cmd = m.sh.Update(msg)
	case m.listFiltering():
		m.content, cmd = m.content.Update(msg)
	}
	return m, cmd
}

func (m *model) updateShell(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	switch {
	case km.ShellBack.Has(key):
		if m.jqOn {
			m.jqOn = false
			m.jq.Blur()
			m.sh.Focus()
			return *m, nil
		}
		return *m, m.back()
	case km.JQ.Has(key):
		m.jqOn = !m.jqOn
		if m.jqOn {
			m.sh.Blur()
			m.jq.Focus()
		} else {
			m.jq.Blur()
			m.sh.Focus()
		}
		return *m, nil
	case km.Output.Has(key):
		m.shFmt = (m.shFmt + 1) % len(config.OutputFormats)
		return *m, nil
	case km.Complete.Has(key):
		if !m.jqOn {
			m.completeShell()
		}
		return *m, nil
	case km.Submit.Has(key):
		line := strings.TrimSpace(m.sh.Value())
		if line == "" {
			return *m, nil
		}
		if m.kubeconfig == "" || m.context == "" {
			m.status = "select kubeconfig and context first"
			return *m, nil
		}
		m.subject = "kubectl"
		m.kback = "kcmd"
		m.ran = runInfo{}
		m.logText = ""
		m.logStick = true
		m.logAt = 0
		m.stage = "kout"
		m.status = "running"
		spin := m.startLoad("running kubectl", false)
		return *m, tea.Batch(spin, m.runKubectl(line))
	default:
		if !m.jqOn {
			m.shCycle = nil
			m.shCycleAt = -1
		}
		var cmd tea.Cmd
		if m.jqOn {
			m.jq, cmd = m.jq.Update(msg)
		} else {
			m.sh, cmd = m.sh.Update(msg)
		}
		return *m, cmd
	}
}

func (m *model) completeShell() {
	if len(m.shCycle) > 1 {
		m.shCycleAt = (m.shCycleAt + 1) % len(m.shCycle)
		m.applyShellChoice(m.shCycle[m.shCycleAt])
		m.shHint = shellHint(m.shCycle, m.shCycleAt)
		return
	}
	line := m.sh.Value()
	cands, word := shellCandidates(line, m.resources)
	m.shCycle = nil
	m.shCycleAt = -1
	if len(cands) == 0 {
		m.shHint = "no match"
		return
	}
	if len(cands) == 1 {
		m.applyShellChoice(cands[0])
		m.shHint = cands[0]
		return
	}
	pre := cands[0]
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c, pre) && pre != "" {
			pre = pre[:len(pre)-1]
		}
	}
	if len(pre) > len(word) {
		m.applyShellChoice(pre)
	}
	m.shCycle = cands
	m.shCycleAt = -1
	shown := cands
	if len(shown) > 8 {
		shown = shown[:8]
	}
	m.shHint = strings.Join(shown, "  ")
}

func (m *model) applyShellChoice(choice string) {
	line := m.sh.Value()
	trail := strings.HasSuffix(line, " ")
	fields := strings.Fields(line)
	if trail || len(fields) == 0 {
		if strings.TrimSpace(line) == "" {
			m.sh.SetValue(choice)
		} else {
			m.sh.SetValue(strings.TrimRight(line, " ") + " " + choice)
		}
	} else {
		fields[len(fields)-1] = choice
		m.sh.SetValue(strings.Join(fields, " "))
	}
	m.sh.CursorEnd()
}

func shellCandidates(line string, resources []string) ([]string, string) {
	trail := strings.HasSuffix(line, " ")
	fields := strings.Fields(line)
	word := ""
	if !trail && len(fields) > 0 {
		word = fields[len(fields)-1]
	}
	depth := len(fields)
	if !trail && depth > 0 {
		depth--
	}
	var base []string
	if depth == 0 {
		base = []string{
			"get", "describe", "logs", "delete", "apply", "diff", "top", "exec",
			"rollout", "scale", "edit", "explain", "wait", "annotate", "label",
			"port-forward", "auth", "api-resources", "create", "events",
		}
	} else if depth == 1 {
		verb := ""
		if len(fields) > 0 {
			verb = fields[0]
		}
		switch verb {
		case "get", "describe", "delete", "edit", "explain", "logs", "wait", "top":
			base = []string{"pods", "svc", "deploy", "sts", "ds", "cm", "secret", "ing", "ns", "sa", "job", "pvc", "no", "events"}
			base = append(base, resources...)
		case "rollout":
			base = []string{"status", "restart", "undo", "history"}
		default:
			base = append([]string{}, resources...)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range base {
		if s == "" || seen[s] {
			continue
		}
		if word != "" && !strings.HasPrefix(s, word) {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out, word
}

func shellHint(cands []string, at int) string {
	if at < 0 {
		at = 0
	}
	start := 0
	if at > 7 {
		start = at - 7
	}
	end := start + 8
	if end > len(cands) {
		end = len(cands)
	}
	var parts []string
	for i := start; i < end; i++ {
		if i == at {
			parts = append(parts, "["+cands[i]+"]")
		} else {
			parts = append(parts, cands[i])
		}
	}
	return strings.Join(parts, "  ")
}

func (m model) runKubectl(line string) tea.Cmd {
	kc, ctx, ns := m.kubeconfig, m.context, m.namespace
	format := config.OutputFormats[m.shFmt%len(config.OutputFormats)]
	if format == "default" {
		format = ""
	}
	jqExpr := strings.TrimSpace(m.jq.Value())
	pane := m.logID
	return func() tea.Msg {
		text, code := m.k.RunLine(line, kc, ctx, ns, format, jqExpr)
		return kOutMsg{id: pane, text: text, code: code, line: line, ctx: ctx}
	}
}

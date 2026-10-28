package ui

import (
	"sort"
	"strings"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateCmd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	switch {
	case km.Cancel.Has(key):
		m.cmdOn = false
		m.cmd.Blur()
		return m, nil
	case km.Submit.Has(key):
		q := strings.TrimSpace(m.cmd.Value())
		m.cmdOn = false
		m.cmd.Blur()
		if q == "" {
			return m, nil
		}
		return m.runCommand(q)
	case km.Complete.Has(key):
		if cmd, ok := m.completeArg(); ok {
			return m, cmd
		}
		m.completeCmd()
		return m, nil
	case len([]rune(key)) > 1 && (km.ScrollDown.Has(key) || km.ScrollUp.Has(key)) && strings.Contains(m.cmd.Value(), " "):

		step := 1
		if km.ScrollUp.Has(key) {
			step = -1
		}
		cmd := m.cycleArg(step)
		return m, cmd
	default:
		m.cmdCycle = nil
		m.cmdCycleAt = -1
		m.cmdBase = ""
		var cmd tea.Cmd
		m.cmd, cmd = m.cmd.Update(msg)

		fetch := m.liveHint()
		return m, tea.Batch(cmd, fetch)
	}
}

func (m model) runCommand(raw string) (tea.Model, tea.Cmd) {

	m.vs.jumpFrom, m.vs.jumpSel, m.vs.vback = "", "", ""

	if next, cmd, ok := m.runV07Command(strings.TrimSpace(raw)); ok {
		return next, cmd
	}
	q := strings.ToLower(strings.TrimSpace(raw))
	if target, ok := m.cfg.Aliases[q]; ok {
		q = target
	}
	switch q {

	case "kubeconfig", "kube", "kc", "c":
		return m.openNamed("kube")
	case "context", "ctx", "x":
		return m.openNamed("ctx")
	case "namespace", "ns", "project", "projects", "proj", "o":
		return m.openNamed("proj")
	case "pods", "pod", "po", "r":
		return m.openNamed("pods")
	case "forward", "p":

		return m.openPods()
	case "forwards", "fwd", "fwds", "f", "pf":
		return m.openNamed("fwds")
	case "kustomize", "kust", "kustomization", "k":
		return m.openNamed("kust")
	case "kubectl", "shell", "sh":
		return m.openNamed("sh")
	}
	kind := m.canonical(q)
	if kind == "" {

		if target, ok := kube.Builtin[q]; ok {
			base, _, _ := strings.Cut(target, ".")
			if len(m.resources) > 0 && m.canonical(base) == "" {
				m.status = kube.MissingAPI(target, "doesn't have a resource type")
				return m, m.notify(toneWarn, "not installed: "+base)
			}
			kind = target
		}
	}
	if kind == "" && kube.KnownKind(q) {
		if len(m.resources) > 0 {

			m.status = kube.MissingAPI(q, "doesn't have a resource type")
			return m, m.notify(toneWarn, "not installed: "+q)
		}
		kind = q
	}
	if kind == "" {
		m.status = "unknown: " + q
		return m, nil
	}
	if kind == "pods" {
		return m.openNamed("pods")
	}
	return m.openResource(kind)
}

func (m model) runV07Command(raw string) (tea.Model, tea.Cmd, bool) {
	word, rest, _ := strings.Cut(raw, " ")
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(word) {
	case "xray":
		if rest == "" {

			m.cmdOn = true
			m.cmd.SetValue("xray ")
			m.cmd.Focus()
			m.cmd.CursorEnd()
			m.cmdCycle, m.cmdCycleAt, m.cmdBase = nil, -1, ""
			m.cmdHint = xrayKindList()
			m.status = "xray <kind> [name] · tab completes · or " + m.cfg.Keys.Xray.Short() + " on a table row"
			return m, nil, true
		}
		kind, name, _ := strings.Cut(rest, " ")
		next, cmd := m.openXray(strings.ToLower(kind), strings.TrimSpace(name))
		return next, cmd, true
	case "logs", "mlogs", "stern":
		if rest == "" {
			m.status = "logs <selector or pod regex> [--since=5m] [--tail=N] [-c container] [--all=false]"
			return m, m.notify(toneWarn, "logs needs a selector or regex"), true
		}
		next, cmd := m.runLogsCommand(rest)
		return next, cmd, true
	case "helm", "releases", "charts":
		switch strings.ToLower(rest) {
		case "all", "-a", "--all-namespaces":
			m.vs.helmAll = true
		case "":
			m.vs.helmAll = false
		}
		next, cmd := m.openHelm()
		return next, cmd, true
	case "pulse":
		next, cmd := m.openPulse()
		return next, cmd, true
	case "fav", "favs", "favorites", "favourites":
		if rest != "" {
			next, cmd := m.favByName(rest)
			return next, cmd, true
		}
		next, cmd := m.openFavorites()
		return next, cmd, true
	case "theme", "themes":
		next, cmd := m.openTheme()
		return next, cmd, true
	case "login", "token":
		next, cmd := m.openLogin()
		return next, cmd, true
	}
	return m, nil, false
}

func (m *model) completeCmd() {
	if len(m.cmdCycle) > 1 {
		m.cmdCycleAt = (m.cmdCycleAt + 1) % len(m.cmdCycle)
		m.cmd.SetValue(m.cmdCycle[m.cmdCycleAt])
		m.cmd.CursorEnd()
		m.cmdHint = cycleHint(m.cmdCycle, m.cmdCycleAt)
		return
	}
	tok := strings.ToLower(strings.TrimSpace(m.cmd.Value()))
	cands := m.cmdCandidates(tok)
	m.cmdCycle = nil
	m.cmdCycleAt = -1
	if len(cands) == 0 {
		m.cmdHint = "no match"
		return
	}
	if len(cands) == 1 {
		m.cmd.SetValue(cands[0])
		m.cmd.CursorEnd()
		m.cmdHint = cands[0]
		return
	}
	pre := cands[0]
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c, pre) && pre != "" {
			pre = pre[:len(pre)-1]
		}
	}
	if pre != "" && len(pre) >= len(tok) {
		m.cmd.SetValue(pre)
		m.cmd.CursorEnd()
	}
	m.cmdCycle = cands
	m.cmdCycleAt = -1
	shown := cands
	if len(shown) > 8 {
		shown = shown[:8]
	}
	m.cmdHint = strings.Join(shown, "  ")
}

func cycleHint(cands []string, at int) string {
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
	parts := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if i == at {
			parts = append(parts, "["+cands[i]+"]")
		} else {
			parts = append(parts, cands[i])
		}
	}
	if end < len(cands) {
		parts = append(parts, "…")
	}
	return strings.Join(parts, "  ")
}

func (m model) cmdCandidates(tok string) []string {
	base := []string{"kubeconfig", "context", "namespace", "pods", "forward", "forwards", "kustomize", "kubectl",
		"xray", "logs", "helm", "pulse", "fav", "theme", "login"}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		if tok != "" && !strings.HasPrefix(s, tok) {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range base {
		add(s)
	}
	for _, s := range m.resources {
		add(s)
	}
	for a := range m.cfg.Aliases {
		add(a)
	}
	for a := range kube.Builtin {
		add(a)
	}
	sort.Strings(out)
	return out
}

func (m model) canonical(q string) string {
	if q == "pods" || q == "pod" || q == "po" {
		return "pods"
	}
	for _, s := range m.resources {
		if s == q {
			return s
		}
	}
	return ""
}

func fetchResources(k kube.Client, kc, ctx string) tea.Cmd {
	return func() tea.Msg {
		names, err := k.APIResources(kc, ctx)
		if err != nil {
			return resListMsg{ctx: ctx}
		}
		return resListMsg{ctx: ctx, names: names}
	}
}

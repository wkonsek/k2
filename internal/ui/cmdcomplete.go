package ui

import (
	"errors"
	"sort"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

const namesTTL = 10 * time.Second

type nameEntry struct {
	names []string
	err   string
	at    time.Time
}

type namesMsg struct {
	key   string
	names []string
	err   error
}

var complKinds = []struct {
	name    string
	aliases []string
}{
	{"deploy", []string{"deployment", "deployments"}},
	{"sts", []string{"statefulset", "statefulsets"}},
	{"ds", []string{"daemonset", "daemonsets"}},
	{"rs", []string{"replicaset", "replicasets"}},
	{"job", []string{"jobs"}},
	{"cronjob", []string{"cronjobs", "cj"}},
	{"pod", []string{"pods", "po"}},
	{"svc", []string{"service", "services"}},
}

func xrayKindList() string {
	names := make([]string, len(complKinds))
	for i, k := range complKinds {
		names[i] = k.name
	}
	return strings.Join(names, "  ")
}

func matchRank(cand, tok string, aliases ...string) int {
	tok = strings.ToLower(tok)
	best := -1
	for _, c := range append([]string{cand}, aliases...) {
		c = strings.ToLower(c)
		switch {
		case strings.HasPrefix(c, tok):
			return 0
		case strings.Contains(c, tok):
			best = 1
		}
	}
	return best
}

func ranked(cands []string, tok string, aliases func(string) []string) []string {
	var first, second []string
	for _, c := range cands {
		var al []string
		if aliases != nil {
			al = aliases(c)
		}
		switch matchRank(c, tok, al...) {
		case 0:
			first = append(first, c)
		case 1:
			second = append(second, c)
		}
	}
	return append(first, second...)
}

type argCompletion struct {
	base string
	toks []string
	note string
	cmd  tea.Cmd
	ok   bool
}

func (a argCompletion) lines() []string {
	out := make([]string, len(a.toks))
	for i, t := range a.toks {
		out[i] = a.base + t
	}
	return out
}

func (m *model) argCandidates(val string) argCompletion {
	word, rest, spaced := strings.Cut(val, " ")
	if !spaced {
		return argCompletion{}
	}
	word = strings.ToLower(word)
	args := strings.Fields(rest)
	trailing := strings.HasSuffix(rest, " ") || rest == ""

	cur, tok := len(args), ""
	if !trailing && len(args) > 0 {
		cur, tok = len(args)-1, args[len(args)-1]
	}
	base := word + " "
	for _, a := range args[:cur] {
		base += a + " "
	}
	switch word {
	case "xray":
		switch cur {
		case 0:
			names := make([]string, len(complKinds))
			for i, k := range complKinds {
				names[i] = k.name
			}
			toks := ranked(names, tok, func(c string) []string {
				for _, k := range complKinds {
					if k.name == c {
						return k.aliases
					}
				}
				return nil
			})
			return argCompletion{base: base, toks: toks, ok: true}
		case 1:
			short, ok := kube.XrayKind(args[0])
			if !ok {
				return argCompletion{base: base, note: "no owner tree for " + args[0] + " (" + xrayKindList() + ")", ok: true}
			}
			return m.nameCompletion(base, tok, xrayResource[short])
		}
	case "logs", "mlogs", "stern":
		if cur == 0 && !strings.HasPrefix(tok, "-") && !strings.ContainsAny(tok, "=,()!") {
			return m.nameCompletion(base, tok, "pods")
		}
	case "fav", "favs", "favorites", "favourites":

		var names []string
		for _, e := range m.favEntries() {
			names = append(names, favName(e.fav))
		}
		return argCompletion{base: word + " ", toks: ranked(names, strings.TrimSpace(rest), nil), ok: true}
	case "profile":
		names := m.profileNames()
		if cur == 0 || (cur == 1 && strings.EqualFold(args[0], "save")) {
			if len(names) == 0 {
				return argCompletion{base: base, note: "no profiles yet", ok: true}
			}
			return argCompletion{base: base, toks: ranked(names, tok, nil), ok: true}
		}
	}
	return argCompletion{}
}

func favName(f config.Favorite) string {
	if f.Name != "" {
		return f.Name
	}
	return f.Context + "/" + or(f.Namespace, "default")
}

func (m *model) nameCompletion(base, tok, resource string) argCompletion {
	a := argCompletion{base: base, ok: true}
	if m.kubeconfig == "" || m.context == "" {
		a.note = "select kubeconfig and context first"
		return a
	}
	key := m.kubeconfig + "\x00" + m.context + "\x00" + m.namespace + "\x00" + resource
	if m.names == nil {
		m.names, m.namesBusy = map[string]*nameEntry{}, map[string]bool{}
	}
	e := m.names[key]
	if e == nil || now().Sub(e.at) > namesTTL {
		if !m.namesBusy[key] {
			m.namesBusy[key] = true
			a.cmd = namesCmd(m.k, key, m.kubeconfig, m.context, m.namespace, resource)
		}
		if e == nil {
			a.note = "loading " + resource + " names…"
			return a
		}
	}
	if e.err != "" {
		a.note = "could not list " + resource + ": " + e.err
		return a
	}
	a.toks = ranked(e.names, tok, nil)
	return a
}

func namesCmd(k kube.Client, key, kc, ctx, ns, resource string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"get", resource, "-o", "name", "--kubeconfig", kc, "--context", ctx, "--request-timeout=5s"}
		if ns == "" {
			args = append(args, "-A")
		} else {
			args = append(args, "-n", ns)
		}
		res := k.Run.Run(shell.Request{Name: "kubectl", Args: args})
		if res.Err != nil {
			msg := strings.TrimSpace(string(res.Stderr) + " " + string(res.Stdout))
			if msg == "" {
				msg = res.Err.Error()
			}
			return namesMsg{key: key, err: errors.New(msg)}
		}
		out := res.Stdout
		var names []string
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if _, n, ok := strings.Cut(strings.TrimSpace(l), "/"); ok && n != "" {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		return namesMsg{key: key, names: names}
	}
}

func (m model) applyNames(msg namesMsg) (tea.Model, tea.Cmd) {
	if m.names == nil {
		m.names, m.namesBusy = map[string]*nameEntry{}, map[string]bool{}
	}
	delete(m.namesBusy, msg.key)
	e := &nameEntry{names: msg.names, at: now()}
	if msg.err != nil {
		e.err = shell.OneLine(msg.err.Error())
	}
	m.names[msg.key] = e
	var cmd tea.Cmd
	if m.cmdOn {
		cmd = m.liveHint()
	}
	return m, cmd
}

func (m *model) liveHint() tea.Cmd {
	a := m.argCandidates(m.cmd.Value())
	if !a.ok {
		if strings.Contains(m.cmd.Value(), " ") {
			m.cmdHint = ""
		}
		return nil
	}
	m.cmdCycle, m.cmdCycleAt = nil, -1
	m.cmdHint = suggestLine(a.toks, a.note, -1)
	return a.cmd
}

func suggestLine(toks []string, note string, at int) string {
	switch {
	case len(toks) == 0 && note != "":
		return note
	case len(toks) == 0:
		return "no match"
	case at >= 0:
		return cycleHint(toks, at)
	}
	shown := toks
	more := ""
	if len(shown) > 8 {
		shown, more = shown[:8], "  …"
	}
	return strings.Join(shown, "  ") + more
}

func (m *model) completeArg() (tea.Cmd, bool) {
	val := m.cmd.Value()
	if len(m.cmdCycle) > 1 && m.cmdBase != "" && strings.HasPrefix(val, m.cmdBase) {
		m.cycleArg(1)
		return nil, true
	}
	a := m.argCandidates(val)
	if !a.ok {
		return nil, false
	}
	m.cmdCycle, m.cmdCycleAt, m.cmdBase = nil, -1, ""
	switch len(a.toks) {
	case 0:
		m.cmdHint = suggestLine(nil, a.note, -1)
		return a.cmd, true
	case 1:
		m.cmd.SetValue(a.base + a.toks[0] + completionSpace(a))
		m.cmd.CursorEnd()
		m.cmdHint = a.toks[0]

		if strings.HasSuffix(m.cmd.Value(), " ") {
			next := m.liveHint()
			return tea.Batch(a.cmd, next), true
		}
		return a.cmd, true
	}
	pre := foldPrefix(a.toks)
	if cur := strings.TrimPrefix(val, a.base); len(pre) > len(cur) && strings.HasPrefix(strings.ToLower(pre), strings.ToLower(cur)) {
		m.cmd.SetValue(a.base + pre)
		m.cmd.CursorEnd()
	}
	m.cmdCycle, m.cmdCycleAt, m.cmdBase = a.toks, -1, a.base
	m.cmdHint = suggestLine(a.toks, "", -1)
	return a.cmd, true
}

func completionSpace(a argCompletion) string {
	if strings.HasPrefix(a.base, "xray ") && strings.Count(a.base, " ") == 1 {
		return " "
	}
	return ""
}

func (m *model) cycleArg(step int) tea.Cmd {
	if len(m.cmdCycle) == 0 || m.cmdBase == "" {
		a := m.argCandidates(m.cmd.Value())
		if !a.ok || len(a.toks) == 0 {
			if a.ok {
				m.cmdHint = suggestLine(nil, a.note, -1)
			}
			return a.cmd
		}
		m.cmdCycle, m.cmdCycleAt, m.cmdBase = a.toks, -1, a.base
	}
	n := len(m.cmdCycle)
	if m.cmdCycleAt < 0 && step < 0 {
		m.cmdCycleAt = 0
	}
	m.cmdCycleAt = ((m.cmdCycleAt+step)%n + n) % n
	m.cmd.SetValue(m.cmdBase + m.cmdCycle[m.cmdCycleAt])
	m.cmd.CursorEnd()
	m.cmdHint = suggestLine(m.cmdCycle, "", m.cmdCycleAt)
	return nil
}

func foldPrefix(ss []string) string {
	pre := ss[0]
	for _, s := range ss[1:] {
		for pre != "" && !strings.HasPrefix(strings.ToLower(s), strings.ToLower(pre)) {
			pre = pre[:len(pre)-1]
		}
	}
	return pre
}

package ui

import (
	"errors"
	"strings"

	"k2/internal/kustomize"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type nsCreatedMsg struct {
	pane  string
	token int
	verb  string
	dir   string
	err   error
}

func (m *model) askMissingNS(msg kOutMsg) bool {
	verb := m.subject
	if m.readOnly || m.kfile == "" || (verb != "diff" && verb != "apply") {
		return false
	}
	ns := kustomize.MissingNamespaces(msg.text)
	if len(ns) == 0 {
		return false
	}
	*m = m.askCreateMissingNS(verb, m.kfile, ns)
	return true
}

func (m model) askCreateMissingNS(verb, dir string, ns []string) model {
	label, joined := "namespace", ns[0]
	if len(ns) > 1 {
		label, joined = "namespaces", strings.Join(ns, ", ")
	}
	title := "+ create namespace"
	question := "create namespace " + ns[0] + "?"
	if len(ns) > 1 {
		title = "+ create namespaces"
		question = "create namespaces " + joined + "?"
	}
	ctx := m.context
	if m.dangerous() && ctx != "" {
		ctx += "  " + skull + " production"
	}
	return m.ask(askSpec{
		title:    title,
		question: question,
		details: []string{
			kindDetail("cluster", m.context),
			kindDetail("context", ctx),
			kindDetail(label, joined),
			kindDetail("then", "re-run kustomize "+verb),
		},
		danger: true,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			return m.createMissingNS(verb, dir, ns)
		},
	})
}

func (m model) createMissingNS(verb, dir string, ns []string) (tea.Model, tea.Cmd) {
	what := "creating namespace"
	if len(ns) > 1 {
		what = "creating namespaces"
	}
	spin := m.startLoad(what, false)
	run, kc, ctx, pane, token := m.k.Run, m.kubeconfig, m.context, m.logID, m.load.token
	return m, tea.Batch(spin, func() tea.Msg {
		for _, name := range ns {
			out, err := shell.Combined(run, "kubectl", "create", "namespace", name, "--kubeconfig", kc, "--context", ctx)
			if err != nil {
				text := strings.TrimSpace(string(out))
				if text == "" {
					text = err.Error()
				}
				return nsCreatedMsg{pane: pane, token: token, verb: verb, dir: dir, err: errors.New(text)}
			}
		}
		return nsCreatedMsg{pane: pane, token: token, verb: verb, dir: dir}
	})
}

func (m model) applyNSCreated(msg nsCreatedMsg) (tea.Model, tea.Cmd) {
	if msg.token != 0 && m.load.token == msg.token {
		m.load = load{}
	}
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		return m, m.notify(toneErr, "create namespace failed")
	}
	return m.runKust(msg.verb, msg.dir)
}

package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"k2/internal/config"
	"k2/internal/forward"
	"k2/internal/kustomize"
)

type askSpec struct {
	title    string
	question string
	details  []string
	danger   bool
	kind     string
	ns, name string

	onYes   func(m model) (tea.Model, tea.Cmd)
	cmd     tea.Cmd
	running string
}

func (m model) ask(s askSpec) model {
	d := m.newDialog("confirm", s.title, s.kind, s.ns, s.name)
	d.ask, d.pending, d.pendMsg = s.question, s.cmd, s.running
	d.details, d.danger, d.onYes = s.details, s.danger, s.onYes
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.dlg = d

	return m
}

func target(kind, ns, name string) string {
	if ns == "" {
		return kind + " " + name
	}
	return kind + " " + ns + "/" + name
}

func (m model) contextLine() string {
	s := "context " + m.context
	if m.dangerous() {
		s += "  " + skull + " production"
	}
	return s
}

func (m model) askDelete() (tea.Model, tea.Cmd) {
	if m.stage == "fwds" {
		return m.askStopForward()
	}
	kind, ns, name, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	short := kind
	if m.stage == "workpods" {
		short = "pod"
	}
	s := askSpec{
		title:    "✗ delete " + short,
		question: "delete " + short + "/" + name + "?",
		details:  []string{target(kind, ns, name), m.contextLine()},
		danger:   true, kind: kind, ns: ns, name: name,
		running: "deleting " + name + "…",
	}
	if m.stage == "workpods" {
		kc, ctx := m.kubeconfig, m.context
		s.cmd = func() tea.Msg {
			return editDoneMsg{what: "deleted " + name, err: m.k.DeletePod(kc, ctx, ns, name)}
		}
		s.details = append(s.details, "the pod's controller may start a new one")
	} else {
		s.cmd = m.kubectlAction("deleted "+kind+"/"+name, "delete", kind, name, "-n", ns)
	}
	return m.ask(s), nil
}

func forwardLine(f forward.Forward) string {
	kind := f.Kind
	if kind == "" {
		kind = "pod"
	}
	return fmt.Sprintf("localhost:%s → %s:%s  (%s)", f.Local, kind+"/"+f.Pod, f.Remote, f.Namespace+" · "+f.Context)
}

func (m model) askStopForward() (tea.Model, tea.Cmd) {
	sel := m.content.SelectedItem()
	if sel == nil || sel.(item).id == "" {
		return m, nil
	}
	pid, _ := strconv.Atoi(sel.(item).id)
	var f *forward.Forward
	for i := range m.forwards {
		if m.forwards[i].PID == pid {
			f = &m.forwards[i]
		}
	}
	details := []string{"pid " + strconv.Itoa(pid)}
	name := sel.(item).id
	if f != nil {
		details = []string{forwardLine(*f), "pid " + strconv.Itoa(pid)}
		name = f.Pod
	}
	return m.ask(askSpec{
		title: "■ stop port-forward", question: "stop and remove this forward?",
		details: details, danger: true, name: name,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.stopSelected()
			return m, m.notify(toneOK, m.status)
		},
	}), nil
}

func (m model) askStopAll() (tea.Model, tea.Cmd) {
	var details []string
	for i, f := range m.forwards {
		if i == 5 {
			details = append(details, fmt.Sprintf("… and %d more", len(m.forwards)-5))
			break
		}
		details = append(details, forwardLine(f))
	}
	return m.ask(askSpec{
		title:    "■ stop all port-forwards",
		question: "stop and remove all " + plural(len(m.forwards), "forward", "forwards") + "?",
		details:  details, danger: true,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.stopAll()
			return m, m.notify(toneOK, "all forwards stopped")
		},
	}), nil
}

type kcountMsg struct {
	seq int
	n   int
	ok  bool
}

func (m *model) askKust(verb string) (tea.Model, tea.Cmd) {
	if m.content.SelectedItem() == nil {
		m.status = "nothing selected"
		return *m, nil
	}
	dir, ok := m.selectedKust()
	if !ok {
		return m.notKust()
	}
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return *m, nil
	}
	m.rememberK()
	rel := dir
	if m.kroot != "" {
		if r, err := filepath.Rel(m.kroot, dir); err == nil && r != "" && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	title := "⇪ kustomize apply"
	if verb == "delete" {
		title = "✗ kustomize delete"
	}
	next := m.ask(askSpec{
		title:    title,
		question: verb + " " + rel + " on " + m.context + "?",
		details:  []string{config.HomePath(dir), m.contextLine(), "resources: counting…"},
		danger:   true,
		name:     rel,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			return m.runKust(verb, dir)
		},
	})
	seq, r := next.dlgSeq, m.k.Run
	return next, func() tea.Msg {
		n, ok := kustomize.Build(r, dir)
		return kcountMsg{seq: seq, n: n, ok: ok}
	}
}

func (m model) setKustCount(msg kcountMsg) model {
	if m.dlg == nil || m.dlg.seq != msg.seq || len(m.dlg.details) == 0 {
		return m
	}
	d := *m.dlg
	d.details = append([]string(nil), d.details...)
	line := "resources: " + plural(msg.n, "object", "objects")
	if !msg.ok {
		line = "resources: unknown (kubectl kustomize failed)"
	}
	d.details[len(d.details)-1] = line
	m.dlg = &d
	return m
}

func (m model) runKust(verb, dir string) (tea.Model, tea.Cmd) {
	m.kfile = dir
	m.subject = verb
	m.kback = m.stage
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "kout"
	m.status = verb + " " + config.HomePath(dir)
	spin := m.startLoad(map[string]string{"apply": "applying", "delete": "deleting"}[verb], false)
	return m, tea.Batch(spin, m.kustomizeCmd(verb, dir))
}

func (m model) askClosePane() model {
	details := []string{m.paneTitleLine()}
	danger := false
	if m.ed.live() {
		details = append(details, m.ed.tool()+" runs in this pane and is killed")
		danger = true
	}
	return m.ask(askSpec{
		title: "✗ close pane", question: "close pane?", details: details, danger: danger,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.closePane()
			return m, m.relayout()
		},
	})
}

func (m model) askCloseTab() model {
	n := len(m.tabIDs())
	details := []string{fmt.Sprintf("tab %d · %s", m.tab+1, plural(n, "pane", "panes"))}
	danger := false
	if e := len(m.editorsIn(m.tab)); e > 0 {
		details = append(details, "vim or a shell runs in "+plural(e, "pane", "panes")+" and is killed")
		danger = true
	}
	return m.ask(askSpec{
		title: "✗ close tab", question: fmt.Sprintf("close tab %d?", m.tab+1), details: details, danger: danger,
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.closeTab()
			return m, m.relayout()
		},
	})
}

func (m model) paneTitleLine() string {
	s := m.stage
	switch {
	case m.ed.live():
		s = m.ed.tool()
	case m.stage == "nav":
		s = "home"
	case strings.TrimSpace(m.content.Title) != "":
		s = strings.TrimSpace(m.content.Title)
	}
	if m.context != "" {
		s += " · " + m.context
	}
	return s
}

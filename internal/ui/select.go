package ui

import (
	"errors"
	"fmt"
	"strings"

	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type objRef struct {
	kind, ns, name string
}

func (m *model) toggleMark() {
	it, ok := m.selectedItem()
	if !ok || it.id == "" {
		return
	}
	next := map[string]bool{}
	for k, v := range m.vs.marks {
		if v {
			next[k] = true
		}
	}
	if next[it.id] {
		delete(next, it.id)
	} else {
		next[it.id] = true
	}
	if len(next) == 0 {
		next = nil
	}
	m.vs.marks = next
	if len(next) == 0 {
		m.status = "selection cleared"
	} else {
		m.status = fmt.Sprintf("%d selected · %s delete · %s logs", len(next), m.cfg.Keys.Delete.Label(), m.cfg.Keys.MultiLogs.Label())
	}
	m.content.CursorDown()
}

func (m model) markedNames() []string {
	var names []string
	for _, li := range m.content.Items() {
		it, ok := li.(item)
		if !ok || !m.vs.marks[it.id] {
			continue
		}
		_, name := kube.SplitRef(it.id, m.namespace)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (m model) markedRefs() []objRef {
	if len(m.vs.marks) == 0 {
		return nil
	}
	var out []objRef
	for _, li := range m.content.Items() {
		it, ok := li.(item)
		if !ok || !m.vs.marks[it.id] {
			continue
		}
		if ref, ok := m.refOf(it); ok {
			out = append(out, ref)
		}
	}
	return out
}

func (m model) refOf(it item) (objRef, bool) {
	if it.id == "" {
		return objRef{}, false
	}
	switch m.stage {
	case "workpods":
		ns, name := kube.SplitRef(it.id, m.namespace)
		return objRef{"pod", ns, name}, name != ""
	case "res":
		ns, name := kube.SplitRef(it.id, it.desc)
		if ns == "" {
			ns = m.namespace
		}
		return objRef{m.kind, ns, name}, m.kind != "" && name != ""
	default:
		return objRef{}, false
	}
}

func (m model) askDeleteRefs(refs []objRef) (tea.Model, tea.Cmd) {
	if len(refs) == 1 {
		r := refs[0]
		short := r.kind
		if short == "pod" {
			short = "pod"
		}
		return m.ask(askSpec{
			title: "✗ delete " + short, question: "delete " + short + "/" + r.name + "?",
			details: []string{target(r.kind, r.ns, r.name), m.contextLine()},
			danger:  true, kind: r.kind, ns: r.ns, name: r.name,
			running: "deleting " + r.name + "…",
			onYes: func(m model) (tea.Model, tea.Cmd) {
				m.vs.marks = nil
				return m, m.deleteRefs([]objRef{r})
			},
		}), nil
	}
	n := len(refs)
	kind := refs[0].kind
	details := []string{m.contextLine()}
	for i, r := range refs {
		if i == 8 {
			details = append(details, fmt.Sprintf("… and %d more", n-i))
			break
		}
		details = append(details, target(r.kind, r.ns, r.name))
	}
	return m.ask(askSpec{
		title:    "✗ delete " + plural(n, kind, kind),
		question: fmt.Sprintf("delete %d %s?", n, plural(n, kind, kind)),
		details:  details, danger: true,
		running: fmt.Sprintf("deleting %d…", n),
		onYes: func(m model) (tea.Model, tea.Cmd) {
			m.vs.marks = nil
			return m, m.deleteRefs(refs)
		},
	}), nil
}

func (m model) deleteRefs(refs []objRef) tea.Cmd {
	run, kc, ctx := m.k.Run, m.kubeconfig, m.context
	return func() tea.Msg {
		groups := map[string][]string{}
		var order []string
		for _, r := range refs {
			key := r.kind + "\x00" + r.ns
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], r.name)
		}
		var failed []string
		for _, key := range order {
			kind, ns, _ := strings.Cut(key, "\x00")
			args := append([]string{"delete", kind}, groups[key]...)
			if ns != "" {
				args = append(args, "-n", ns)
			}
			args = append(args, "--kubeconfig", kc, "--context", ctx)
			out, err := shell.Combined(run, "kubectl", args...)
			if err != nil {
				text := strings.TrimSpace(string(out))
				if text == "" {
					text = err.Error()
				}
				failed = append(failed, text)
			}
		}
		if len(failed) > 0 {
			return editDoneMsg{what: "deleted", err: errors.New(strings.Join(failed, "\n"))}
		}
		return editDoneMsg{what: "deleted " + plural(len(refs), "object", "objects")}
	}
}

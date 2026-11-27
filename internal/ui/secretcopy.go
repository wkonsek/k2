package ui

import (
	"strings"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

type secretNSMsg struct {
	pane, ns, name string
	names          []string
	err            error
}

type secretCheckMsg struct {
	pane, dest, name string
	exists           bool
	err              error
}

func (m model) startSecretCopy(ns, name string) (tea.Model, tea.Cmd) {
	k, kc, ctx, pane := m.k, m.kubeconfig, m.context, m.logID
	m.status = "loading namespaces…"
	return m, func() tea.Msg {
		names, err := k.Projects(kc, ctx)
		return secretNSMsg{pane: pane, ns: ns, name: name, names: names, err: err}
	}
}

func (m model) applySecretNS(msg secretNSMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		return m, m.notify(toneErr, "cannot list namespaces")
	}
	if m.readOnly || m.dlg != nil {
		return m, nil
	}
	m.dlg = m.secretPickDialog(msg.ns, msg.name, msg.names, "")
	m.status = "pick the target namespace"
	return m, nil
}

func (m model) secretPickDialog(ns, name string, pool []string, dest string) *dialog {
	d := m.newDialog("secretcopy", "⧉ copy secret "+name+" to namespace", "secret", ns, name).
		withField("", "filter namespaces", "copy "+ns+"/"+name+" to:")
	d.pool = pool
	d.filterNS()
	pick := dest
	if pick == "" {
		for _, n := range d.options {
			if n != ns {
				pick = n
				break
			}
		}
	}
	for i, n := range d.options {
		if n == pick {
			d.idx = i
		}
	}
	return d
}

func (d *dialog) filterNS() {
	keep := d.picked()
	q := strings.ToLower(strings.TrimSpace(d.input.Value()))
	d.options = nil
	for _, n := range d.pool {
		if q == "" || strings.Contains(strings.ToLower(n), q) {
			d.options = append(d.options, n)
		}
	}
	d.idx = 0
	for i, n := range d.options {
		if n == keep {
			d.idx = i
		}
	}
}

func (m model) secretCopyKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dlg
	key := msg.String()
	if d.dest == "" {
		switch {
		case len(d.options) > 0 && (key == "down" || key == "tab"):
			d.idx = (d.idx + 1) % len(d.options)
			return m, nil
		case len(d.options) > 0 && (key == "up" || key == "shift+tab"):
			d.idx = (d.idx - 1 + len(d.options)) % len(d.options)
			return m, nil
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	d.msg = ""
	if d.dest == "" {
		d.filterNS()
	}
	return m, cmd
}

func (m model) secretNameDialog(ns, name string, pool []string, dest, newName string) *dialog {
	d := m.newDialog("secretcopy", "⧉ copy secret "+name+" to "+dest, "secret", ns, name).
		withField(newName, name, "name in "+dest+" (enter keeps "+name+", esc picks another namespace)")
	d.pool, d.dest = pool, dest
	return d
}

func (m model) submitSecretCopy() (tea.Model, tea.Cmd) {
	d := m.dlg
	if d.dest == "" {
		dest := d.picked()
		if dest == "" {
			d.msg = "no namespace matches"
			return m, nil
		}
		m.dlg = m.secretNameDialog(d.ns, d.name, d.pool, dest, d.name)
		return m, nil
	}
	name := strings.TrimSpace(d.input.Value())
	if name == "" {
		name = d.name
	}
	if err := kube.ValidSecretName(name); err != nil {
		d.msg = err.Error()
		return m, nil
	}
	if d.dest == d.ns && name == d.name {
		d.msg = "that is the same secret: pick another namespace or name"
		return m, nil
	}
	k, kc, ctx, pane, dest := m.k, m.kubeconfig, m.context, m.logID, d.dest
	d.msg = "checking " + dest + "/" + name + "…"
	return m, func() tea.Msg {
		ok, err := k.SecretExists(kc, ctx, dest, name)
		return secretCheckMsg{pane: pane, dest: dest, name: name, exists: ok, err: err}
	}
}

func (m model) applySecretCheck(msg secretCheckMsg) (tea.Model, tea.Cmd) {
	d := m.dlg
	if d == nil || d.act != "secretcopy" || d.dest != msg.dest {
		return m, nil
	}
	if msg.err != nil {
		d.msg = shortErr(msg.err)
		return m, nil
	}
	ns, name, pool, dest, newName := d.ns, d.name, d.pool, msg.dest, msg.name
	question := "copy secret " + name + " to " + dest + "?"
	title := "⧉ copy secret"
	if msg.exists {
		question = "secret " + dest + "/" + newName + " exists · overwrite it?"
		title = "⧉ overwrite secret"
	}
	return m.ask(askSpec{
		title:    title,
		question: question,
		kind:     "secret", ns: dest, name: newName,
		details: []string{
			kindDetail("from", ns+"/"+name),
			kindDetail("to", dest+"/"+newName),
			kindDetail("context", m.context),
		},
		danger: msg.exists || m.dangerous(),
		onYes: func(m model) (tea.Model, tea.Cmd) {
			return m.runSecretCopy(ns, name, dest, newName, msg.exists)
		},
		onBack: func(m model) (tea.Model, tea.Cmd) {
			m.dlg = m.secretNameDialog(ns, name, pool, dest, newName)
			return m, nil
		},
	}), nil
}

func (m model) runSecretCopy(ns, name, dest, newName string, replace bool) (tea.Model, tea.Cmd) {
	k, kc, ctx := m.k, m.kubeconfig, m.context
	what := "copied secret " + name + " to " + dest
	if newName != name {
		what += "/" + newName
	}
	running := "copying secret…"
	m.status = running
	return m, m.runAction(running, func() tea.Msg {
		err := k.CopySecret(kc, ctx, ns, name, dest, newName, replace)
		return editDoneMsg{what: what, err: err, status: what}
	})
}

package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var lookPath = exec.LookPath

type helmMsg struct {
	pane     string
	token    int
	refresh  bool
	all      bool
	releases []kube.HelmRelease
	err      error
}

func (m model) helmCLI() string {
	if m.cfg.Helm.Binary == "" {
		return ""
	}
	if _, err := lookPath(m.cfg.Helm.Binary); err != nil {
		return ""
	}
	return m.cfg.Helm.Binary
}

func (m model) helmNS() string {
	if m.vs.helmAll {
		return ""
	}
	return m.namespace
}

func (m model) openHelm() (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	spin := m.startLoad("loading helm releases", true)
	return m, tea.Batch(spin, helmCmd(m.k, m.logID, m.load.token, false, m.vs.helmAll, m.kubeconfig, m.context, m.helmNS()))
}

func helmCmd(k kube.Client, pane string, token int, refresh, all bool, kc, ctx, ns string) tea.Cmd {
	return func() tea.Msg {
		secrets, err := k.HelmSecrets(kc, ctx, ns)
		msg := helmMsg{pane: pane, token: token, refresh: refresh, all: all, err: err}
		if err == nil {
			msg.releases = kube.HelmLatest(secrets)
		}
		return msg
	}
}

func (m *model) applyHelm(msg helmMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if msg.refresh {
			delete(m.rowBusy, msg.pane)
			if m.stage != "helm" || msg.err != nil || msg.all != m.vs.helmAll || m.content.FilterState() != list.Unfiltered {
				return
			}
			m.vs.helm = msg.releases
			m.showHelm("")
			return
		}
		if m.load.token != msg.token || m.stage != m.load.from {
			return
		}
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "could not read helm releases")
			return
		}
		if m.stage != "helm" {
			m.vs.sortCol, m.vs.sortDesc = "", false
			m.content.ResetFilter()
			m.content.SetItems(nil)
		}
		m.stage = "helm"
		m.vs.helm = msg.releases
		m.showHelm("")
		if len(msg.releases) == 0 {
			m.status = m.emptyHint()
			cmd = m.notify(toneWarn, m.status)
			return
		}
		m.status = m.helmHint()
	})
	return cmd
}

func (m model) helmHint() string {
	k := m.cfg.Keys
	hint := k.HelmValues.Label() + " values, " + k.HelmComputed.Label() + " computed, " + k.HelmManifest.Label() + " manifest, " +
		k.HelmNotes.Label() + " notes, " + k.HelmHistory.Label() + " history, " + k.AllNamespaces.Label() + " all namespaces"
	if m.helmCLI() != "" {
		hint += ", " + k.HelmRollback.Label() + " rollback, " + k.Delete.Label() + " uninstall"
	}
	return hint
}

func (m *model) showHelm(pick string) {
	all := m.helmNS() == ""
	t := kube.HelmTable(m.vs.helm, all)
	ns := m.helmNS()
	header, items, _ := packKube(t, m.tableWidth(), ns)
	m.content.SetShowTitle(false)
	setRows(&m.content, header, items, m.vs, pick)
}

func (m model) selectedRelease() (kube.HelmRelease, bool) {
	it, ok := m.selectedItem()
	if !ok {
		return kube.HelmRelease{}, false
	}
	ns, name := kube.SplitRef(it.id, it.desc)
	if ns == "" {
		ns = m.namespace
	}
	for _, r := range m.vs.helm {
		if r.Name == name && r.Namespace == ns {
			return r, true
		}
	}
	return kube.HelmRelease{}, false
}

func (m model) helmKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	switch {
	case km.AllNamespaces.Has(key):
		m.vs.helmAll = !m.vs.helmAll
		next, cmd := m.openHelm()
		return next, cmd, true
	case km.CopyName.Has(key), km.CopyRef.Has(key):
		r, ok := m.selectedRelease()
		if !ok {
			return m, nil, true
		}
		text := r.Name
		if km.CopyRef.Has(key) {
			text = "helm release " + r.Name + " -n " + r.Namespace
		}
		m.status = "copying " + text + "…"
		return m, m.copyCmd(text), true
	}
	if !(km.HelmValues.Has(key) || km.Select.Has(key) || km.HelmComputed.Has(key) || km.HelmManifest.Has(key) ||
		km.HelmNotes.Has(key) || km.HelmHistory.Has(key) || km.HelmRollback.Has(key) || km.Delete.Has(key)) {
		return m, nil, false
	}
	r, ok := m.selectedRelease()
	if !ok {
		return m, nil, true
	}
	ref := r.Namespace + "/" + r.Name
	switch {
	case km.HelmValues.Has(key), km.Select.Has(key):
		text := fmt.Sprintf("# user-supplied values of %s, revision %d\n%s", ref, r.Revision, kube.ValuesYAML(r.Config))
		return m.helmText("yaml", "values "+ref, text), nil, true
	case km.HelmComputed.Has(key):
		text := fmt.Sprintf("# computed values of %s, revision %d (chart defaults merged with the user-supplied values)\n%s",
			ref, r.Revision, kube.ValuesYAML(kube.MergeValues(r.Defaults, r.Config)))
		return m.helmText("yaml", "computed values "+ref, text), nil, true
	case km.HelmManifest.Has(key):
		return m.helmText("yaml", "manifest "+ref, or(r.Manifest, "# no manifest\n")), nil, true
	case km.HelmNotes.Has(key):
		return m.helmText("view", "notes "+ref, or(r.Notes, "no notes")), nil, true
	case km.HelmHistory.Has(key):
		next, cmd := m.helmHistory(r)
		return next, cmd, true
	case km.HelmRollback.Has(key):
		next, cmd := m.askHelm("rollback", r)
		return next, cmd, true
	default:
		next, cmd := m.askHelm("uninstall", r)
		return next, cmd, true
	}
}

func (m model) helmText(stage, subject, text string) model {
	m.subject = subject
	m.vs.vback = "helm"
	m.logText = text
	m.logStick, m.logAt = false, 0
	m.stage = stage
	m.status = subject + " · " + m.cfg.Keys.Back.Label() + " back"
	return m
}

func (m model) askHelm(act string, r kube.HelmRelease) (tea.Model, tea.Cmd) {
	bin := m.helmCLI()
	if bin == "" {
		why := "helm.binary is empty"
		if m.cfg.Helm.Binary != "" {
			why = m.cfg.Helm.Binary + " is not in PATH"
		}
		m.status = act + " needs the helm CLI (" + why + "); the view itself works without it"
		return m, m.notify(toneWarn, act+" needs the helm CLI")
	}
	args := append([]string{act, r.Name}, kube.HelmArgs(m.kubeconfig, m.context, r.Namespace)...)
	details := []string{"release " + r.Namespace + "/" + r.Name + " (revision " + strconv.Itoa(r.Revision) + ", " + or(r.Chart, "?") + ")", m.contextLine(), bin + " " + strings.Join(args[:2], " ")}
	spec := askSpec{kind: "helm", ns: r.Namespace, name: r.Name, danger: true, details: details}
	switch act {
	case "rollback":
		if r.Revision <= 1 {
			m.status = r.Name + " has no earlier revision"
			return m, m.notify(toneWarn, m.status)
		}
		spec.title = "↶ helm rollback"
		spec.question = fmt.Sprintf("roll back %s to revision %d?", r.Name, r.Revision-1)
		spec.running = "rolling back…"
		spec.cmd = m.helmAction("rolled back "+r.Name, bin, args)
	default:
		spec.title = "✗ helm uninstall"
		spec.question = "uninstall release " + r.Name + "?"
		spec.running = "uninstalling…"
		spec.cmd = m.helmAction("uninstalled "+r.Name, bin, args)
	}
	return m.ask(spec), nil
}

func (m model) helmAction(what, bin string, args []string) tea.Cmd {
	run := m.run
	return func() tea.Msg {
		out, err := shell.Combined(run, bin, args...)
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return editDoneMsg{what: what, err: errors.New(text)}
		}
		return editDoneMsg{what: what}
	}
}

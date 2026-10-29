package ui

import (
	"fmt"
	"strconv"
	"strings"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

type helmHist struct {
	ns, name string
	revs     []kube.HelmRelease
	mark     int
}

type helmHistMsg struct {
	pane     string
	token    int
	ns, name string
	revs     []kube.HelmRelease
	err      error
}

func (h *helmHist) current() kube.HelmRelease {
	for _, r := range h.revs {
		if r.Status == "deployed" {
			return r
		}
	}
	if len(h.revs) > 0 {
		return h.revs[0]
	}
	return kube.HelmRelease{}
}

func (h *helmHist) rev(n int) (kube.HelmRelease, bool) {
	for _, r := range h.revs {
		if r.Revision == n {
			return r, true
		}
	}
	return kube.HelmRelease{}, false
}

func (m model) helmHistory(r kube.HelmRelease) (tea.Model, tea.Cmd) {
	k, kc, ctx, pane := m.k, m.kubeconfig, m.context, m.logID
	spin := m.startLoad("reading history", true)
	token := m.load.token
	fetch := func() tea.Msg {
		secrets, err := k.ObjectsWhere(kc, ctx, "secrets", r.Namespace, "owner=helm,name="+r.Name)
		msg := helmHistMsg{pane: pane, token: token, ns: r.Namespace, name: r.Name, err: err}
		if err == nil {
			msg.revs = kube.HelmHistory(secrets, r.Namespace, r.Name)
		}
		return msg
	}
	return m, tea.Batch(spin, fetch)
}

func (m *model) applyHelmHist(msg helmHistMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		delete(m.rowBusy, msg.pane)
		if msg.token == 0 {
			if m.stage != "helmhist" || m.vs.hist == nil || m.vs.hist.name != msg.name {
				return
			}
		} else if m.load.token != msg.token || m.stage != m.load.from {
			return
		}
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "could not read the history of "+msg.name)
			return
		}
		mark := 0
		if h := m.vs.hist; h != nil && h.ns == msg.ns && h.name == msg.name {
			mark = h.mark
		}
		m.vs.hist = &helmHist{ns: msg.ns, name: msg.name, revs: msg.revs, mark: mark}
		m.content.ResetFilter()
		m.content.SetItems(nil)
		m.stage = "helmhist"
		m.showHist("")
		m.status = m.histHint()
	})
	return cmd
}

func (m model) histHint() string {
	k := m.cfg.Keys
	hint := "history of " + m.vs.hist.ns + "/" + m.vs.hist.name + " · " + keyName(k.HelmMark.Short()) + " mark · " + k.HelmDiff.Short() + " diff"
	if m.helmCLI() != "" {
		hint += " · " + k.HelmRollback.Short() + " roll back to it"
	}
	return hint + " · " + k.Back.Short() + " back"
}

func (m *model) showHist(pick string) {
	h := m.vs.hist
	if h == nil {
		return
	}
	cur := h.current().Revision
	headers := []string{"", "REVISION", "STATUS", "CHART", "APP VERSION", "UPDATED", "DESCRIPTION"}
	cells := make([][]string, len(h.revs))
	refs := make([][2]string, len(h.revs))
	for i, r := range h.revs {
		mark := ""
		switch {
		case r.Revision == cur:
			mark = "●"
		case r.Revision == h.mark:
			mark = "◆"
		}
		cells[i] = []string{mark, strconv.Itoa(r.Revision), or(r.Status, "-"), or(r.Chart, "-"), or(r.AppVersion, "-"), helmUpdated(r), r.Description}
		refs[i] = [2]string{r.Namespace, strconv.Itoa(r.Revision)}
	}
	header, items, _ := packTable(headers, cells, refs, nil, 2, m.tableWidth(), h.ns)
	for i, r := range h.revs {
		it := items[i].(item)
		it.id = "rev/" + strconv.Itoa(r.Revision)
		items[i] = it
	}
	m.content.SetShowTitle(false)
	vs := m.vs
	vs.sortCol = ""
	setRows(&m.content, header, items, vs, pick)
}

func helmUpdated(r kube.HelmRelease) string {
	if r.Updated.IsZero() {
		return "-"
	}
	return r.Updated.Local().Format("2006-01-02 15:04")
}

func (m model) selectedRev() (kube.HelmRelease, bool) {
	it, ok := m.selectedItem()
	if !ok || m.vs.hist == nil {
		return kube.HelmRelease{}, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(it.id, "rev/"))
	if err != nil {
		return kube.HelmRelease{}, false
	}
	return m.vs.hist.rev(n)
}

func (m model) histKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	h := m.vs.hist
	if h == nil {
		return m, nil, false
	}
	r, sel := m.selectedRev()
	ref := h.ns + "/" + h.name
	switch {
	case km.HelmMark.Has(key):
		if !sel {
			return m, nil, true
		}
		next := *h
		if next.mark == r.Revision {
			next.mark = 0
			m.status = "mark cleared"
		} else {
			next.mark = r.Revision
			m.status = fmt.Sprintf("revision %d marked · %s on another revision diffs them", r.Revision, km.HelmDiff.Short())
		}
		m.vs.hist = &next
		m.showHist("rev/" + strconv.Itoa(r.Revision))
		return m, nil, true
	case km.HelmDiff.Has(key):
		if !sel {
			return m, nil, true
		}
		other := h.current()
		if h.mark != 0 && h.mark != r.Revision {
			other, _ = h.rev(h.mark)
		}
		if other.Revision == r.Revision {
			m.status = fmt.Sprintf("revision %d is the current one: mark another with %s, or select an older one", r.Revision, keyName(km.HelmMark.Short()))
			return m, m.notify(toneWarn, "nothing to diff against"), true
		}
		a, b := r, other
		if a.Revision > b.Revision {
			a, b = b, a
		}
		m.subject = fmt.Sprintf("diff %s %d→%d", ref, a.Revision, b.Revision)
		m.vs.vback = "helmhist"
		m.logText = kube.RevisionDiff(a, b)
		m.logStick, m.logAt = false, 0
		m.stage = "hdiff"
		m.status = m.subject + " · " + km.Search.Short() + " search · " + km.Back.Short() + " back"
		return m, nil, true
	case km.HelmRollback.Has(key):
		if !sel {
			return m, nil, true
		}
		next, cmd := m.askRollbackTo(h.current(), r)
		return next, cmd, true
	case km.HelmValues.Has(key), km.Select.Has(key):
		if !sel {
			return m, nil, true
		}
		return m.histText("yaml", fmt.Sprintf("values %s rev %d", ref, r.Revision), kube.ValuesYAML(r.Config)), nil, true
	case km.HelmComputed.Has(key):
		if !sel {
			return m, nil, true
		}
		return m.histText("yaml", fmt.Sprintf("computed values %s rev %d", ref, r.Revision), kube.ValuesYAML(kube.MergeValues(r.Defaults, r.Config))), nil, true
	case km.HelmManifest.Has(key):
		if !sel {
			return m, nil, true
		}
		return m.histText("yaml", fmt.Sprintf("manifest %s rev %d", ref, r.Revision), or(r.Manifest, "# no manifest\n")), nil, true
	case km.HelmNotes.Has(key):
		if !sel {
			return m, nil, true
		}
		return m.histText("view", fmt.Sprintf("notes %s rev %d", ref, r.Revision), or(r.Notes, "no notes")), nil, true
	case km.CopyName.Has(key):
		if !sel {
			return m, nil, true
		}
		text := h.name + " " + strconv.Itoa(r.Revision)
		m.status = "copying " + text + "…"
		return m, m.copyCmd(text), true
	}
	return m, nil, false
}

func (m *model) histRefreshNow() {
	h := m.vs.hist
	if h == nil {
		return
	}
	secrets, err := m.k.ObjectsWhere(m.kubeconfig, m.context, "secrets", h.ns, "owner=helm,name="+h.name)
	if err != nil {
		m.status = m.errLine(err)
		return
	}
	next := *h
	next.revs = kube.HelmHistory(secrets, h.ns, h.name)
	m.vs.hist = &next
	m.showHist("")
}

func (m model) histText(stage, subject, text string) model {
	m = m.helmText(stage, subject, text)
	m.vs.vback = "helmhist"
	return m
}

func (m model) askRollbackTo(cur, target kube.HelmRelease) (tea.Model, tea.Cmd) {
	if target.Revision == cur.Revision {
		m.status = fmt.Sprintf("revision %d is the current one: select an older revision to roll back to", target.Revision)
		return m, m.notify(toneWarn, fmt.Sprintf("revision %d is current", target.Revision))
	}
	bin, why := m.helmBinary()
	if bin == "" {
		m.status = "rollback needs the helm CLI (" + why + "); the history and diffs work without it"
		return m, m.notify(toneWarn, "rollback needs the helm CLI")
	}
	rev := strconv.Itoa(target.Revision)
	args := append([]string{"rollback", target.Name, rev}, kube.HelmArgs(m.kubeconfig, m.context, target.Namespace)...)
	ver := func(r kube.HelmRelease) string { return or(r.Chart, "?") + " (app " + or(r.AppVersion, "?") + ")" }
	spec := askSpec{
		kind: "helm", ns: target.Namespace, name: target.Name, danger: true,
		title:    "↶ helm rollback",
		question: fmt.Sprintf("roll back %s from revision %d to revision %d?", target.Name, cur.Revision, target.Revision),
		details: []string{
			"release " + target.Namespace + "/" + target.Name,
			fmt.Sprintf("revision %d → %d", cur.Revision, target.Revision),
			"chart " + ver(cur) + " → " + ver(target),
			m.contextLine(),
			bin + " rollback " + target.Name + " " + rev,
		},
		running: "rolling back…",
		cmd:     m.helmAction(fmt.Sprintf("rolled back %s to revision %d", target.Name, target.Revision), bin, args),
	}
	return m.ask(spec), nil
}

func (m model) helmBinary() (string, string) {
	if bin := m.helmCLI(); bin != "" {
		return bin, ""
	}
	if m.cfg.Helm.Binary == "" {
		return "", "helm.binary is empty"
	}
	return "", m.cfg.Helm.Binary + " is not in PATH"
}

func colorHelmDiff(l string) string {
	switch {
	case strings.HasPrefix(l, "=== "):
		return titleStyle.Render(l)
	case strings.HasPrefix(l, "@@"), strings.HasPrefix(l, "# "):
		return dimStyle.Render(l)
	case strings.HasPrefix(l, "+"):
		return okStyle.Render(l)
	case strings.HasPrefix(l, "-"):
		return errStyle.Render(l)
	}
	return l
}

package ui

import (
	"os/exec"
	"strings"
	"time"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) selectedItem() (item, bool) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return item{}, false
	}
	it, ok := sel.(item)
	return it, ok && it.id != ""
}

func (m model) tableAction(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	table := m.stage == "workpods" || m.stage == "res"
	base := kube.BaseKind(m.kind)
	if m.stage == "workpods" {
		base = "pods"
	}
	switch {
	case m.stage == "logs" && km.Previous.Has(key):
		next, cmd := m.openLogs(m.logPod, m.vs.logCont, !m.vs.logPrev)
		return next, cmd, true
	case !table:
		return m, nil, false
	case km.SortNext.Has(key), km.SortPrev.Has(key):
		step := 1
		if km.SortPrev.Has(key) {
			step = -1
		}
		m.vs.sortCol = cycleSort(sortHeads(m.content), m.vs.sortCol, step)
		m.resort()
		return m, nil, true
	case km.SortReverse.Has(key):
		if m.vs.sortCol == "" {
			heads := sortHeads(m.content)
			if len(heads) == 0 {
				return m, nil, true
			}
			m.vs.sortCol = heads[flexIndex(heads)]
		}
		m.vs.sortDesc = !m.vs.sortDesc
		m.resort()
		return m, nil, true
	case km.CopyName.Has(key), km.CopyRef.Has(key):
		kind, ns, name, ok := m.selectedRef()
		if !ok {
			return m, nil, true
		}
		text := name
		if km.CopyRef.Has(key) {
			text = kind + "/" + name
			if ns != "" {
				text += " -n " + ns
			}
		}

		m.status = "copying " + text + "…"
		return m, m.copyCmd(text), true
	}
	kind, ns, name, ok := m.selectedRef()
	if !ok {
		return m, nil, false
	}
	it, _ := m.selectedItem()
	switch {
	case km.Shell.Has(key) && base == "pods":
		if picked, cmd, asked := m.pickContainer("shell", it, ns, name); asked {
			return picked, cmd, true
		}
		next, cmd := m.openShell(ns, name, "")
		return next, cmd, true
	case km.Scale.Has(key) && (base == "deployments" || base == "statefulsets" || base == "replicasets"):
		d := m.newDialog("scale", "⇅ scale "+kind+"/"+name, kind, ns, name).
			withField(or(it.extra["replicas"], "1"), "replicas", "replicas")
		m.dlg = d
		return m, nil, true
	case km.SetImage.Has(key) && setImageKinds[base]:
		next, cmd := m.openSetImage(kind, ns, name)
		return next, cmd, true
	case km.Restart.Has(key) && (base == "deployments" || base == "statefulsets" || base == "daemonsets"):
		what := "restarted " + kind + "/" + name
		cmd := m.kubectlAction(what, "rollout", "restart", kind+"/"+name, "-n", ns)
		return m.confirmDialog("↻ rollout restart", "rollout restart "+kind+"/"+name+"?", kind, ns, name, "restarting…", cmd), nil, true
	case km.Trigger.Has(key) && base == "cronjobs":
		job := manualJobName(name, time.Now())
		what := "created job " + job
		cmd := m.kubectlAction(what, "create", "job", "--from=cronjob/"+name, job, "-n", ns)
		return m.confirmDialog("▶ trigger cronjob", "run cronjob/"+name+" now as job "+job+"?", kind, ns, name, "creating job…", cmd), nil, true
	case km.Approve.Has(key) && base == "installplans":
		next, cmd := m.approvePlan(it, kind, ns, name)
		return next, cmd, true
	case km.Label.Has(key), km.Annotate.Has(key):
		act, icon := "label", "🏷"
		if km.Annotate.Has(key) {
			act, icon = "annotate", "✎"
		}
		m.dlg = m.newDialog(act, icon+" "+act+" "+kind+"/"+name, kind, ns, name).
			withField("", "key=value  or  key-", "key=value sets, key- removes (space separated)")
		return m, nil, true
	}
	return m, nil, false
}

func manualJobName(cron string, now time.Time) string {
	suffix := "-manual-" + now.Format("0102150405")
	if max := 52 - len(suffix); len(cron) > max {
		cron = strings.TrimRight(cron[:max], "-.")
	}
	return cron + suffix
}

func (m model) pickContainer(act string, it item, ns, name string) (tea.Model, tea.Cmd, bool) {
	var opts, notes []string
	for _, c := range strings.Split(it.extra["containers"], ",") {
		if c != "" {
			opts, notes = append(opts, c), append(notes, "")
		}
	}
	for _, c := range strings.Split(it.extra["init"], ",") {
		if c != "" {
			opts, notes = append(opts, c), append(notes, "init")
		}
	}
	if len(opts) < 2 {
		return m, nil, false
	}
	title := map[string]string{"logs": "▤ logs", "prevlogs": "▤ previous logs", "shell": "$ shell"}[act] + " · container"
	d := m.newDialog(act, title, "pod", ns, name)
	d.options, d.notes = opts, notes
	m.dlg = d
	m.status = "pick a container"
	return m, nil, true
}

func (m model) openLogs(podRef, container string, prev bool) (tea.Model, tea.Cmd) {
	m.logPod = podRef
	m.vs.logCont, m.vs.logPrev = container, prev
	m.logText = ""
	m.logSince = time.Time{}
	m.vs.logMarks = nil
	m.stage = "logs"
	m.logStick = true
	m.logAt = 0
	m.status = m.scrollKeys() + " scroll    " + m.cfg.Keys.CopyMode.Short() + " copy    " + m.cfg.Keys.LogMark.Short() + " mark    " + m.cfg.Keys.Vim.Short() + " vim    " + m.cfg.Keys.Previous.Short() + " previous"
	if prev {
		m.status = "previous log (not followed) · " + m.cfg.Keys.Previous.Short() + " back to the live log"
	}
	m.logBusy = true
	return m, tea.Batch(m.startLoad("loading logs", false), m.fetchLogs())
}

const shellScript = "command -v bash >/dev/null && exec bash || exec sh"

func shellArgs(kubeconfig, context, ns, pod, container string) []string {
	args := []string{"exec", "-it", pod, "-n", ns, "--kubeconfig", kubeconfig, "--context", context}
	if container != "" {
		args = append(args, "-c", container)
	}
	return append(args, "--", "sh", "-c", shellScript)
}

type shellDoneMsg struct{ err error }

func (m model) openShell(ns, pod, container string) (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	label := "shell " + pod
	if container != "" {
		label += ":" + container
	}
	args := shellArgs(m.kubeconfig, m.context, ns, pod, container)
	return m.runEditor("shell", label, pod, "",
		func() *exec.Cmd { return exec.Command("kubectl", args...) },
		func(err error) tea.Msg { return shellDoneMsg{err: err} })
}

func (m model) drill(stage, kind, sel, title string) (tea.Model, tea.Cmd) {
	it, _ := m.selectedItem()
	m.vs.parent, m.vs.parentSel = m.kind, it.id
	m.vs.podSel, m.vs.podTitle = sel, title
	return m.openList(stage, kind)
}

func (m model) backToParent() (tea.Model, tea.Cmd) {
	kind, sel := m.vs.parent, m.vs.parentSel
	m.vs.parent, m.vs.parentSel, m.vs.podSel, m.vs.podTitle = "", "", "", ""
	m.vs.pick = sel
	return m.openList("res", kind)
}

func (m model) enterRow(it item) (tea.Model, tea.Cmd, bool) {
	ns, name := kube.SplitRef(it.id, it.desc)
	switch kube.BaseKind(m.kind) {
	case "subscriptions":

		csv := it.extra["csv"]
		if csv == "" {
			m.status = "no CSV yet for " + name
			return m, m.notify(toneWarn, m.status), true
		}
		pick := csv
		if m.namespace == "" && ns != "" {
			pick = ns + "/" + csv
		}
		next, cmd := m.drill("res", "clusterserviceversions.operators.coreos.com", "", "")
		nm := next.(model)
		nm.vs.pick = pick
		return nm, cmd, true
	case "clusterserviceversions":
		return m.showCSV(ns, name)
	case "customresourcedefinitions":
		res := it.extra["resource"]
		if res == "" || res == "." {
			return m, nil, false
		}
		next, cmd := m.drill("res", res, "", "")
		return next, cmd, true
	case "jobs":

		_ = ns
		next, cmd := m.drill("workpods", "pods", "job-name="+name, "job/"+name)
		return next, cmd, true
	}
	return m, nil, false
}

func (m model) showCSV(ns, name string) (tea.Model, tea.Cmd, bool) {
	if ns == "" {
		ns = m.namespace
	}
	m.subject = "csv " + name
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "view"
	m.vs.vback = ""
	m.status = "csv " + name + " · " + m.cfg.Keys.Back.Label() + " back"
	k, kc, ctx, pane, kind := m.k, m.kubeconfig, m.context, m.logID, m.kind
	fetch := func() tea.Msg {
		o, err := k.Object(kc, ctx, kind, ns, name)
		if err != nil {
			return viewMsg{id: pane, err: err, what: "csv " + name}
		}
		return viewMsg{id: pane, text: kube.CSVSummary(o), what: "csv " + name}
	}
	return m, tea.Batch(m.startLoad("loading csv", false), fetch), true
}

func (m model) approvePlan(it item, kind, ns, name string) (tea.Model, tea.Cmd) {
	if it.extra["approved"] == "true" {
		m.status = name + " is already approved"
		return m, m.notify(toneWarn, m.status)
	}
	if it.extra["approval"] != "Manual" {
		m.status = name + " is not a Manual plan (approval: " + or(it.extra["approval"], "-") + ")"
		return m, m.notify(toneWarn, "not a Manual plan")
	}
	cmd := m.kubectlAction("approved "+name, "patch", "installplan", name, "-n", ns, "--type", "merge", "-p", approvePatch)
	return m.confirmDialog("✓ approve installplan", "approve installplan "+name+" on "+m.context+"?", kind, ns, name, "approving…", cmd), nil
}

const approvePatch = `{"spec":{"approved":true}}`

package ui

import (
	"os"
	"path/filepath"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) activateNav() (tea.Model, tea.Cmd) {
	sel := m.nav.SelectedItem()
	if sel == nil {
		return m, nil
	}
	nav := sel.(navItem)

	switch nav.id {
	case "kube":
		files, err := getKubeFiles()
		if err != nil {
			m.status = m.errLine(err)
			return m, nil
		}
		m.content.ResetFilter()
		m.content.Title = "Kubeconfig"
		m.content.Select(0)
		m.stage = "files"
		m.status = "choose a kubeconfig"
		return m, m.content.SetItems(files)

	case "ctx":
		m.noteNS()
		if m.kubeconfig == "" {
			m.status = "select a kubeconfig first"
			return m, nil
		}
		contexts, err := m.k.Contexts(m.kubeconfig)
		if err != nil {
			m.status = m.errLine(err)
			return m, nil
		}
		items := make([]list.Item, len(contexts))
		for i, c := range contexts {
			items[i] = item{title: c}
		}
		m.content.ResetFilter()
		m.content.Title = "Context"
		m.content.Select(0)
		m.stage = "contexts"
		m.status = "choose a context"
		return m, m.content.SetItems(items)

	case "proj":
		m.noteNS()
		return m.openList("projects", "namespaces")

	case "pods":
		return m.openList("workpods", "pods")

	case "pf":

		return m.openList("workpods", "pods")

	case "fwds":
		m.status = ""
		return m, m.showForwards()

	case "kind":
		return m.openKind()

	case "kust":
		return m, m.showKDir(m.kroot)

	case "sh":
		m.stage = "kcmd"
		m.jqOn = false
		m.sh.Focus()
		m.jq.Blur()
		m.shHint = m.cfg.Keys.Complete.Label() + " completes"
		m.status = "kubectl " + or(m.context, "no context")
		return m, nil
	}
	return m, nil
}

func (m model) handleContentEnter() (tea.Model, tea.Cmd) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return m, nil
	}
	it := sel.(item)

	switch m.stage {
	case "files":
		m.kubeconfig = it.title
		m.context = m.k.CurrentContext(m.kubeconfig)
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
		m.clearHist()
		m.stage = "nav"
		m.status = config.HomePath(m.kubeconfig)
		m.save()
	case "kdir":
		if it.desc == "kust-file" || it.desc == "plain-file" {
			if next, cmd, blocked := m.readOnlyBlock("edit"); blocked {
				return next, cmd
			}
			m.status = "opening vim…"
			return m.editKustomization()
		}
		return m, m.showKDir(it.id)
	case "contexts":
		if m.context != it.title {
			m.clearHist()
		}
		m.context = it.title
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
		m.save()
		next, cmd := m.resumeNS()
		nm := next.(model)
		if nm.kubeconfig != "" && nm.context != "" {
			return nm, tea.Batch(cmd, fetchResources(nm.k, nm.kubeconfig, nm.context))
		}
		return nm, cmd
	case "projects":
		name := it.id
		if name == "" {
			name = strings.TrimPrefix(it.title, markSquare+" ")
		}
		ctx, ns, err := m.k.UseProject(m.kubeconfig, m.context, name)
		if err != nil {
			m.status = m.errLine(err)
			m.content.Title = m.status
			return m, nil
		}
		if m.context != ctx {
			m.clearHist()
		}
		m.context = ctx
		m.namespace = ns
		m.save()
		return m.resumeNS()
	case "workpods":
		if m.inContainers() {
			next, cmd, _ := m.containerKey("enter")
			return next, cmd
		}
		return m.enterPod()
	case "res":
		if next, cmd, ok := m.enterRow(it); ok {
			return next, cmd
		}
		ns, name := kube.SplitRef(it.id, it.desc)
		if ns == "" {
			ns = m.namespace
		}
		if ns != "" {
			m.subject = ns + "/" + name
		} else {
			m.subject = name
		}
		m.logText = ""
		m.logStick = false
		m.logAt = 0
		m.stage = "yaml"
		m.vs.vback = ""
		m.armWatch(m.kind, ns, name)
		m.status = m.subject
		return m, tea.Batch(m.startLoad("loading yaml", false), m.fetchYAML(it.id, it.desc))
	}
	return m, nil
}

func (m model) openNamed(id string) (tea.Model, tea.Cmd) {
	for i, raw := range m.nav.Items() {
		if raw.(navItem).id == id {
			m.nav.Select(i)
			return m.activateNav()
		}
	}
	if id == "pods" {
		return m.openPods()
	}
	m.status = "unknown: " + id
	return m, nil
}

func (m model) openPods() (tea.Model, tea.Cmd) { return m.openList("workpods", "pods") }

func (m model) openResource(kind string) (tea.Model, tea.Cmd) { return m.openList("res", kind) }

func (m model) openCommandPods() (tea.Model, tea.Cmd) {
	for i, raw := range m.nav.Items() {
		if raw.(navItem).id == "pods" {
			m.nav.Select(i)
			break
		}
	}
	return m.openCommandList("workpods", "pods")
}

func (m model) openCommandList(stage, kind string) (tea.Model, tea.Cmd) {
	drilled := m.vs.parent != "" || m.vs.podSel != ""
	if !m.histSkip && (drilled || m.stage != stage || m.kind != kind) {
		m.rememberCommandSource()
	}
	m.resetDrill()
	m.histSkip = true
	next, cmd := m.openList(stage, kind)
	nm := next.(model)
	nm.histSkip = false
	return nm, cmd
}

func (m *model) rememberCommandSource() {
	if _, _, ok := parseContainerSel(m.vs.podSel); ok && m.stage != "workpods" {
		saved := m.stage
		m.stage = "workpods"
		m.rememberView()
		m.stage = saved
		return
	}
	m.rememberView()
}

func (m *model) resetDrill() {
	if ns, pod, ok := parseContainerSel(m.vs.podSel); ok {
		m.vs.pick = pod
		if m.namespace == "" {
			m.vs.pick = ns + "/" + pod
		}
		m.vs.sortCol, m.vs.sortDesc, m.vs.marks = m.vs.sortBack.col, m.vs.sortBack.desc, m.vs.sortBack.marks
	} else if m.vs.pick == "" {
		if it, ok := m.selectedItem(); ok {
			m.vs.pick = it.id
		}
	}
	m.vs.podSel, m.vs.podTitle = "", ""
	m.vs.podSelBack, m.vs.podTitleBack = "", ""
	m.vs.sortBack = podSort{}
	m.vs.parent, m.vs.parentSel = "", ""
}

func namespaceItems(projects []string, current string) []list.Item {
	allTitle := "all namespaces"
	if current == "" {
		allTitle = markSquare + " all namespaces"
	}
	items := []list.Item{item{title: allTitle, id: "*"}}
	for _, p := range projects {
		title := p
		if p == current {
			title = markSquare + " " + p
		}
		items = append(items, item{title: title, id: p})
	}
	return items
}

func (m *model) showProjects(projects []string, err error) tea.Cmd {
	if err != nil {
		m.status = m.errLine(err)
		if text, ok := m.listFailure(err.Error()); ok {
			m.status = text
		}
	}
	items := namespaceItems(projects, m.namespace)
	m.content.ResetFilter()
	m.content.Title = "Namespace"
	m.content.Select(0)
	m.stage = "projects"
	if m.status == "" {
		m.status = "default namespace"
	}
	return m.content.SetItems(items)
}

func (m model) jumpTarget(key string) string {
	km := m.cfg.Keys
	switch {
	case km.Kubeconfig.Has(key):
		return "kube"
	case km.Context.Has(key):
		return "ctx"
	case km.Namespace.Has(key):
		return "proj"
	case km.Forwards.Has(key):
		return "fwds"
	case km.Kustomize.Has(key):
		return "kust"
	case km.Kubectl.Has(key):
		return "sh"
	}
	return ""
}

func (m model) jump(key string) (tea.Model, tea.Cmd) {
	id := m.jumpTarget(key)
	if id == "" {
		return m, nil
	}
	for i, raw := range m.nav.Items() {
		if raw.(navItem).id == id {
			m.nav.Select(i)
			return m.activateNav()
		}
	}
	return m, nil
}

func (m *model) back() tea.Cmd {
	switch m.stage {
	case "yaml", "view":
		m.stage = "res"
		m.vs.secret = ""
		m.status = or(m.kind, "resources")
	case "describe":
		m.stage = or(m.dback, "res")
		m.status = or(m.kind, "resources")
		if m.stage == "workpods" {
			m.status = "pods"
		}
	case "logs":
		m.stage = "workpods"
		m.status = "pods"
	case "kout":
		switch m.kback {
		case "kdir":
			return m.showKDir(m.kroot)
		case "kcmd":
			m.stage = "kcmd"
			m.sh.Focus()
			m.status = "kubectl"
		default:
			m.stage = "kust"
			m.status = "kustomize"
		}
	case "kdir":
		parent := filepath.Dir(m.kroot)
		if m.kroot == "" || parent == m.kroot {
			m.stage = "nav"
			m.status = m.context
			return nil
		}
		return m.showKDir(parent)
	case "kcmd":
		m.stage = "nav"
		m.jqOn = false
		m.sh.Blur()
		m.jq.Blur()
		m.status = m.context
	case "projects", "contexts":
		if cmd, ok := m.reopenList(); ok {
			return cmd
		}
		m.stage = "nav"
		m.status = m.context
	case "kust":
		return m.showKDir(m.kroot)
	default:
		m.stage = "nav"
		m.status = m.context
	}
	return nil
}

func (m *model) noteNS() {
	switch m.stage {
	case "projects", "contexts", "files":
		return
	case "nav":
		m.vs.listFrom = ""
	default:
		m.vs.listFrom = m.stage
	}
}

func (m model) resumeNS() (tea.Model, tea.Cmd) {
	from := m.vs.listFrom
	m.vs.listFrom = ""
	m.status = m.namespace
	if m.status == "" {
		m.status = "all namespaces"
	}
	switch from {
	case "", "nav":
		m.stage = "nav"
		return m, nil
	case "workpods", "logs":
		return m.openList("workpods", "pods")
	case "res", "yaml", "describe", "view":
		if m.kind == "" {
			m.stage = "nav"
			return m, nil
		}
		return m.openList("res", m.kind)
	case "pulse":
		if m.namespace == "" {
			m.stage = "nav"
			m.status = "pulse needs a namespace"
			return m, m.notify(toneWarn, m.status)
		}
		return m.openPulse()
	case "helm", "helmhist", "hdiff", "htpl":
		return m.openHelm()
	case "kind":
		return m.openKind()
	case "mlogs":
		if m.vs.ml != nil {
			return m.openMultiLogs(m.vs.ml.q, m.vs.ml.title, m.vs.ml.from)
		}
		m.stage = "nav"
		return m, nil
	case "fwds":
		return m, m.showForwards()
	case "kdir", "kust":
		return m, m.showKDir(m.kroot)
	case "kcmd":
		m.stage = "kcmd"
		return m, nil
	case "xray":
		m.stage = "xray"
		return m, nil
	default:
		m.stage = from
		return m, nil
	}
}

func (m *model) reopenList() (tea.Cmd, bool) {
	if m.vs.listFrom == "" {
		return nil, false
	}
	next, cmd := m.resumeNS()
	*m = next.(model)
	return cmd, true
}

func getKubeFiles() ([]list.Item, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	names, err := kube.KubeFiles(filepath.Join(home, ".kube"))
	if err != nil {
		return nil, err
	}
	files := make([]list.Item, 0, len(names))
	for _, path := range names {
		files = append(files, item{title: path})
	}
	return files, nil
}

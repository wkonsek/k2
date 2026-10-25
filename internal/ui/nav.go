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

		if m.stage == "workpods" || (m.stage == "res" && m.kind != "") {
			m.vs.listFrom = m.stage
		} else if m.stage != "projects" {
			m.vs.listFrom = ""
		}
		return m.openList("projects", "namespaces")

	case "pods":
		return m.openList("workpods", "pods")

	case "pf":

		return m.openList("workpods", "pods")

	case "fwds":
		m.status = ""
		return m, m.showForwards()

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
		m.stage = "nav"
		m.status = config.HomePath(m.kubeconfig)
		m.save()
	case "kdir":
		if it.desc == "kust-file" || it.desc == "plain-file" {
			m.status = "opening vim…"
			return m.editKustomization()
		}
		return m, m.showKDir(it.id)
	case "contexts":
		m.context = it.title
		m.stage = "nav"
		m.status = m.context
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
		m.save()
	case "projects":
		name := it.id
		if name == "" {
			name = strings.TrimPrefix(it.title, "● ")
		}
		ctx, ns, err := m.k.UseProject(m.kubeconfig, m.context, name)
		if err != nil {
			m.status = m.errLine(err)
			m.content.Title = m.status
			return m, nil
		}
		m.context = ctx
		m.namespace = ns
		from := m.vs.listFrom
		m.vs.listFrom = ""
		m.save()
		if from == "workpods" || (from == "res" && m.kind != "") {
			kind := m.kind
			if from == "workpods" {
				kind = "pods"
			}
			if ns == "" {
				m.status = "all namespaces"
			} else {
				m.status = ns
			}
			return m.openList(from, kind)
		}
		m.stage = "nav"
		if ns == "" {
			m.status = "all namespaces"
		} else {
			m.status = ns
		}
	case "workpods":

		ns, name := kube.SplitRef(it.id, m.namespace)
		if next, cmd, asked := m.pickContainer("logs", it, ns, name); asked {
			return next, cmd
		}
		return m.openLogs(it.id, "", false)
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

func (m *model) showProjects(projects []string, err error) tea.Cmd {
	if err != nil {
		m.status = m.errLine(err)
	}
	allTitle := "all namespaces"
	if m.namespace == "" {
		allTitle = "● all namespaces"
	}
	items := []list.Item{item{title: allTitle, id: "*"}}
	for _, p := range projects {
		title := p
		if p == m.namespace {
			title = "● " + p
		}
		items = append(items, item{title: title, id: p})
	}
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
	case "projects":
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

func (m *model) reopenList() (tea.Cmd, bool) {
	stage := m.vs.listFrom
	kind := m.kind
	switch stage {
	case "workpods":
		kind = "pods"
	case "res":
		if kind == "" {
			return nil, false
		}
	default:
		return nil, false
	}
	m.vs.listFrom = ""
	next, cmd := m.openList(stage, kind)
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

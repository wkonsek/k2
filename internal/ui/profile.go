package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/layout"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) profileCommand(raw string) (tea.Model, tea.Cmd, bool) {
	word, rest, _ := strings.Cut(raw, " ")
	if !strings.EqualFold(word, "profile") {
		return m, nil, false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 || (len(fields) == 1 && strings.EqualFold(fields[0], "save")) {
		next, cmd := m.askSaveProfile()
		return next, cmd, true
	}
	if strings.EqualFold(fields[0], "save") {
		next, cmd := m.saveProfile(fields[1])
		return next, cmd, true
	}
	next, cmd := m.loadProfile(fields[0])
	return next, cmd, true
}

func (m model) askSaveProfile() (tea.Model, tea.Cmd) {
	d := m.newDialog("profile", "save profile", "", "", "")
	d.withField("", "name", "writes ~/.k2/profiles/<name>.yaml · enter saves")
	m.dlg = d
	return m, nil
}

func (m model) saveProfile(name string) (tea.Model, tea.Cmd) {
	name = strings.TrimSpace(name)
	if !config.ValidProfileName(name) {
		m.status = "profile name must be letters, numbers, . _ -"
		return m, m.notify(toneWarn, m.status)
	}
	prof := m.captureProfile()
	if err := config.SaveProfile(m.cfg.StateDir, name, prof); err != nil {
		m.status = err.Error()
		return m, m.notify(toneErr, m.status)
	}
	m.status = "saved profile " + name
	return m, m.notify(toneOK, m.status)
}

func (m model) captureProfile() config.Profile {
	m.commit()
	root := m.tabs[m.tab].root
	split := "right"
	if root != nil && root.Dir == layout.Col {
		split = "down"
	}
	ids := root.Leaves()
	index := map[string]int{}
	for i, id := range ids {
		index[id] = i
	}
	prof := config.Profile{Split: split, Layout: profileTree(root, index)}
	for _, id := range ids {
		s := m.panes[m.paneIdx(id)]
		if id == m.logID {
			s = m.snap()
		}
		prof.Panes = append(prof.Panes, profilePane(s))
	}
	return prof
}

func profileTree(n *layout.Node, index map[string]int) config.ProfileLayout {
	if n == nil || n.Dir == layout.Leaf {
		i := 0
		if n != nil {
			i = index[n.Pane]
		}
		return config.ProfileLayout{Pane: &i}
	}
	split := "right"
	if n.Dir == layout.Col {
		split = "down"
	}
	lay := config.ProfileLayout{Split: split}
	for _, k := range n.Kids {
		lay.Children = append(lay.Children, profileTree(k, index))
	}
	return lay
}

func profilePane(s paneSnap) config.ProfilePane {
	p := config.ProfilePane{Kubeconfig: s.kubeconfig, Context: s.context, Namespace: s.namespace}
	if p.Namespace == "" {
		p.Namespace = "*"
	}
	switch s.stage {
	case "workpods":
		p.View = "pods"
	case "res":
		p.View = s.kind
	case "pulse":
		p.View = "pulse"
	case "helm", "helmhist", "hdiff":
		p.View = "helm"
	case "logs":
		p.View = "logs"
		p.Target = s.logPod
	case "mlogs":
		p.View = "logs"
		if s.vs.ml != nil {
			p.Target = s.vs.ml.title
			if s.vs.ml.q.Selector != "" {
				p.Selector = s.vs.ml.q.Selector
			} else if s.vs.ml.q.Regex != nil {
				p.Selector = s.vs.ml.q.Regex.String()
			}
		}
	case "fwds":
		p.View = "forwards"
	default:
		p.View = "home"
	}
	return p
}

func (m model) loadProfile(name string) (tea.Model, tea.Cmd) {
	name = strings.TrimSpace(name)
	prof, err := m.cfg.LookupProfile(name)
	if err != nil {
		m.status = err.Error()
		return m, m.notify(toneWarn, m.status)
	}
	if len(prof.Panes) == 0 {
		m.status = "profile " + name + " is empty"
		return m, m.notify(toneWarn, m.status)
	}
	for m.tabs[m.tab].root.Count() > 1 {
		m.closePane()
	}
	if len(prof.Panes) > m.cfg.MaxPanes {
		m.status = fmt.Sprintf("profile %s has %d panes, max is %d", name, len(prof.Panes), m.cfg.MaxPanes)
		return m, m.notify(toneWarn, m.status)
	}
	ids := m.growPanes(len(prof.Panes))
	var root *layout.Node
	if prof.Layout.Empty() {
		root = chainLayout(ids, prof.Split)
	} else {
		root = layoutFromProfile(prof.Layout, ids)
		if root == nil || !root.Valid() || root.Count() != len(ids) {
			m.status = "profile " + name + ": layout does not match its panes"
			return m, m.notify(toneWarn, m.status)
		}
	}
	m.tabs[m.tab].root = root
	m.tabs[m.tab].focus = ids[0]
	m.refit()
	var cmds []tea.Cmd
	cmds = append(cmds, m.relayout())
	for i, pane := range prof.Panes[:len(ids)] {
		m.focusID(ids[i])
		if pane.Kubeconfig != "" {
			m.kubeconfig = pane.Kubeconfig
		}
		if pane.Context != "" {
			m.context = pane.Context
		}
		switch pane.Namespace {
		case "*":
			m.namespace = ""
		case "":
		default:
			m.namespace = pane.Namespace
		}
		m.commit()
		next, cmd := m.openProfilePane(pane)
		m = next.(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.commit()
	}
	m.status = "profile " + name
	return m, tea.Batch(cmds...)
}

func (m *model) growPanes(n int) []string {
	if n < 1 {
		n = 1
	}
	ids := []string{m.logID}
	for len(ids) < n && len(ids) < m.cfg.MaxPanes {
		p := freshPane(m.cfg.Keys)
		p.kubeconfig, p.context, p.namespace = m.kubeconfig, m.context, m.namespace
		m.commit()
		m.panes = append(m.panes, p)
		ids = append(ids, p.logID)
	}
	return ids
}

func chainLayout(ids []string, split string) *layout.Node {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		return layout.New(ids[0])
	}
	dir := layout.Row
	if strings.EqualFold(split, "down") {
		dir = layout.Col
	}
	kids := make([]*layout.Node, len(ids))
	for i, id := range ids {
		kids[i] = layout.New(id)
	}
	return &layout.Node{Dir: dir, Kids: kids}
}

func layoutFromProfile(n config.ProfileLayout, ids []string) *layout.Node {
	if n.Empty() {
		return nil
	}
	if len(n.Children) >= 2 && (n.Split == "right" || n.Split == "down") {
		dir := layout.Row
		if n.Split == "down" {
			dir = layout.Col
		}
		out := &layout.Node{Dir: dir}
		for _, k := range n.Children {
			kid := layoutFromProfile(k, ids)
			if kid == nil {
				return nil
			}
			out.Kids = append(out.Kids, kid)
		}
		return out
	}
	if n.Pane == nil || *n.Pane < 0 || *n.Pane >= len(ids) {
		return nil
	}
	return layout.New(ids[*n.Pane])
}

func (m model) profileNames() []string {
	entries, err := os.ReadDir(filepath.Join(m.cfg.StateDir, "profiles"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if config.ValidProfileName(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (m model) openProfilePane(pane config.ProfilePane) (tea.Model, tea.Cmd) {
	view := strings.ToLower(strings.TrimSpace(pane.View))
	switch view {
	case "", "home", "nav":
		m.stage = "nav"
		return m, nil
	case "pods", "pod", "po":
		return m.openList("workpods", "pods")
	case "pulse":
		return m.openPulse()
	case "helm":
		return m.openHelm()
	case "forwards", "fwd", "fwds":
		return m, m.showForwards()
	case "logs", "mlogs", "stern":
		if pane.Selector != "" {
			q := kube.LogQuery{All: m.cfg.Logs.MultiAllContainers}
			title := pane.Target
			if strings.Contains(pane.Selector, "=") && !strings.Contains(pane.Selector, " ") {
				q.Selector = pane.Selector
				if title == "" {
					title = pane.Selector
				}
			} else if re, err := regexp.Compile(pane.Selector); err == nil {
				q.Regex = re
				if title == "" {
					title = pane.Selector
				}
			}
			if q.Selector != "" || q.Regex != nil {
				return m.openMultiLogs(q, title, "nav")
			}
		}
		if pane.Target != "" && !strings.Contains(pane.Target, " ") {
			return m.openLogs(pane.Target, "", false)
		}
		return m.openList("workpods", "pods")
	default:
		return m.openResource(view)
	}
}

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k2/internal/config"
	"k2/internal/kustomize"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) showKDir(dir string) tea.Cmd {
	if dir == "" || dir == "." {
		if m.kroot != "" {
			dir = m.kroot
		} else if wd, err := os.Getwd(); err == nil {
			dir = wd
		} else {
			dir = "/"
		}
	}
	dir = filepath.Clean(dir)
	entries, err := kustomize.List(dir)
	if err != nil {
		m.status = m.errLine(err)
		return nil
	}

	prev := m.kroot
	m.rememberK()
	want, ok := m.vs.kpos[dir]
	if prev != "" && prev != dir && filepath.Dir(prev) == dir {
		want, ok = kmark{name: filepath.Base(prev), idx: want.idx}, true
	}
	items := make([]list.Item, 0, len(entries))
	for _, e := range entries {
		desc := e.Kind
		switch e.Kind {
		case "parent":
			desc = ""
		case "file":
			desc = "kust-file"
		case "other":
			desc = "plain-file"
		}
		items = append(items, item{title: e.Name, id: e.Path, desc: desc})
	}
	m.kroot = dir
	m.stage = "kdir"
	m.content.ResetFilter()
	m.content.Title = config.HomePath(dir)
	m.content.SetShowTitle(true)
	m.content.SetDelegate(dirDelegate{width: m.tableWidth()})
	m.status = "green dot can be applied"
	m.save()
	cmd := m.content.SetItems(items)
	m.content.Select(0)
	if ok {
		m.content.Select(markIndex(items, want, func(it item) string { return it.title }))
	}
	return cmd
}

func (m *model) rememberK() {
	if m.stage != "kdir" || m.kroot == "" {
		return
	}
	sel := m.content.SelectedItem()
	if sel == nil {
		return
	}
	next := make(map[string]kmark, len(m.vs.kpos)+1)
	for k, v := range m.vs.kpos {
		next[k] = v
	}
	next[m.kroot] = kmark{name: sel.(item).title, idx: m.content.Index()}
	m.vs.kpos = next
}

func markIndex(items []list.Item, mark kmark, key func(item) string) int {
	for i, li := range items {
		if key(li.(item)) == mark.name {
			return i
		}
	}
	return max(0, min(mark.idx, len(items)-1))
}

func (m model) scanKustomize() tea.Cmd {
	root := m.kroot
	return func() tea.Msg {
		return kScanMsg{root: root, dirs: kustomize.Find(root)}
	}
}

func (m *model) showKList(root string, dirs []string) tea.Cmd {
	if len(dirs) == 0 {
		m.status = "no kustomization.yaml under " + config.HomePath(root)
		return nil
	}

	m.rememberK()
	var keep *kmark
	if m.stage == "kust" && m.kroot == root {
		if sel := m.content.SelectedItem(); sel != nil {
			keep = &kmark{name: sel.(item).id, idx: m.content.Index()}
		}
	}
	items := make([]list.Item, len(dirs))
	for i, dir := range dirs {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "" {
			rel = dir
		}
		items[i] = item{title: rel, id: dir, desc: "kust"}
	}
	m.kroot = root
	m.stage = "kust"
	m.content.ResetFilter()
	m.content.Title = config.HomePath(root)
	m.content.SetShowTitle(true)
	m.content.SetDelegate(dirDelegate{width: m.tableWidth()})
	k := m.cfg.Keys
	m.status = k.Apply.Label() + " apply   " + k.KustomizeDelete.Label() + " delete   " + k.Diff.Label() + " diff   " + k.Edit.Label() + " edit"
	m.save()
	cmd := m.content.SetItems(items)
	m.content.Select(0)
	if keep != nil {
		m.content.Select(markIndex(items, *keep, func(it item) string { return it.id }))
	}
	return cmd
}

func (m *model) notKust() (tea.Model, tea.Cmd) {
	m.status = "not a kustomization"
	return *m, m.notify(toneWarn, "not a kustomization")
}

func (m model) selectedKust() (string, bool) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return "", false
	}
	it := sel.(item)
	if it.desc != "kust" && it.desc != "kust-file" {
		return "", false
	}
	if it.id == "" || it.id == ".." {
		return "", false
	}
	return it.id, true
}

func (m model) kustomizeCmd(verb, dir string) tea.Cmd {
	kc, ctx, pane, token, r := m.kubeconfig, m.context, m.logID, m.load.token, m.k.Run
	target := m.kustTarget(dir)
	finish := func(text string, code int) kOutMsg {
		return kOutMsg{id: pane, text: text, code: code, target: target, ctx: ctx}
	}
	if verb == "diff" {
		return func() tea.Msg {
			text, code := kustomize.Run(r, verb, dir, kc, ctx)
			return finish(text, code)
		}
	}
	return func() tea.Msg {
		ch := make(chan tea.Msg, 16)
		go func() {
			total, _ := kustomize.Build(r, dir)
			ch <- progressMsg{id: pane, token: token, total: total, ch: ch}
			done, deleted := 0, 0
			text, code := kustomize.RunStream(r, verb, dir, kc, ctx, func(line string) {
				if !kustomize.Settles(line) {
					return
				}
				done++
				if kustomize.Classify(line) == kustomize.Deleted {
					deleted++
				}
				ch <- progressMsg{id: pane, token: token, done: done, total: total, ch: ch}

				if verb == "delete" && total > 0 && done == total && deleted > 0 {
					ch <- progressMsg{id: pane, token: token, what: fmt.Sprintf("waiting for %d to terminate", deleted), ch: ch}
				}
			})
			ch <- finish(text, code)
		}()
		return <-ch
	}
}

func listen(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m model) kustTarget(dir string) string {
	if m.kroot != "" {
		if rel, err := filepath.Rel(m.kroot, dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return config.HomePath(dir)
}

func (m model) editKustomization() (tea.Model, tea.Cmd) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return m, nil
	}
	it := sel.(item)
	var path string
	switch it.desc {
	case "kust-file", "plain-file":
		path = filepath.Join(it.id, it.title)
	case "kust":
		path = kustomize.FileIn(it.id)
	default:
		m.status = "not a file"
		return m, m.notify(toneWarn, "not a file: enter opens a directory")
	}
	if path == "" {
		return m, func() tea.Msg { return kOutMsg{text: "no kustomization file", code: 1} }
	}
	return m.runEditor("kust", "vim "+filepath.Base(path), filepath.Base(path), "",
		func() *exec.Cmd { return vimCommand(path) },
		func(error) tea.Msg { return lessDoneMsg{} })
}

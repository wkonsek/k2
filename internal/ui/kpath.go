package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

const maxCands = 8

func (m model) openKPath() (tea.Model, tea.Cmd) {
	start := config.HomePath(m.kroot)
	if !strings.HasSuffix(start, "/") {
		start += "/"
	}
	d := m.newDialog("kpath", "go to directory", "", "", "")
	d.where = "kustomize · " + config.HomePath(m.kroot)
	d.withField(start, "~/repo/overlays", "tab completes · ~/ / .. work")
	m.dlg = d
	return m, nil
}

func (m model) kpathKey(msg tea.KeyMsg) (model, bool) {
	d := m.dlg
	if !m.cfg.Keys.Complete.Has(msg.String()) {
		d.cands, d.candBase, d.candAt = nil, "", -1
		return m, false
	}
	d.msg = ""
	if len(d.cands) > 1 && d.candBase != "" {

		d.candAt = (d.candAt + 1) % len(d.cands)
		d.input.SetValue(d.candBase + d.cands[d.candAt] + "/")
		d.input.CursorEnd()
		return m, true
	}
	value, cands := completePath(d.input.Value(), m.kroot)
	switch {
	case value == "" && len(cands) == 0:
		d.msg = "no matching directory"
	case len(cands) > 1:
		d.cands, d.candAt = cands, -1
		d.candBase = splitBase(value)
		d.input.SetValue(value)
	default:
		d.cands = nil
		d.input.SetValue(value)
	}
	d.input.CursorEnd()
	return m, true
}

func (m model) submitKPath() (tea.Model, tea.Cmd) {
	d := m.dlg
	target := expandPath(d.input.Value(), m.kroot)
	st, err := os.Stat(target)
	if err != nil || !st.IsDir() {
		text := "not a directory: " + config.HomePath(target)
		d.msg = text
		return m, m.notify(toneErr, text)
	}
	m.dlg = nil
	return m, m.showKDir(target)
}

func expandPath(typed, base string) string {
	p := strings.TrimSpace(typed)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if p == "" {
		p = base
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

func splitBase(typed string) string {
	if i := strings.LastIndex(typed, "/"); i >= 0 {
		return typed[:i+1]
	}
	return ""
}

func completePath(typed, base string) (value string, cands []string) {
	dirPart := splitBase(typed)
	prefix := typed[len(dirPart):]
	dir := expandPath(dirPart, base)
	if dirPart == "" {
		dir = filepath.Clean(base)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		if !e.IsDir() {

			st, err := os.Stat(filepath.Join(dir, name))
			if err != nil || !st.IsDir() {
				continue
			}
		}
		cands = append(cands, name)
	}
	switch len(cands) {
	case 0:
		return "", nil
	case 1:
		return dirPart + cands[0] + "/", nil
	}
	sort.Strings(cands)
	return dirPart + commonPrefix(cands), cands
}

func commonPrefix(names []string) string {
	p := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func (d *dialog) candLines() []string {
	if len(d.cands) == 0 {
		return nil
	}
	var parts []string
	for i, c := range d.cands {
		if i == maxCands {
			parts = append(parts, dimStyle.Render("…"))
			break
		}
		if i == d.candAt {
			parts = append(parts, selectedItemStyle.Render(c+"/"))
		} else {
			parts = append(parts, itemStyle.Render(c+"/"))
		}
	}
	return []string{strings.Join(parts, "  ")}
}

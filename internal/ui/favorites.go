package ui

import (
	"fmt"
	"strings"

	"k2/internal/config"
	"k2/internal/state"

	tea "github.com/charmbracelet/bubbletea"
)

type favEntry struct {
	fav        config.Favorite
	fromConfig bool
	state      int
}

func (m model) favEntries() []favEntry {
	var out []favEntry
	for _, f := range m.cfg.Favorites {
		out = append(out, favEntry{fav: f, fromConfig: true, state: -1})
	}
	for i, f := range m.favs {
		out = append(out, favEntry{fav: config.Favorite{Kubeconfig: f.Kubeconfig, Context: f.Context, Namespace: f.Namespace, View: f.View}, state: i})
	}
	return out
}

func favLabel(i int, f config.Favorite) string {
	label := f.Name
	if label == "" {
		label = f.Context + " · " + or(f.Namespace, "default namespace")
	}
	num := "  "
	if i < 9 {
		num = fmt.Sprintf("%d ", i+1)
	}
	return num + label
}

func favNote(e favEntry) string {
	var parts []string
	if e.fav.View != "" {
		parts = append(parts, ":"+e.fav.View)
	}
	if e.fav.Kubeconfig != "" {
		parts = append(parts, config.HomePath(e.fav.Kubeconfig))
	}
	if e.fromConfig {
		parts = append(parts, "config")
	}
	return strings.Join(parts, "  ")
}

func (m model) openFavorites() (tea.Model, tea.Cmd) {
	d := m.newDialog("favorites", "★ favorites", "", m.namespace, "")
	m.dlg = d
	m.fillFavorites(0)
	return m, nil
}

func (m *model) fillFavorites(idx int) {
	if m.dlg == nil {
		return
	}
	d := *m.dlg
	d.options, d.notes = nil, nil
	for i, e := range m.favEntries() {
		d.options = append(d.options, favLabel(i, e.fav))
		d.notes = append(d.notes, favNote(e))
	}
	k := m.cfg.Keys
	d.hint = ""
	if len(d.options) == 0 {
		d.msg = "none yet: " + k.FavAdd.Label() + " pins this pane's context, namespace and view"
	} else {
		d.msg = ""
	}
	d.details = []string{"enter or 1-9 jump · " + k.FavAdd.Label() + " add this pane · " + k.FavRemove.Label() + " remove · " +
		k.FavUp.Label() + "/" + k.FavDown.Label() + " move"}
	d.idx = max(0, min(idx, len(d.options)-1))
	m.dlg = &d
}

func (m model) favoritesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := m.dlg
	entries := m.favEntries()
	switch {
	case km.Cancel.Has(key):
		m.dlg = nil
		m.status = "cancelled"
		return m, nil
	case km.Submit.Has(key):
		if d.idx < len(entries) {
			return m.jumpFavorite(entries[d.idx].fav)
		}
		return m, nil
	case len(key) == 1 && key >= "1" && key <= "9":
		if i := int(key[0] - '1'); i < len(entries) {
			return m.jumpFavorite(entries[i].fav)
		}
		return m, nil
	case km.ScrollDown.Has(key) && len(entries) > 0:
		m.dlg.idx = (d.idx + 1) % len(entries)
		return m, nil
	case km.ScrollUp.Has(key) && len(entries) > 0:
		m.dlg.idx = (d.idx - 1 + len(entries)) % len(entries)
		return m, nil
	case km.FavAdd.Has(key):
		f := m.currentFavorite()
		if f.Context == "" {
			m.dlg.msg = "nothing to pin: select a kubeconfig and context first"
			return m, nil
		}
		for i, e := range entries {
			if e.fav.Context == f.Context && e.fav.Namespace == f.Namespace && e.fav.Kubeconfig == f.Kubeconfig && e.fav.View == f.View {
				m.fillFavorites(i)
				m.dlg.msg = "already pinned"
				return m, nil
			}
		}
		m.favs = append(append([]state.Favorite(nil), m.favs...), f)
		m.save()
		m.fillFavorites(len(entries))
		return m, m.notify(toneOK, "pinned "+f.Context+" · "+or(f.Namespace, "default"))
	case km.FavRemove.Has(key):
		if d.idx >= len(entries) {
			return m, nil
		}
		e := entries[d.idx]
		if e.fromConfig {
			m.dlg.msg = "a config.yaml favorite: remove it in the file"
			return m, nil
		}
		next := append([]state.Favorite(nil), m.favs[:e.state]...)
		m.favs = append(next, m.favs[e.state+1:]...)
		m.save()
		m.fillFavorites(d.idx)
		return m, m.notify(toneOK, "unpinned")
	case km.FavUp.Has(key), km.FavDown.Has(key):
		if d.idx >= len(entries) {
			return m, nil
		}
		e := entries[d.idx]
		step := 1
		if km.FavUp.Has(key) {
			step = -1
		}
		to := e.state + step
		if e.fromConfig || to < 0 || to >= len(m.favs) {
			if e.fromConfig {
				m.dlg.msg = "config.yaml favorites keep their order from the file"
			}
			return m, nil
		}
		favs := append([]state.Favorite(nil), m.favs...)
		favs[e.state], favs[to] = favs[to], favs[e.state]
		m.favs = favs
		m.save()
		m.fillFavorites(d.idx + step)
		return m, nil
	}
	return m, nil
}

func (m model) currentFavorite() state.Favorite {
	return state.Favorite{Kubeconfig: m.kubeconfig, Context: m.context, Namespace: m.namespace, View: m.currentView()}
}

func (m model) currentView() string {
	switch m.stage {
	case "workpods":
		if m.vs.podSel == "" {
			return "pods"
		}
	case "res", "yaml", "describe":
		if m.stage == "res" || m.dback == "res" {
			return m.kind
		}
	case "helm":
		if m.vs.helmAll {
			return "helm all"
		}
		return "helm"
	case "pulse":
		return "pulse"
	case "xray":
		if x := m.vs.xray; x != nil {
			return strings.TrimSpace("xray " + x.kind + " " + x.name)
		}
	case "mlogs":
		if v := m.vs.ml; v != nil {
			return "logs " + v.q.String()
		}
	case "fwds":
		return "forwards"
	}
	return ""
}

func (m model) jumpFavorite(f config.Favorite) (tea.Model, tea.Cmd) {
	m.dlg = nil
	if f.Kubeconfig != "" {
		m.kubeconfig = f.Kubeconfig
	}
	if m.kubeconfig == "" {
		m.status = "select a kubeconfig first (the favorite names none)"
		return m, m.notify(toneWarn, m.status)
	}
	m.context = f.Context
	m.namespace = f.Namespace
	if m.namespace == "" {
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
	}
	m.stage = "nav"
	m.vs = viewState{wrapFlip: m.vs.wrapFlip}
	m.load = load{}
	m.status = m.context + " · " + or(m.namespace, "all namespaces")
	m.save()
	note := m.notify(toneOK, "★ "+m.status)
	if f.View == "" {
		return m, note
	}
	next, cmd := m.runCommand(f.View)
	return next, tea.Batch(note, cmd)
}

func (m model) favByName(name string) (tea.Model, tea.Cmd) {
	entries := m.favEntries()
	want := strings.ToLower(strings.TrimSpace(name))
	if n := len(want); n == 1 && want[0] >= '1' && want[0] <= '9' {
		if i := int(want[0] - '1'); i < len(entries) {
			return m.jumpFavorite(entries[i].fav)
		}
	}
	var hits []config.Favorite
	for _, e := range entries {
		if strings.ToLower(favName(e.fav)) == want {
			return m.jumpFavorite(e.fav)
		}
		if strings.Contains(strings.ToLower(favName(e.fav)), want) {
			hits = append(hits, e.fav)
		}
	}
	if len(hits) == 1 {
		return m.jumpFavorite(hits[0])
	}
	if len(hits) == 0 {
		m.status = "no favorite named " + name
	} else {
		m.status = fmt.Sprintf("%d favorites match %s: tab completes", len(hits), name)
	}
	return m, m.notify(toneWarn, m.status)
}

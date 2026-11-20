package ui

import (
	"fmt"
	"strings"

	"k2/internal/config"
	"k2/internal/state"

	tea "github.com/charmbracelet/bubbletea"
)

type cmdFavEntry struct {
	fav        config.CommandFavorite
	fromConfig bool
	state      int
}

func (m model) cmdFavEntries() []cmdFavEntry {
	var out []cmdFavEntry
	for _, f := range m.cfg.CommandFavs {
		out = append(out, cmdFavEntry{fav: f, fromConfig: true, state: -1})
	}
	for i, f := range m.cmdFavs {
		out = append(out, cmdFavEntry{fav: config.CommandFavorite{Name: f.Name, Command: f.Command}, state: i})
	}
	return out
}

func cmdFavLabel(i int, f config.CommandFavorite) string {
	num := "  "
	if i < 9 {
		num = fmt.Sprintf("%d ", i+1)
	}
	return num + or(f.Name, f.Command)
}

func cmdFavNote(e cmdFavEntry) string {
	var parts []string
	if e.fav.Name != "" {
		parts = append(parts, ":"+e.fav.Command)
	}
	if e.fromConfig {
		parts = append(parts, "config")
	}
	return strings.Join(parts, "  ")
}

func cleanCommand(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ":"))
}

func (m model) openCmdFavorites() (tea.Model, tea.Cmd) {
	d := m.newDialog("cmdfavs", "★ commands", "", "", "")
	d.where = "every context · run as if typed after :"
	m.dlg = d
	m.fillCmdFavs(0)
	return m, nil
}

func (m *model) fillCmdFavs(idx int) {
	if m.dlg == nil {
		return
	}
	d := *m.dlg
	d.options, d.notes = nil, nil
	for i, e := range m.cmdFavEntries() {
		d.options = append(d.options, cmdFavLabel(i, e.fav))
		d.notes = append(d.notes, cmdFavNote(e))
	}
	k := m.cfg.Keys
	d.hasIn, d.kind, d.hint = false, "", ""
	if len(d.options) == 0 {
		d.msg = "none yet: " + k.CmdSave.Label() + " in the : line saves what you typed, " + k.FavAdd.Label() + " here adds the last command"
	} else {
		d.msg = ""
	}
	d.details = []string{"enter/1-9 run · " + k.FavAdd.Label() + " add last · " + k.CmdEdit.Label() + " edit · " + k.CmdRename.Label() + " rename · " +
		k.FavRemove.Label() + " remove · " + k.FavUp.Label() + "/" + k.FavDown.Label() + " move"}
	d.idx = max(0, min(idx, len(d.options)-1))
	m.dlg = &d
}

func (m model) addCmdFavorite(text string) (model, bool) {
	text = cleanCommand(text)
	if text == "" {
		return m, false
	}
	for _, e := range m.cmdFavEntries() {
		if strings.EqualFold(e.fav.Command, text) {
			return m, false
		}
	}
	m.cmdFavs = append(append([]state.CommandFavorite(nil), m.cmdFavs...), state.CommandFavorite{Command: text})
	m.save()
	return m, true
}

func (m model) saveTypedCommand() (tea.Model, tea.Cmd) {
	return m.saveCommandText(m.cmd.Value())
}

func (m model) saveCommandText(text string) (tea.Model, tea.Cmd) {
	text = cleanCommand(text)
	if text == "" {
		m.status = "nothing to save: type a command first"
		return m, m.notify(toneWarn, m.status)
	}
	next, added := m.addCmdFavorite(text)
	if !added {
		next.status = "already a favorite: " + text
		return next, next.notify(toneWarn, next.status)
	}
	next.status = "saved favorite: " + text
	return next, next.notify(toneOK, "saved favorite: "+fitWidth(text, 70))
}

func (m model) runCmdFavorite(e cmdFavEntry) (tea.Model, tea.Cmd) {
	m.dlg = nil
	m.status = ":" + e.fav.Command
	return m.runCommand(e.fav.Command)
}

func (m model) cmdFavByName(name string) (tea.Model, tea.Cmd) {
	entries := m.cmdFavEntries()
	want := strings.ToLower(strings.TrimSpace(name))
	if len(want) == 1 && want[0] >= '1' && want[0] <= '9' {
		if i := int(want[0] - '1'); i < len(entries) {
			return m.runCmdFavorite(entries[i])
		}
	}
	var hits []cmdFavEntry
	for _, e := range entries {
		label := strings.ToLower(or(e.fav.Name, e.fav.Command))
		if label == want {
			return m.runCmdFavorite(e)
		}
		if strings.Contains(label, want) {
			hits = append(hits, e)
		}
	}
	if len(hits) == 1 {
		return m.runCmdFavorite(hits[0])
	}
	if len(hits) == 0 {
		m.status = "no favorite command named " + name
	} else {
		m.status = fmt.Sprintf("%d favorite commands match %s: tab completes", len(hits), name)
	}
	return m, m.notify(toneWarn, m.status)
}

func (m model) cmdFavsCommand(rest string) (tea.Model, tea.Cmd) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return m.openCmdFavorites()
	}
	word, arg, _ := strings.Cut(rest, " ")
	if strings.EqualFold(word, "add") {
		return m.saveCommandText(arg)
	}
	return m.cmdFavByName(rest)
}

func (m model) cmdFavoritesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := m.dlg
	entries := m.cmdFavEntries()
	if d.hasIn {
		return m.cmdFavEditKey(msg, entries)
	}
	switch {
	case km.Cancel.Has(key):
		m.dlg = nil
		m.status = "cancelled"
		return m, nil
	case km.Submit.Has(key):
		if d.idx < len(entries) {
			return m.runCmdFavorite(entries[d.idx])
		}
		return m, nil
	case len(key) == 1 && key >= "1" && key <= "9":
		if i := int(key[0] - '1'); i < len(entries) {
			return m.runCmdFavorite(entries[i])
		}
		return m, nil
	case km.ScrollDown.Has(key) && len(entries) > 0:
		m.dlg.idx = (d.idx + 1) % len(entries)
		return m, nil
	case km.ScrollUp.Has(key) && len(entries) > 0:
		m.dlg.idx = (d.idx - 1 + len(entries)) % len(entries)
		return m, nil
	case km.FavAdd.Has(key):
		if m.lastCmd == "" {
			m.dlg.msg = "nothing to add yet: run a command with : first"
			return m, nil
		}
		next, added := m.addCmdFavorite(m.lastCmd)
		if !added {
			for i, e := range entries {
				if strings.EqualFold(e.fav.Command, m.lastCmd) {
					next.fillCmdFavs(i)
				}
			}
			next.dlg.msg = "already saved"
			return next, nil
		}
		next.fillCmdFavs(len(entries))
		return next, next.notify(toneOK, "saved favorite: "+fitWidth(m.lastCmd, 70))
	case km.FavRemove.Has(key):
		if d.idx >= len(entries) {
			return m, nil
		}
		e := entries[d.idx]
		if e.fromConfig {
			m.dlg.msg = "a config.yaml favorite: remove it in the file"
			return m, nil
		}
		next := append([]state.CommandFavorite(nil), m.cmdFavs[:e.state]...)
		m.cmdFavs = append(next, m.cmdFavs[e.state+1:]...)
		m.save()
		m.fillCmdFavs(d.idx)
		return m, m.notify(toneOK, "removed")
	case km.CmdEdit.Has(key), km.CmdRename.Has(key):
		if d.idx >= len(entries) {
			return m, nil
		}
		e := entries[d.idx]
		if e.fromConfig {
			m.dlg.msg = "a config.yaml favorite: edit it in the file"
			return m, nil
		}
		nd := *d
		if km.CmdEdit.Has(key) {
			nd.withField(e.fav.Command, "command", "edit the command · enter saves · esc keeps it")
			nd.kind = "edit"
		} else {
			nd.withField(e.fav.Name, "label", "label shown in the list, empty clears it · enter saves · esc keeps it")
			nd.kind = "rename"
		}
		nd.msg = ""
		m.dlg = &nd
		return m, nil
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
		if e.fromConfig || to < 0 || to >= len(m.cmdFavs) {
			if e.fromConfig {
				m.dlg.msg = "config.yaml favorites keep their order from the file"
			}
			return m, nil
		}
		favs := append([]state.CommandFavorite(nil), m.cmdFavs...)
		favs[e.state], favs[to] = favs[to], favs[e.state]
		m.cmdFavs = favs
		m.save()
		m.fillCmdFavs(d.idx + step)
		return m, nil
	}
	return m, nil
}

func (m model) cmdFavEditKey(msg tea.KeyMsg, entries []cmdFavEntry) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := m.dlg
	switch {
	case km.Cancel.Has(key):
		m.fillCmdFavs(d.idx)
		return m, nil
	case km.Submit.Has(key):
		if d.idx >= len(entries) {
			return m, nil
		}
		e := entries[d.idx]
		val := strings.TrimSpace(d.input.Value())
		favs := append([]state.CommandFavorite(nil), m.cmdFavs...)
		note := "renamed"
		if d.kind == "edit" {
			val = cleanCommand(val)
			if val == "" {
				d.msg = "the command is empty"
				return m, nil
			}
			favs[e.state].Command = val
			note = "updated"
		} else {
			favs[e.state].Name = val
		}
		m.cmdFavs = favs
		m.save()
		m.fillCmdFavs(d.idx)
		return m, m.notify(toneOK, note)
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	d.msg = ""
	return m, cmd
}

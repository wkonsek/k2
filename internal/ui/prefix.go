package ui

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

const prefixWait = 1500 * time.Millisecond

const repeatWait = time.Second

type repeatTimeoutMsg struct{ tok int }

func (m model) startRepeat() (model, tea.Cmd) {
	m.repeatOn = true
	m.repeatTok++
	tok := m.repeatTok
	return m, tea.Tick(repeatWait, func(time.Time) tea.Msg { return repeatTimeoutMsg{tok: tok} })
}

func (m model) endRepeat(msg repeatTimeoutMsg) model {
	if m.repeatOn && msg.tok == m.repeatTok {
		m.repeatOn = false
	}
	return m
}

func (m model) repeatKey(key string) (tea.Model, tea.Cmd, bool) {
	name := m.cfg.Keys.PrefixedAction(key)
	if !config.MoveKeys[name] {
		return m, nil, false
	}
	next, cmd, ok := m.paneAction(name)
	if !ok {
		next = m
	}
	n, rcmd := next.(model).startRepeat()
	return n, tea.Batch(cmd, rcmd), true
}

type prefixTimeoutMsg struct{ tok int }

func (m model) prefixTakes(key string) bool {
	if !m.cfg.Keys.Prefix.Has(key) {
		return false
	}
	if m.ed.live() {
		return true
	}
	return !(m.inputFocused() && m.inputEditKey(key))
}

func (m model) startPrefix() (tea.Model, tea.Cmd) {
	m.prefixOn = true
	m.prefixTok++
	m.status = "prefix " + m.cfg.Keys.Prefix.Label() + " …"
	tok := m.prefixTok
	return m, tea.Tick(prefixWait, func(time.Time) tea.Msg { return prefixTimeoutMsg{tok: tok} })
}

func (m model) endPrefix(msg prefixTimeoutMsg) model {
	if m.prefixOn && msg.tok == m.prefixTok {
		m.prefixOn = false
		if strings.HasPrefix(m.status, "prefix ") {
			m.status = ""
		}
	}
	return m
}

func (m model) afterPrefix(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	km := m.cfg.Keys
	key := msg.String()
	m.prefixOn = false
	if strings.HasPrefix(m.status, "prefix ") {
		m.status = ""
	}
	switch {
	case key == "esc":
		return m, nil, true
	case km.Prefix.Has(key):
		if m.ed.live() {
			m.ed.t.Key(msg)
		}
		return m, nil, true
	}
	if m.ed.live() && m.ed.kind == "shell" && km.CopyMode.Has(key) {
		next, cmd := m.enterShellCopy()
		return next, cmd, true
	}
	if name := km.PrefixedAction(key); name != "" {
		next, cmd, ok := m.paneAction(name)
		if !ok {
			return m, nil, true
		}
		if config.MoveKeys[name] {

			n, rcmd := next.(model).startRepeat()
			return n, tea.Batch(cmd, rcmd), true
		}
		return next.(model), cmd, true
	}
	if m.globalKey(key) {
		return m, nil, false
	}
	m.status = fmt.Sprintf("%s %s: not a pane or tab key (%s)", km.Prefix.Label(), key, m.prefixHelp())
	return m, nil, true
}

var arrowGlyph = map[string]string{"left": "←", "right": "→", "up": "↑", "down": "↓"}

func (m model) moveArrows() string {
	var b strings.Builder
	for _, name := range []string{"pane_left", "pane_down", "pane_up", "pane_right"} {
		k := m.cfg.Keys.PrefixedFor(name)
		if g, ok := arrowGlyph[k]; ok {
			k = g
		}
		b.WriteString(k)
	}
	return b.String()
}

func (m model) repeatLegend() string {
	return "  " + keyStyle.Render("repeat") + dimStyle.Render("   ") + pair(m.moveArrows(), "move pane") +
		dimStyle.Render("   ") + dimStyle.Render("any other key ends it")
}

func (m model) prefixHelp() string {
	k := m.cfg.Keys
	all := func(name string) string { return k.PrefixedAll(name) }
	var parts []string
	add := func(keys, what string) {
		if keys != "" && keys != "/" {
			parts = append(parts, keys+" "+what)
		}
	}
	add(all("split_down"), "split down")
	add(all("split_right"), "split right")
	add(all("next_pane"), "next pane")
	add(all("prev_pane"), "previous pane")
	add(all("last_pane"), "last pane")
	add(all("close_pane"), "close pane")
	add(all("new_tab"), "new tab")
	add(all("close_tab"), "close tab")
	add(all("rename_tab"), "rename tab")
	add(k.PrefixedFor("next_tab")+"/"+k.PrefixedFor("prev_tab"), "tabs")
	add(k.PrefixedFor("tab_1")+"-"+k.PrefixedFor("tab_9"), "tab N")
	add(m.moveArrows(), "move (repeats)")
	add(joinKeys(all("zoom"), k.Fullscreen.Label()), "full")
	parts = append(parts, "esc cancel")
	return strings.Join(parts, " · ")
}

func (m model) prefixLegend() string {
	k := m.cfg.Keys

	all := func(name string) string { return k.PrefixedAll(name) }
	parts := []string{keyStyle.Render("prefix " + k.Prefix.Short())}
	add := func(keys, what string) {
		if keys != "" && keys != "/" && keys != "-" {
			parts = append(parts, pair(keys, what))
		}
	}
	add(all("split_down"), "split ↓")
	add(all("split_right"), "split →")
	add(all("next_pane"), "next pane")
	add(all("prev_pane"), "prev pane")
	add(all("last_pane"), "last")
	add(all("close_pane"), "close")
	add(all("new_tab"), "tab")
	add(all("close_tab"), "close tab")
	add(all("rename_tab"), "rename")
	add(k.PrefixedFor("next_tab")+"/"+k.PrefixedFor("prev_tab"), "tabs")
	add(k.PrefixedFor("tab_1")+"-"+k.PrefixedFor("tab_9"), "tab N")
	add(m.moveArrows(), "move")
	add(joinKeys(all("zoom"), k.Fullscreen.Short()), "full")
	if m.ed.live() {
		parts = append(parts, pair(k.Prefix.Short(), "literal"))
	}
	parts = append(parts, pair("esc", "cancel"))
	return "  " + strings.Join(parts, dimStyle.Render("   "))
}

func (m model) paneAction(name string) (tea.Model, tea.Cmd, bool) {
	free := !m.cmdOn
	many := len(m.tabIDs()) > 1
	switch name {
	case "split_down", "split_right":
		if free {
			next, cmd := m.split(name == "split_right")
			return next, cmd, true
		}
	case "close_pane":
		if free {
			if len(m.tabIDs()) <= 1 {
				m.status = "last pane"
				return m, nil, true
			}
			return m.askClosePane(), nil, true
		}
	case "new_tab":
		if free {
			next, cmd := m.newTab()
			return next, cmd, true
		}
	case "close_tab":
		if free {
			if len(m.tabs) <= 1 {
				m.status = "last tab"
				return m, nil, true
			}
			return m.askCloseTab(), nil, true
		}
	case "next_tab", "prev_tab":
		if free && len(m.tabs) > 1 {
			if name == "next_tab" {
				m.stepTab(1)
			} else {
				m.stepTab(-1)
			}
			return m, m.relayout(), true
		}
	case "pane_left", "pane_right", "pane_up", "pane_down":

		if many {
			m.moveFocus(moveDir(name))
			return m, nil, true
		}
	case "prev_pane", "next_pane":
		if many {
			if name == "next_pane" {
				m.cycleFocus(1)
			} else {
				m.cycleFocus(-1)
			}
			return m, nil, true
		}
	case "rename_tab":
		if free {
			return m.openRenameTab(), nil, true
		}
	case "zoom":

		if free {
			m.zoom = !m.zoom
			return m, m.relayout(), true
		}
	case "last_pane":
		if many {
			m.lastPane()
			return m, nil, true
		}
	default:
		var n int
		if _, err := fmt.Sscanf(name, "tab_%d", &n); err == nil && n >= 1 && n <= 9 && free {
			m.switchTab(n - 1)
			return m, m.relayout(), true
		}
	}
	return m, nil, false
}

func short(b interface{ Short() string }) string { return b.Short() }

func joinKeys(keys ...string) string {
	var out []string
	for _, k := range keys {
		if k != "" {
			out = append(out, k)
		}
	}
	return strings.Join(out, "/")
}

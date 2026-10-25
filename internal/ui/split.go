package ui

import (
	"fmt"

	"k2/internal/config"
	"k2/internal/layout"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type tab struct {
	root  *layout.Node
	focus string

	last string

	name string
}

const (
	minPaneW = 20
	minPaneH = 5
)

func (m model) paneIdx(id string) int {
	for i := range m.panes {
		if i == m.focus && m.logID == id {
			return i
		}
		if i != m.focus && m.panes[i].logID == id {
			return i
		}
	}
	return -1
}

func (m model) tabOf(id string) int {
	for i, t := range m.tabs {
		if t.root.Has(id) {
			return i
		}
	}
	return m.tab
}

func (m model) tabIDs() []string {
	if m.tab < 0 || m.tab >= len(m.tabs) {
		return []string{m.logID}
	}
	return m.tabs[m.tab].root.Leaves()
}

func (m model) tabPos(id string) int {
	if len(m.tabs) == 0 {
		return 0
	}
	for i, x := range m.tabs[m.tabOf(id)].root.Leaves() {
		if x == id {
			return i
		}
	}
	return 0
}

func (m model) paneArea() layout.Rect {
	_, mainW, panelH := m.layout()
	return layout.Rect{W: max(mainW, 8), H: panelH}
}

func (m model) rects(t int) map[string]layout.Rect {
	if t < 0 || t >= len(m.tabs) {
		return map[string]layout.Rect{m.logID: m.paneArea()}
	}
	return m.tabs[t].root.Rects(m.paneArea())
}

func (m model) paneBox(id string) (w, h int) {
	a := m.paneArea()
	if m.zoom && id == m.logID {
		return a.W, a.H
	}
	r, ok := m.rects(m.tabOf(id))[id]
	if !ok {
		return a.W, a.H
	}
	return r.W, r.H
}

func (m model) split(right bool) (tea.Model, tea.Cmd) {
	t := &m.tabs[m.tab]
	if t.root.Count() >= m.cfg.MaxPanes {
		m.status = fmt.Sprintf("%d panes max", m.cfg.MaxPanes)
		return m, m.notify(toneWarn, m.status)
	}
	p := freshPane(m.cfg.Keys)
	grown := t.root.Clone()
	if right {
		grown = grown.SplitRight(m.logID, p.logID)
	} else {
		grown = grown.SplitDown(m.logID, p.logID)
	}
	for _, r := range grown.Rects(m.paneArea()) {
		if r.W < minPaneW || r.H < minPaneH {
			m.status = "no room for another pane here"
			return m, m.notify(toneWarn, m.status)
		}
	}
	m.commit()
	t.last = m.logID
	p.kubeconfig, p.context, p.namespace = m.kubeconfig, m.context, m.namespace
	m.panes = append(m.panes, p)
	t.root = grown
	m.focus = len(m.panes) - 1
	m.restore(m.panes[m.focus])
	t.focus = m.logID
	m.status = fmt.Sprintf("pane %d/%d", m.tabPos(m.logID)+1, t.root.Count())
	return m, m.relayout()
}

func (m *model) closePane() {
	t := &m.tabs[m.tab]
	ids := t.root.Leaves()
	if len(ids) <= 1 {
		m.status = "last pane"
		return
	}
	pos := m.tabPos(m.logID)
	if m.ed.live() {
		m.ed.t.Kill()
	}
	m.commit()
	t.root = t.root.Remove(m.logID)
	m.panes = append(m.panes[:m.focus], m.panes[m.focus+1:]...)
	m.focus = -1
	ids = t.root.Leaves()
	next := ids[min(pos, len(ids)-1)]
	m.focus = m.paneIdx(next)
	m.restore(m.panes[m.focus])
	t.focus = next
	m.refit()
	m.status = fmt.Sprintf("pane %d/%d", m.tabPos(next)+1, len(ids))
}

func (m *model) focusID(id string) {
	i := m.paneIdx(id)
	if i < 0 || id == m.logID {
		return
	}
	from := m.logID
	m.focusPane(i)
	m.tabs[m.tab].focus = id
	m.tabs[m.tab].last = from
	m.refit()
}

func (m *model) lastPane() {
	t := m.tabs[m.tab]
	if t.last == "" || t.last == m.logID || !t.root.Has(t.last) {
		m.status = "no previous pane"
		return
	}
	m.focusID(t.last)
	m.status = fmt.Sprintf("pane %d/%d", m.tabPos(m.logID)+1, t.root.Count())
}

func (m *model) moveFocus(mv layout.Move) {
	ids := m.tabIDs()
	if next := layout.Neighbor(m.rects(m.tab), ids, m.logID, mv); next != "" {
		m.focusID(next)
	}
}

func (m model) paneMoveKey(key string) bool {
	return config.MoveKeys[m.cfg.Keys.DirectAction(key)]
}

func moveDir(name string) layout.Move {
	switch name {
	case "pane_left":
		return layout.Left
	case "pane_right":
		return layout.Right
	case "pane_up":
		return layout.Up
	}
	return layout.Down
}

func (m *model) cycleFocus(step int) {
	ids := m.tabIDs()
	if len(ids) < 2 {
		return
	}
	pos := m.tabPos(m.logID)
	m.focusID(ids[(pos+step+len(ids))%len(ids)])
}

func (m *model) refit() {
	_, mainW, panelH := m.layout()
	m.fitContent(mainW, panelH)
	m.resizeEditors()
}

func (m *model) relayout() tea.Cmd {
	m.refit()
	return tea.Batch(m.refreshVisible()...)
}

func (m model) inputEditKey(key string) bool {
	if !m.inputFocused() {
		return false
	}
	km := textinput.DefaultKeyMap
	for _, b := range []interface{ Keys() []string }{
		km.CharacterForward, km.CharacterBackward, km.WordForward, km.WordBackward,
		km.DeleteWordBackward, km.DeleteWordForward, km.DeleteAfterCursor, km.DeleteBeforeCursor,
		km.DeleteCharacterBackward, km.DeleteCharacterForward, km.LineStart, km.LineEnd, km.Paste,
	} {
		for _, k := range b.Keys() {
			if k == key {
				return true
			}
		}
	}
	return false
}

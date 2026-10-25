package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"k2/internal/paneterm"
)

type editor struct {
	t       *paneterm.Term
	kind    string
	label   string
	name    string
	cleanup string

	done   bool
	code   int
	screen string
}

func (e *editor) live() bool { return e != nil && !e.done }

func (m model) editorSize(id string) (w, h int) {
	bw, bh := m.paneBox(id)
	return max(bw-4, 2), max(bh-2, 1)
}

func (m model) runEditor(kind, label, name, cleanup string, mk func() *exec.Cmd, done func(error) tea.Msg) (tea.Model, tea.Cmd) {
	if m.ed.live() {
		m.status = "an editor already runs in this pane"
		return m, nil
	}
	if !m.cfg.UI.EditorInline {
		return m, tea.ExecProcess(mk(), done)
	}
	w, h := m.editorSize(m.logID)
	t, err := paneterm.Start(m.logID, mk(), w, h)
	if err != nil {
		m.status = "inline editor unavailable (" + err.Error() + "); using the full terminal"
		return m, tea.ExecProcess(mk(), done)
	}
	m.ed = &editor{t: t, kind: kind, label: label, name: name, cleanup: cleanup}
	m.prefixOn = false
	m.status = m.ed.tool() + " · " + m.cfg.Keys.Prefix.Label() + " then a k2 key"
	return m, t.Next()
}

var vimCommand = func(path string) *exec.Cmd {
	editor := "vim"
	if _, err := exec.LookPath(editor); err != nil {
		editor = "vi"
	}
	return exec.Command(editor, path)
}

func (m model) editorOutput(msg paneterm.OutputMsg) tea.Cmd {
	if m.ownsTerm(msg.T) {
		return msg.T.Next()
	}
	return nil
}

func (m model) ownsTerm(t *paneterm.Term) bool {
	if m.ed != nil && m.ed.t == t {
		return true
	}
	for i, p := range m.panes {
		if i != m.focus && p.ed != nil && p.ed.t == t {
			return true
		}
	}
	return false
}

func (m model) editorExited(msg paneterm.ExitMsg) (tea.Model, tea.Cmd) {
	if msg.T == nil {
		return m, nil
	}
	focused := msg.T.ID == m.logID
	var cmd tea.Cmd
	m.withPane(msg.T.ID, func() { cmd = m.finishEditor(msg, focused) })
	return m, cmd
}

func (m *model) finishEditor(msg paneterm.ExitMsg, focused bool) tea.Cmd {
	ed := m.ed
	if ed == nil || ed.t != msg.T || ed.done {
		return nil
	}
	if ed.cleanup != "" {
		_ = os.Remove(ed.cleanup)
	}
	m.prefixOn = false
	failed := msg.Code != 0 || msg.Err != nil
	if (ed.kind == "edit" || ed.kind == "shell") && failed {
		ed.done, ed.code, ed.screen = true, msg.Code, msg.T.Plain()
		what := map[string]string{"edit": "kubectl edit", "shell": "shell"}[ed.kind]
		m.status = fmt.Sprintf("%s failed (exit %d) · %s closes", what, msg.Code, m.cfg.Keys.Back.Label())
		if msg.Err != nil {
			m.status = what + ": " + msg.Err.Error()
		}
		if focused {
			return m.notify(toneErr, map[string]string{"edit": "edit ", "shell": "shell "}[ed.kind]+ed.name+" failed")
		}
		return nil
	}
	m.ed = nil
	if msg.Err != nil {
		m.status = "editor: " + msg.Err.Error()
		return nil
	}
	m.status = ""
	switch ed.kind {
	case "edit":
		what := "edited " + ed.name
		if focused {
			return func() tea.Msg { return editDoneMsg{what: what} }
		}
		m.status = what
	case "vim":
		if focused {
			return func() tea.Msg { return vimDoneMsg{} }
		}
	case "shell":
		m.status = "shell closed"
	}
	return nil
}

func (m model) editorKey(msg tea.KeyMsg) (model, bool) {
	km := m.cfg.Keys
	key := msg.String()
	if m.ed.done {
		switch {
		case km.Back.Has(key):
			m.ed = nil
			m.status = ""
			return m, true
		case m.globalKey(key):
			return m, false
		}
		return m, true
	}

	if m.paneMoveKey(key) {
		return m, false
	}
	m.ed.t.Key(msg)
	return m, true
}

func (m model) globalKey(key string) bool {
	k := m.cfg.Keys
	return k.Quit.Has(key) || k.Fullscreen.Has(key) || k.LastError.Has(key) || m.paneMoveKey(key) || k.DirectAction(key) != ""
}

func (m *model) resizeEditors() {
	if m.ed.live() {
		m.ed.t.Resize(m.editorSize(m.logID))
	}
	for i, p := range m.panes {
		if i != m.focus && p.ed.live() {
			p.ed.t.Resize(m.editorSize(p.logID))
		}
	}
}

func (m model) editorsIn(t int) []string {
	var ids []string
	for _, id := range m.tabs[t].root.Leaves() {
		if e := m.paneEditor(id); e.live() {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m model) paneEditor(id string) *editor {
	if id == m.logID {
		return m.ed
	}
	for i, p := range m.panes {
		if i != m.focus && p.logID == id {
			return p.ed
		}
	}
	return nil
}

func (m *model) killEditors() {
	if m.ed.live() {
		m.ed.t.Kill()
	}
	for i, p := range m.panes {
		if i != m.focus && p.ed.live() {
			p.ed.t.Kill()
		}
	}
}

func (e *editor) body(focused bool, w, h int, back string) string {
	if !e.done {
		return e.t.View(focused)
	}
	lines := strings.Split(e.screen, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > h-1 {
		lines = lines[len(lines)-(h-1):]
	}
	for i, l := range lines {
		lines[i] = clipLine(l, w)
	}
	foot := errStyle.Render(clipLine(fmt.Sprintf("── exit %d · %s closes ──", e.code, back), w))
	return strings.Join(lines, "\n") + "\n" + foot
}

func (e *editor) title(prefix string) string {
	if e.done {
		return fmt.Sprintf("%s · exit %d", e.label, e.code)
	}
	return e.label + " · " + e.tool() + " · " + prefix + " k2"
}

func (e *editor) tool() string {
	if e != nil && e.kind == "shell" {
		return "shell"
	}
	return "vim"
}

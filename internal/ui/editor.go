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

	scroll int
	mark   bool
	drag   bool
	ax, ay int
	bx, by int
	done   bool
	code   int
	screen string
}

func (e *editor) live() bool { return e != nil && !e.done }

func (m model) editorSize(id string) (w, h int) {
	if m.zoom && id == m.logID && m.ed.live() && m.ed.kind == "shell" {
		h = m.height - m.tabBarRows() - 1
		if h < 1 {
			h = 1
		}
		w = max(m.width, 2)
		return w, h
	}
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
	if kind == "shell" {
		m.status = "shell · pgup scrolls · " + m.cfg.Keys.Prefix.Label() + " y copies"
	}
	return m, tea.Batch(t.Next(), m.mouseForShells())
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
	return m, tea.Batch(cmd, m.mouseForShells())
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

	if m.paneMoveKey(key) || km.Fullscreen.Has(key) {
		return m, false
	}
	if m.shellScrollKey(key) {
		return m, true
	}
	if m.ed.kind == "shell" && m.ed.scroll > 0 && !m.ed.t.AltScreen() && key == "esc" {
		m.noteShellScroll(0)
		return m, true
	}
	if m.ed.scroll > 0 {
		m.noteShellScroll(0)
	}
	m.ed.mark = false
	m.ed.t.Key(msg)
	return m, true
}

func (m model) shellScrollKey(key string) bool {
	if m.ed == nil || m.ed.kind != "shell" || m.ed.t == nil || m.ed.t.AltScreen() {
		return false
	}
	_, rows := m.editorSize(m.logID)
	page := max(rows-1, 1)
	km := m.cfg.Keys
	switch {
	case key == "pgup" || key == "shift+pgup":
		m.noteShellScroll(m.ed.scroll + page)
		return true
	case key == "pgdown" || key == "shift+pgdown":
		m.noteShellScroll(m.ed.scroll - page)
		return true
	case m.ed.scroll > 0 && km.ScrollUp.Has(key):
		m.noteShellScroll(m.ed.scroll + 1)
		return true
	case m.ed.scroll > 0 && km.ScrollDown.Has(key):
		m.noteShellScroll(m.ed.scroll - 1)
		return true
	case m.ed.scroll > 0 && km.Top.Has(key):
		m.noteShellScroll(m.ed.t.ScrollbackLen())
		return true
	case m.ed.scroll > 0 && km.Bottom.Has(key):
		m.noteShellScroll(0)
		return true
	}
	return false
}

func (m model) noteShellScroll(n int) {
	if m.ed == nil || m.ed.t == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	if maxN := m.ed.t.ScrollbackLen(); n > maxN {
		n = maxN
	}
	m.ed.scroll = n
	m.ed.mark = false
	if n == 0 {
		m.status = "shell · pgup scrolls · " + m.cfg.Keys.Prefix.Label() + " y copies"
		return
	}
	m.status = "scrollback · " + m.cfg.Keys.ScrollUp.Short() + "/" + m.cfg.Keys.ScrollDown.Short() + " line · pgup/pgdown page · esc live"
}

func (m model) shellLive() bool {
	if m.ed.live() && m.ed.kind == "shell" {
		return true
	}
	for i, p := range m.panes {
		if i != m.focus && p.ed.live() && p.ed.kind == "shell" {
			return true
		}
	}
	return false
}

func (m model) mouseForShells() tea.Cmd {
	if m.shellLive() {
		return tea.EnableMouseCellMotion
	}
	return tea.DisableMouse
}

func (m model) shellWheel(msg tea.MouseMsg) (model, tea.Cmd) {
	if m.ed == nil || !m.ed.live() || m.ed.kind != "shell" || m.ed.t.AltScreen() {
		return m, nil
	}
	if m.ed.drag {
		return m.shellDrag(msg)
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.noteShellScroll(m.ed.scroll + 3)
	case tea.MouseButtonWheelDown:
		m.noteShellScroll(m.ed.scroll - 3)
	case tea.MouseButtonLeft:
		return m.shellDrag(msg)
	default:
		return m, nil
	}
	return m, nil
}

func (m model) shellDrag(msg tea.MouseMsg) (model, tea.Cmd) {
	x, y, ok := m.shellCell(msg.X, msg.Y)
	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if !ok {
			return m, nil
		}
		m.ed.drag = true
		m.ed.mark = true
		m.ed.ax, m.ed.ay, m.ed.bx, m.ed.by = x, y, x, y
		return m, nil
	case m.ed.drag && msg.Action == tea.MouseActionMotion:
		if ok {
			m.ed.bx, m.ed.by = x, y
		}
		return m, nil
	case m.ed.drag && msg.Action == tea.MouseActionRelease:
		m.ed.drag = false
		if ok {
			m.ed.bx, m.ed.by = x, y
		}
		if m.ed.ax == m.ed.bx && m.ed.ay == m.ed.by {
			m.ed.mark = false
			return m, nil
		}
		text := strings.TrimSpace(m.ed.t.Selected(m.ed.scroll, m.ed.ax, m.ed.ay, m.ed.bx, m.ed.by))
		if text == "" {
			m.ed.mark = false
			return m, nil
		}
		return m, m.copyCmdAs(text, "selection")
	default:
		return m, nil
	}
}

func (m model) shellCell(px, py int) (x, y int, ok bool) {
	ox, oy, w, h := m.shellOrigin()
	x, y = px-ox, py-oy
	if x < 0 || y < 0 || x >= w || y >= h {
		return 0, 0, false
	}
	return x, y, true
}

func (m model) shellOrigin() (x, y, w, h int) {
	w, h = m.editorSize(m.logID)
	if m.zoom {
		return 0, m.tabBarRows(), w, h
	}
	sideW, _, _ := m.layout()
	r, ok := m.rects(m.tab)[m.logID]
	if !ok {
		return sideW + 2, m.tabBarRows(), w, h
	}
	return sideW + 2 + r.X + 2, m.tabBarRows() + r.Y + 1, w, h
}

func (m model) enterShellCopy() (model, tea.Cmd) {
	if m.ed == nil || m.ed.t == nil {
		return m, nil
	}
	text := strings.TrimRight(m.ed.t.Transcript(), "\n")
	if strings.TrimSpace(text) == "" {
		m.status = "nothing to copy"
		return m, nil
	}
	m.shellYank = strings.Split(text, "\n")
	m.logStick = true
	m.ed.mark = false
	next, cmd := m.enterCopy()
	n := next.(model)
	return n, cmd
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
		cursor := focused && e.scroll == 0 && !e.mark
		if e.mark {
			return e.t.ViewSelected(cursor, e.scroll, e.ax, e.ay, e.bx, e.by)
		}
		return e.t.ViewOffset(cursor, e.scroll)
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

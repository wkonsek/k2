package ui

import (
	"errors"
	"fmt"
	"strings"

	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type dialog struct {
	act     string
	title   string
	where   string
	options []string
	notes   []string
	idx     int
	input   textinput.Model
	hasIn   bool
	hint    string
	ask     string
	pending tea.Cmd
	pendMsg string
	msg     string
	ns      string
	name    string
	kind    string
	details []string
	danger  bool
	onYes   func(m model) (tea.Model, tea.Cmd)
	seq     int
	images  []imageField

	cands    []string
	candBase string
	candAt   int
}

func (m model) newDialog(act, title, kind, ns, name string) *dialog {
	where := m.context
	if ns != "" {
		where += " · " + ns
	}
	return &dialog{act: act, title: title, where: where, kind: kind, ns: ns, name: name}
}

func (d *dialog) withField(value, placeholder, hint string) *dialog {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = keyStyle
	ti.TextStyle = itemStyle
	ti.Placeholder = placeholder
	ti.CharLimit = 512
	ti.SetValue(value)
	ti.CursorEnd()
	ti.Focus()
	d.input, d.hasIn, d.hint = ti, true, hint
	return d
}

func (d *dialog) picked() string {
	if d.idx < 0 || d.idx >= len(d.options) {
		return ""
	}
	return d.options[d.idx]
}

func (m model) confirmDialog(title, question, kind, ns, name, running string, cmd tea.Cmd) model {
	d := m.newDialog("confirm", title, kind, ns, name)
	d.ask, d.pending, d.pendMsg = question, cmd, running
	m.dlg = d
	return m
}

func (m model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := *m.dlg
	m.dlg = &d
	if !printable(msg) && km.Quit.Has(key) {
		m.killEditors()
		m.quitting = true
		return m, tea.Quit
	}
	if d.act == "error" {
		return m.errorKey(key)
	}
	if d.act == "favorites" {
		return m.favoritesKey(msg)
	}
	if d.ask != "" {
		switch {
		case km.Yes.Has(key):
			cmd := d.pending
			m.dlg = nil
			if d.onYes != nil {
				return d.onYes(m)
			}
			m.status = d.pendMsg
			return m, m.runAction(d.pendMsg, cmd)
		case km.No.Has(key):
			if d.act == "confirm" {
				m.dlg = nil
				m.status = "cancelled"
				return m, nil
			}
			d.ask, d.pending, d.details = "", nil, nil
		}
		return m, nil
	}
	switch {
	case km.Cancel.Has(key):
		m.dlg = nil
		m.status = "cancelled"
		return m, nil
	case km.Submit.Has(key):
		return m.submitDialog()
	case d.act == "setimage":
		return m.setImageKey(msg)
	case d.act == "kpath":
		if next, done := m.kpathKey(msg); done {
			return next, nil
		}
	case len(d.options) > 0 && (km.ScrollDown.Has(key) || km.Complete.Has(key)):
		d.idx = (d.idx + 1) % len(d.options)
		return m, nil
	case len(d.options) > 0 && (km.ScrollUp.Has(key) || key == "shift+tab"):
		d.idx = (d.idx - 1 + len(d.options)) % len(d.options)
		return m, nil
	}
	if !d.hasIn {
		return m, nil
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	d.msg = ""
	return m, cmd
}

func (m model) submitDialog() (tea.Model, tea.Cmd) {
	d := m.dlg
	switch d.act {
	case "kpath":
		return m.submitKPath()
	case "rename_tab":
		return m.submitRenameTab()
	case "logfilter":
		return m.submitLogFilter()
	case "xrayfilter":
		return m.submitXrayFilter()
	case "search":
		return m.submitSearch()
	case "setimage":
		return m.submitSetImage()
	case "logs", "prevlogs":
		m.dlg = nil
		return m.openLogs(d.ns+"/"+d.name, d.picked(), d.act == "prevlogs")
	case "theme":
		return m.applyThemePick(d.picked())
	case "shell":
		m.dlg = nil
		return m.openShell(d.ns, d.name, d.picked())
	case "scale":
		n, err := parseReplicas(d.input.Value())
		if err != nil {
			d.msg = err.Error()
			return m, nil
		}
		what := fmt.Sprintf("scaled %s/%s to %d", d.kind, d.name, n)
		cmd := m.kubectlAction(what, "scale", d.kind+"/"+d.name, "-n", d.ns, fmt.Sprintf("--replicas=%d", n))
		return m.runOrAsk(fmt.Sprintf("scale %s/%s to %d on %s?", d.kind, d.name, n, m.context), "scaling…", cmd)
	case "label", "annotate":
		pairs, err := parsePairs(d.input.Value())
		if err != nil {
			d.msg = err.Error()
			return m, nil
		}
		verb := map[string]string{"label": "labeled", "annotate": "annotated"}[d.act]
		args := append([]string{d.act, d.kind + "/" + d.name, "-n", d.ns, "--overwrite"}, pairs...)
		cmd := m.kubectlAction(verb+" "+d.kind+"/"+d.name, args...)
		return m.runOrAsk(fmt.Sprintf("%s %s/%s with %s on %s?", d.act, d.kind, d.name, strings.Join(pairs, " "), m.context),
			d.act+"…", cmd)
	}
	return m, nil
}

func (m model) runOrAsk(question, running string, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.dangerous() {
		m.dlg.ask, m.dlg.pending, m.dlg.pendMsg = question, cmd, running
		return m, nil
	}
	m.dlg = nil
	m.status = running
	return m, m.runAction(running, cmd)
}

func (m *model) runAction(running string, cmd tea.Cmd) tea.Cmd {
	spin := m.startLoad(running, false)
	pane, token := m.logID, m.load.token
	stamped := func() tea.Msg {
		if cmd == nil {
			return editDoneMsg{what: running, pane: pane, token: token}
		}
		msg := cmd()
		if done, ok := msg.(editDoneMsg); ok {
			done.pane, done.token = pane, token
			return done
		}
		return msg
	}
	return tea.Batch(spin, stamped)
}

func parseReplicas(v string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err != nil || n < 0 || fmt.Sprint(n) != strings.TrimSpace(v) {
		return 0, fmt.Errorf("type a number of replicas (0 or more)")
	}
	return n, nil
}

func parsePairs(v string) ([]string, error) {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return nil, fmt.Errorf("type key=value, or key- to remove")
	}
	for _, f := range fields {
		key, _, set := strings.Cut(f, "=")
		if set && key != "" {
			continue
		}
		if !set && len(f) > 1 && strings.HasSuffix(f, "-") {
			continue
		}
		return nil, fmt.Errorf("%q: use key=value, or key- to remove", f)
	}
	return fields, nil
}

func (m model) kubectlAction(what string, args ...string) tea.Cmd {
	run, kc, ctx := m.k.Run, m.kubeconfig, m.context
	full := append(append([]string(nil), args...), "--kubeconfig", kc, "--context", ctx)
	return func() tea.Msg {
		out, err := shell.Combined(run, "kubectl", full...)
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return editDoneMsg{what: what, err: errors.New(text)}
		}
		return editDoneMsg{what: what}
	}
}

func (m model) dialogBox(width int) string {
	d := m.dlg
	inner := width - 4
	hot := m.dangerous()
	title := d.title
	if hot {
		title = skull + " " + title
	}
	lines := []string{dimStyle.Render(fitWidth(d.where, inner)), ""}

	var det []string
	for _, l := range d.details {
		det = append(det, wrapLine(l, inner)...)
	}
	if room := max(3, m.height-10); d.act == "error" && len(det) > room {
		more := len(det) - room + 1
		det = append(det[:room-1], dimStyle.Render(moreRows(more)))
	}
	for _, l := range det {
		st := itemStyle
		if d.act == "error" {
			st = errStyle
		}
		lines = append(lines, st.Render(l))
	}
	if len(d.details) > 0 {
		lines = append(lines, "")
	}
	start, end := m.optionWindow()
	if start > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		o := d.options[i]
		text := o
		if i < len(d.notes) && d.notes[i] != "" {
			text += "  " + d.notes[i]
		}
		text = fitWidth(text, inner-2)
		if i == d.idx {
			lines = append(lines, selectedItemStyle.Render("▸ "+text))
		} else {
			lines = append(lines, itemStyle.Render("  "+text))
		}
	}
	if end < len(d.options) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(d.options)-end)))
	}
	if len(d.images) > 0 && d.ask == "" {
		lines = append(lines, d.imageRows(inner)...)
	}
	if d.hasIn {
		if d.hint != "" {
			lines = append(lines, dimStyle.Render(d.hint))
		}
		lines = append(lines, d.input.View())
		lines = append(lines, d.candLines()...)
	}
	if len(d.options) > 0 || d.hasIn || (len(d.images) > 0 && d.ask == "") {
		lines = append(lines, "")
	}
	k := m.cfg.Keys
	switch {
	case d.ask != "":
		st := warnStyle
		if hot || d.danger {
			st = errStyle
		}
		lines = append(lines, st.Render(d.ask+"  "+m.yn()))
		lines = append(lines, dimStyle.Render("default no · enter does nothing"))
	case d.act == "error":
		lines = append(lines, keyStyle.Render(k.CopyYank.Short())+dimStyle.Render(" copy  ")+keyStyle.Render(k.Cancel.Short())+dimStyle.Render(" close"))
	case d.msg != "" && d.act == "setimage" && len(d.images) == 0:
		lines = append(lines, dimStyle.Render(d.msg))
	case d.msg != "":
		lines = append(lines, errStyle.Render(d.msg))
	case len(d.images) > 0:
		lines = append(lines, keyStyle.Render("tab/↑↓")+dimStyle.Render(" move  ")+keyStyle.Render(k.Submit.Short())+dimStyle.Render(" apply  ")+
			keyStyle.Render(k.Cancel.Short())+dimStyle.Render(" cancel"))
	case len(d.options) > 0:
		lines = append(lines, keyStyle.Render(k.ScrollDown.Short()+"/"+k.ScrollUp.Short())+dimStyle.Render(" move  ")+
			keyStyle.Render(k.Submit.Short())+dimStyle.Render(" pick  ")+keyStyle.Render(k.Cancel.Short())+dimStyle.Render(" cancel"))
	default:
		lines = append(lines, keyStyle.Render(k.Submit.Short())+dimStyle.Render(" ok  ")+
			keyStyle.Render(k.Cancel.Short())+dimStyle.Render(" cancel"))
	}
	body := make([]string, 0, len(lines))
	for _, l := range lines {
		body = append(body, strings.Split(ansi.Hardwrap(l, inner, true), "\n")...)
	}
	return drawBox(strings.Join(body, "\n"), width, len(body)+2, true, title, "", hot || d.danger)
}

func (m model) optionWindow() (start, end int) {
	d := m.dlg
	if d == nil {
		return 0, 0
	}
	n := len(d.options)
	if n == 0 {
		return 0, 0
	}
	_, y, _ := m.popupRect()
	chrome := 6
	if len(d.details) > 0 {
		chrome += len(d.details) + 1
	}
	limit := m.height - chrome
	if y > 0 && m.height-y-chrome >= min(n, 6) {
		limit = m.height - y - chrome
	}
	if limit > n {
		limit = n
	}
	if limit < n {
		limit -= 2
	}
	if limit < 1 {
		limit = 1
	}
	if limit > n {
		limit = n
	}
	if d.idx >= limit {
		start = d.idx - limit + 1
	}
	if start+limit > n {
		start = n - limit
	}
	if start < 0 {
		start = 0
	}
	return start, start + limit
}

func (m model) withDialog(screen string) string {
	if m.dlg == nil {
		return screen
	}
	x, y, w := m.popupRect()
	box := m.dialogBox(w)
	h := strings.Count(box, "\n") + 1
	if y+h > m.height {
		y = max(0, m.height-h)
	} else if h < 12 {
		y += (12 - h) / 2
	}
	return overlay(screen, lipgloss.NewStyle().Render(box), x, y)
}

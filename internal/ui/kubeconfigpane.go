package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	kcPaneMinW   = 76
	kcListMinW   = 26
	kcListMaxW   = 40
	kcMaxChecks  = 4
	kcLabelWidth = 9
)

var kcDebounce = 250 * time.Millisecond

var kcSlots = make(chan struct{}, kcMaxChecks)

type kcInfoEntry struct {
	stamp string
	info  kube.KubeconfigInfo
	err   error
}

type kcLiveEntry struct {
	stamp   string
	live    kube.Live
	done    bool
	running bool
}

type kcPane struct {
	sel  string
	seq  int
	info map[string]kcInfoEntry
	live map[string]kcLiveEntry
}

type kcInfoMsg struct {
	path  string
	stamp string
	info  kube.KubeconfigInfo
	err   error
}

type kcTickMsg struct {
	seq  int
	path string
}

type kcLiveMsg struct {
	path, ctx, stamp string
	live             kube.Live
}

func kcLiveKey(path, ctx string) string { return path + "\x00" + ctx }

func (m *model) kcState() *kcPane {
	if m.kcp == nil {
		m.kcp = &kcPane{info: map[string]kcInfoEntry{}, live: map[string]kcLiveEntry{}}
	}
	return m.kcp
}

func (m model) kcSelected() string {
	if m.stage != "files" {
		return ""
	}
	if sel := m.content.SelectedItem(); sel != nil {
		if it, ok := sel.(item); ok {
			return it.title
		}
	}
	return ""
}

func (m *model) kcSync() tea.Cmd {
	if m.stage != "files" || !m.cfg.UI.KubeconfigPane {
		if m.kcp != nil {
			m.kcp.sel = ""
		}
		return nil
	}
	path := m.kcSelected()
	if path == "" {
		return nil
	}
	p := m.kcState()
	if p.sel == path {
		return nil
	}
	p.sel = path
	p.seq++
	return kcInspectCmd(path)
}

func kcInspectCmd(path string) tea.Cmd {
	return func() tea.Msg {
		stamp := fileStamp([]string{path})
		info, err := kube.InspectKubeconfig(path)
		return kcInfoMsg{path: path, stamp: stamp, info: info, err: err}
	}
}

func (m *model) applyKcInfo(msg kcInfoMsg) tea.Cmd {
	p := m.kcState()
	p.info[msg.path] = kcInfoEntry{stamp: msg.stamp, info: msg.info, err: msg.err}
	if msg.path != p.sel || m.stage != "files" || !m.cfg.UI.KubeconfigLive {
		return nil
	}
	seq := p.seq
	return tea.Tick(kcDebounce, func(time.Time) tea.Msg { return kcTickMsg{seq: seq, path: msg.path} })
}

func (m *model) applyKcTick(msg kcTickMsg) tea.Cmd {
	p := m.kcState()
	if m.stage != "files" || msg.seq != p.seq || msg.path != p.sel {
		return nil
	}
	return m.kcLiveCmds(msg.path)
}

func (m *model) kcLiveCmds(path string) tea.Cmd {
	p := m.kcState()
	entry, ok := p.info[path]
	if !ok || entry.err != nil {
		return nil
	}
	ttl := time.Duration(m.cfg.UI.KubeconfigTTL) * time.Second
	var cmds []tea.Cmd
	for _, c := range entry.info.Contexts {
		key := kcLiveKey(path, c.Name)
		e := p.live[key]
		if e.running {
			continue
		}
		if e.done && e.stamp == entry.stamp && now().Sub(e.live.At) < ttl {
			continue
		}
		p.live[key] = kcLiveEntry{stamp: entry.stamp, running: true, live: e.live, done: e.done}
		cmds = append(cmds, kcLiveCmd(m.k.Run, path, c.Name, entry.stamp))
	}
	return tea.Batch(cmds...)
}

func kcLiveCmd(run shell.Runner, path, ctx, stamp string) tea.Cmd {
	return func() tea.Msg {
		kcSlots <- struct{}{}
		defer func() { <-kcSlots }()
		return kcLiveMsg{path: path, ctx: ctx, stamp: stamp, live: kube.CheckContext(run, path, ctx)}
	}
}

func (m *model) applyKcLive(msg kcLiveMsg) {
	p := m.kcState()
	p.live[kcLiveKey(msg.path, msg.ctx)] = kcLiveEntry{stamp: msg.stamp, live: msg.live, done: true}
}

func (m *model) kcRecheck(path string) tea.Cmd {
	if path == "" || m.stage != "files" || !m.cfg.UI.KubeconfigPane {
		return nil
	}
	p := m.kcState()
	for key := range p.live {
		if strings.HasPrefix(key, path+"\x00") {
			delete(p.live, key)
		}
	}
	p.sel = path
	p.seq++
	return kcInspectCmd(path)
}

func (m model) kcRefreshKey() (tea.Model, tea.Cmd) {
	path := m.kcSelected()
	if path == "" {
		return m, nil
	}
	cmd := m.kcRecheck(path)
	m.status = "rechecking " + config.HomePath(path)
	return m, cmd
}

func (m model) kcLoginContext(path string) string {
	if m.kcp != nil {
		if e, ok := m.kcp.info[path]; ok && e.err == nil {
			return e.info.Current
		}
	}
	info, _ := kube.InspectKubeconfig(path)
	return info.Current
}

func (m model) kubeconfigBody(w int) string {
	if !m.cfg.UI.KubeconfigPane || w < kcPaneMinW {
		return m.content.View()
	}
	listW := min(kcListMaxW, max(kcListMinW, w/3))
	cardW := w - listW - 1
	h := m.content.Height()
	if m.boxH > 0 {
		h = m.boxH
	}
	lst := m.content
	lst.SetSize(listW, h)
	lst.SetDelegate(kcDelegate{inner: contentDelegate(listW), width: listW})
	left := lipgloss.NewStyle().Width(listW).MaxWidth(listW).Render(lst.View())
	card := m.kcCard(m.kcSelected(), cardW, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", card)
}

func (m model) kcCard(path string, width, height int) string {
	inner := max(10, width-4)
	rows := max(1, height-2)
	lines := m.kcCardLines(path, inner, rows)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for i := range lines {
		lines[i] = clipLine(lines[i], inner)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Muted).
		Padding(0, 1).
		Width(width - 2).
		Render(strings.Join(lines, "\n"))
}

func (m model) kcCardLines(path string, inner, rows int) []string {
	if path == "" {
		return []string{dimStyle.Render("no kubeconfig selected")}
	}
	p := m.kcp
	var entry kcInfoEntry
	have := false
	if p != nil {
		entry, have = p.info[path]
	}
	head := titleStyle.Render(clipLine(config.HomePath(path), inner))
	if !have {
		return []string{head, dimStyle.Render(kcSpin(m) + " reading…")}
	}
	if entry.err != nil {
		return []string{head, errStyle.Render(clipLine("cannot read: "+kcShortErr(entry.err), inner))}
	}
	ctxs := kcOrder(entry.info.Contexts)
	if len(ctxs) == 0 {
		return []string{head, dimStyle.Render("no contexts")}
	}
	sub := fmt.Sprintf("%d context", len(ctxs))
	if len(ctxs) != 1 {
		sub += "s"
	}
	if cur := entry.info.Current; cur != "" {
		sub += " · current " + cur
	}
	lines := []string{head, dimStyle.Render(clipLine(sub, inner)), ""}
	footer := m.kcFooter(path, entry, inner)
	avail := rows - len(lines) - 2
	shown := 0
	for i, c := range ctxs {
		block := m.kcContextLines(path, entry, c, inner)
		need := len(block)
		if i > 0 {
			need++
		}
		left := len(ctxs) - i - 1
		reserve := 0
		if left > 0 {
			reserve = 1
		}
		if shown > 0 && need+reserve > avail {
			break
		}
		if i > 0 {
			lines = append(lines, "")
			avail--
		}
		lines = append(lines, block...)
		avail -= len(block)
		shown++
	}
	if more := len(ctxs) - shown; more > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d more", more)))
	}
	if len(lines) > rows-2 {
		lines = lines[:rows-2]
	}
	return append(lines, "", footer)
}

func kcOrder(in []kube.KubeconfigContext) []kube.KubeconfigContext {
	out := make([]kube.KubeconfigContext, 0, len(in))
	for _, c := range in {
		if c.Current {
			out = append(out, c)
		}
	}
	for _, c := range in {
		if !c.Current {
			out = append(out, c)
		}
	}
	return out
}

func (m model) kcFooter(path string, entry kcInfoEntry, inner int) string {
	k := m.cfg.Keys
	var parts []string
	if s := k.Login.Short(); s != "" {
		parts = append(parts, keyStyle.Render(s)+" "+dimStyle.Render("login"))
	}
	if m.cfg.UI.KubeconfigLive {
		if s := k.KubeconfigRefresh.Short(); s != "" {
			parts = append(parts, keyStyle.Render(s)+" "+dimStyle.Render("recheck"))
		}
		var at time.Time
		for _, c := range entry.info.Contexts {
			if e, ok := m.kcp.live[kcLiveKey(path, c.Name)]; ok && e.done && e.live.At.After(at) {
				at = e.live.At
			}
		}
		if !at.IsZero() {
			parts = append(parts, dimStyle.Render("checked "+tokenLeft(max(0, now().Sub(at)))+" ago"))
		}
	}
	return clipLine(strings.Join(parts, dimStyle.Render("  ·  ")), inner)
}

func kcSpin(m model) string {
	if m.cfg.UI.Animations {
		return kindSpinGlyph(m.beat)
	}
	return "…"
}

func kcShortErr(err error) string {
	s := strings.TrimSpace(err.Error())
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func kcRow(label, value string, inner int) string {
	return clipLine(dimStyle.Render(padRight(label, kcLabelWidth))+value, inner)
}

func (m model) kcContextLines(path string, entry kcInfoEntry, c kube.KubeconfigContext, inner int) []string {
	name := titleStyle.Render(c.Name)
	if c.Current {
		name = keyStyle.Render("▸ ") + name + " " + okStyle.Render("current")
	} else {
		name = dimStyle.Render("  ") + name
	}
	valueW := max(4, inner-kcLabelWidth)
	server := dimStyle.Render("—")
	if c.Server != "" {
		server = itemStyle.Render(kcTruncMid(c.Server, valueW))
	}
	ns := c.Namespace
	if ns == "" {
		ns = "default"
	}
	who := or(c.User, "—")
	meta := itemStyle.Render(or(c.Cluster, "—")) + dimStyle.Render(" · ns ") + itemStyle.Render(ns) + dimStyle.Render(" · user ") + itemStyle.Render(who)
	lines := []string{
		clipLine(name, inner),
		kcRow("server", server, inner),
		kcRow("cluster", meta, inner),
		kcRow("auth", kcAuthText(c.Auth), inner),
		kcRow("valid", kcValidText(c.Auth), inner),
	}
	var live *kcLiveEntry
	if m.kcp != nil {
		if e, ok := m.kcp.live[kcLiveKey(path, c.Name)]; ok && e.stamp == entry.stamp {
			live = &e
		}
	}
	lines = append(lines, kcRow("api", m.kcReachText(live), inner), kcRow("access", m.kcAccessText(live), inner))
	return lines
}

func kcTruncMid(s string, w int) string {
	if ansi.StringWidth(s) <= w || w < 8 {
		return clipLine(s, w)
	}
	head := (w - 1) * 2 / 3
	tail := w - 1 - head
	r := []rune(s)
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

func kcAuthText(a kube.AuthInfo) string {
	var kind string
	switch a.Kind {
	case kube.AuthToken:
		kind = "bearer token"
		if a.OpenShift {
			kind = "bearer token (OpenShift oc login)"
		}
	case kube.AuthCert:
		kind = "client cert"
	case kube.AuthExec:
		kind = "exec plugin"
		if a.Command != "" && a.Command != "." {
			kind += " · " + a.Command
		}
	case kube.AuthProvider:
		kind = "auth-provider"
		if a.Provider != "" {
			kind += " · " + a.Provider
		}
	case kube.AuthBasic:
		kind = "basic auth"
	default:
		kind = "none"
	}
	out := itemStyle.Render(kind)
	switch {
	case a.Kind == kube.AuthNone && a.Note != "":
		out += dimStyle.Render(" · ") + errStyle.Render(a.Note)
	case a.Kind == kube.AuthNone:
	case a.Present:
		what := map[kube.AuthKind]string{kube.AuthToken: "token", kube.AuthCert: "cert", kube.AuthBasic: "password", kube.AuthProvider: "token", kube.AuthExec: "command"}[a.Kind]
		out += dimStyle.Render(" · ") + okStyle.Render(what+" present")
	default:
		what := map[kube.AuthKind]string{kube.AuthToken: "token", kube.AuthCert: "cert", kube.AuthBasic: "password", kube.AuthProvider: "token", kube.AuthExec: "command"}[a.Kind]
		out += dimStyle.Render(" · ") + errStyle.Render(what+" missing")
	}
	if a.Note != "" && a.Kind != kube.AuthNone {
		out += dimStyle.Render(" · ") + warnStyle.Render(a.Note)
	}
	return out
}

func kcValidText(a kube.AuthInfo) string {
	switch a.Valid.State {
	case kube.ValidOK:
		left := a.Valid.At.Sub(now())
		st := okStyle
		if left < 24*time.Hour {
			st = warnStyle
		}
		return st.Render("valid until "+a.Valid.At.Local().Format("2006-01-02 15:04")) + dimStyle.Render(" (in "+tokenLeft(left)+")")
	case kube.ValidExpired:
		ago := now().Sub(a.Valid.At)
		return errStyle.Render("expired "+a.Valid.At.Local().Format("2006-01-02 15:04")) + dimStyle.Render(" ("+tokenLeft(ago)+" ago)")
	}
	return dimStyle.Render("unknown")
}

func (m model) kcReachText(e *kcLiveEntry) string {
	switch {
	case !m.cfg.UI.KubeconfigLive:
		return dimStyle.Render("○ live checks off")
	case e == nil || !e.done:
		return dimStyle.Render(kcSpin(m) + " checking…")
	case e.live.Reach.OK:
		out := okStyle.Render("● reachable")
		if c := e.live.Reach.Code; c != 0 && c != 200 {
			out += dimStyle.Render(fmt.Sprintf(" · http %d", c))
		}
		return out
	}
	out := errStyle.Render("● unreachable")
	if r := e.live.Reach.Reason; r != "" {
		out += dimStyle.Render(" · " + r)
	}
	return out
}

func (m model) kcAccessText(e *kcLiveEntry) string {
	switch {
	case !m.cfg.UI.KubeconfigLive:
		return dimStyle.Render("○ live checks off")
	case e == nil || !e.done:
		return dimStyle.Render(kcSpin(m) + " checking…")
	}
	a := e.live.Access
	switch a.State {
	case kube.AccessOK:
		out := okStyle.Render("● auth ok")
		if a.User != "" {
			out += dimStyle.Render(" · ") + itemStyle.Render(a.User)
		}
		return out
	case kube.AccessUnauthorized:
		out := errStyle.Render("● 401 unauthorized / expired")
		if a.Reason != "" {
			out += dimStyle.Render(" · " + a.Reason)
		}
		return out
	case kube.AccessForbidden:
		out := warnStyle.Render("● 403 forbidden")
		if a.Reason != "" {
			out += dimStyle.Render(" · " + a.Reason)
		}
		return out
	case kube.AccessExecFailed:
		out := errStyle.Render("● exec failed")
		if a.Reason != "" {
			out += dimStyle.Render(" · " + a.Reason)
		}
		return out
	}
	out := dimStyle.Render("○ unknown")
	if a.Reason != "" {
		out += dimStyle.Render(" · " + a.Reason)
	}
	return out
}

type kcDelegate struct {
	inner list.DefaultDelegate
	width int
}

func (d kcDelegate) Height() int                               { return d.inner.Height() }
func (d kcDelegate) Spacing() int                              { return d.inner.Spacing() }
func (d kcDelegate) Update(msg tea.Msg, l *list.Model) tea.Cmd { return d.inner.Update(msg, l) }

func (d kcDelegate) Render(w io.Writer, l list.Model, index int, it list.Item) {
	if v, ok := it.(item); ok {
		v.title = kcTail(config.HomePath(v.title), max(8, d.width-4))
		it = v
	}
	d.inner.Render(w, l, index, it)
}

func kcTail(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return "…" + string(r[len(r)-(w-1):])
}

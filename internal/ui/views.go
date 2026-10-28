package ui

import (
	"fmt"
	"strings"

	"k2/internal/config"
	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/layout"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) panel(focused bool, width int) lipgloss.Style {
	border := th.Muted
	if focused {
		border = m.accent()
	}
	_, _, panelH := m.layout()
	return lipgloss.NewStyle().
		Width(width).
		Height(panelH).
		Padding(1, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border)
}

func (m model) sidebarBody(boxW, height int) string {

	sel, focused := -1, false
	inner := boxW
	if inner < 10 {
		inner = 10
	}
	var b strings.Builder

	rest := lineCount(m.whereBlock(inner)) + len(m.nav.Items()) + lineCount(m.fwdDock(inner))
	if info := m.infoBlock(inner); info != "" {
		rest += lineCount(info)
	}
	withLogo := strings.Count(m.mark(inner, true), "\n")+rest <= height
	b.WriteString(m.mark(inner, withLogo))
	b.WriteString(m.whereBlock(inner))
	head := b.Len()
	for i, raw := range m.nav.Items() {
		n := raw.(navItem)
		ready := false
		switch n.id {
		case "kube":
			ready = m.kubeconfig != ""
		case "ctx":
			ready = m.context != ""
		case "proj":
			ready = m.namespace != ""
		case "pods":
			ready = m.namespace != ""
		case "pf":
			ready = m.pod != ""
		case "fwds":
			for _, f := range m.forwards {
				if forward.Up(f) {
					ready = true
					break
				}
			}
		case "kust":
			ready = m.kroot != ""
		}
		key := n.key
		if key == "" {
			key = " "
		}
		dot := " "
		if ready {
			dot = "●"
		}
		gap := inner - lipgloss.Width(key+"  "+n.title+" "+dot)
		if gap < 1 {
			gap = 1
		}
		pad := strings.Repeat(" ", gap)
		var line string
		switch {
		case i == sel && focused:
			line = selectedItemStyle.Render(key + "  " + n.title + pad + " " + dot)
		case i == sel:
			line = lipgloss.NewStyle().Foreground(th.Accent).Render(key + "  " + n.title + pad + " " + dot)
		default:
			line = keyStyle.Render(key) + itemStyle.Render("  "+n.title+pad+" ")
			if ready {
				line += okStyle.Render("●")
			} else {
				line += " "
			}
		}
		b.WriteString(clipLine(line, inner) + "\n")
	}
	top, dock := b.String(), m.fwdDock(inner)

	if info := m.infoBlock(inner); info != "" && lineCount(top)+lineCount(info)+lineCount(dock) <= height {
		top = top[:head] + info + top[head:]
	}
	return pinBottom(top, dock, height)
}

func lineCount(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func (m model) fwdDock(inner int) string {
	var rows []string
	var live []forward.Forward
	for _, f := range m.forwards {
		if forward.Up(f) {
			live = append(live, f)
		}
	}
	head := dimStyle.Render(clipLine("⇄  forwards", inner))
	if len(live) == 0 {
		return head + "\n" + dimStyle.Render("   none")
	}
	shown := 0
	for _, f := range live {
		if len(rows) >= m.cfg.UI.ForwardDockRows {
			rows = append(rows, dimStyle.Render(fmt.Sprintf("   +%d", len(live)-shown)))
			break
		}
		ports := f.Local + " → " + f.Remote
		rows = append(rows, okStyle.Render("● ")+itemStyle.Render(ports))
		name := f.Pod
		if f.Namespace != "" {
			name = f.Namespace + "/" + f.Pod
		}
		for _, line := range wrapText(name, max(6, inner-2)) {
			rows = append(rows, itemStyle.Render("  "+line))
		}
		shown++
	}
	return head + "\n" + strings.Join(rows, "\n")
}

func pinBottom(top, bottom string, height int) string {
	if height < 1 {
		height = 1
	}
	topLines := strings.Split(strings.TrimRight(top, "\n"), "\n")
	botLines := strings.Split(strings.TrimRight(bottom, "\n"), "\n")
	if len(botLines) >= height {
		return strings.Join(botLines[len(botLines)-height:], "\n")
	}
	room := height - len(botLines)
	if len(topLines) > room {
		topLines = topLines[:room]
	}
	for len(topLines) < room {
		topLines = append(topLines, "")
	}
	return strings.Join(append(topLines, botLines...), "\n")
}

func (m model) whereBlock(inner int) string {
	n := len(m.tabIDs())
	ctx := or(m.context, "no context")
	ctxStyle := titleStyle
	if m.dangerous() {
		ctx = skull + " " + ctx
		ctxStyle = errStyle
	}
	ns := or(m.namespace, "all namespaces")
	path := config.HomePath(or(m.kubeconfig, "none"))
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }
	line(titleStyle.Render(clipLine(m.whereTitle(n), inner)))
	line(itemStyle.Render(clipLine(m.paneView(), inner)))
	line("")
	line(dimStyle.Render("context"))
	for _, part := range wrapText(ctx, inner) {
		line(ctxStyle.Render(part))
	}
	line(dimStyle.Render("config"))
	for _, part := range wrapText(path, inner) {
		line(itemStyle.Render(part))
	}
	line(dimStyle.Render("namespace"))
	for _, part := range wrapText(ns, inner) {
		line(itemStyle.Render(part))
	}
	line("")
	return b.String()
}

func (m model) whereTitle(n int) string {
	if len(m.tabs) > 1 {
		return fmt.Sprintf("tab %d/%d · pane %d/%d", m.tab+1, len(m.tabs), m.tabPos(m.logID)+1, n)
	}
	return fmt.Sprintf("active · pane %d/%d", m.tabPos(m.logID)+1, n)
}

func (m model) paneView() string {
	switch m.stage {
	case "nav":
		return "home"
	case "logs":
		return logLabel(m.logPod)
	case "workpods":
		return scopeLabel(m.namespace, "pods")
	case "res", "yaml", "view":
		label := or(m.subject, or(m.kind, m.stage))
		if m.stage == "yaml" && m.vs.watchOn {
			label += " · watching"
		}
		return label
	case "describe":
		label := "describe " + m.subject
		if m.vs.watchOn {
			label += " · watching"
		}
		return label
	case "fwds":
		return "forwards"
	case "kdir", "kust":
		return "kustomize"
	case "kcmd":
		return "kubectl"
	case "kout":
		return or(m.subject, "kustomize")
	case "xray":
		return "owner tree"
	case "helm":
		return "helm releases"
	case "pulse":
		return "pulse " + m.namespace
	case "mlogs":
		return m.mlogTitle()
	case "projects":
		return "namespace"
	case "contexts":
		return "context"
	case "files":
		return "kubeconfig"
	default:
		if m.stage == "" {
			return "home"
		}
		return m.stage
	}
}

func pair(k, v string) string {
	return keyStyle.Render(k) + " " + dimStyle.Render(v)
}

func (m model) legend() string {
	k := m.cfg.Keys
	if m.prefixOn {
		return m.prefixLegend()
	}
	if m.repeatOn {
		return m.repeatLegend()
	}
	if m.ed.live() {
		p := k.Prefix.Short()
		parts := []string{
			keyStyle.Render("vim") + " " + dimStyle.Render("· "+p+" k2"),
			pair("^←→↑↓", "panes"),
			pair(k.Fullscreen.Short(), "full"),
			pair(k.ClosePane.PaneShort(k.Prefix), "close"),
			pair(p+" "+k.Quit.Label(), "quit"),
			pair(p+" "+p, "literal"),
		}
		return "  " + strings.Join(parts, dimStyle.Render("    "))
	}
	parts := []string{
		pair("↑↓", "move"),
		pair(k.Select.Label(), "select"),
	}
	if m.stage == "nav" {

		parts = []string{pair(k.Select.Label(), "pods")}
	}
	back, cmd := pair(k.Back.Short(), "back"), pair(k.Command.Short(), "cmd")
	if m.stage == "nav" {
		parts = append(parts, pair(k.Kubeconfig.Short(), "kube"), pair(k.Context.Short(), "ctx"), pair(k.Namespace.Short(), "ns"), pair(k.Kustomize.Short(), "kust"), pair(k.Kubectl.Short(), "kubectl"), pair(k.Forwards.Short(), "forwards"), cmd)
	} else {
		parts = append(parts, back, cmd)
	}
	if m.stage == "workpods" {
		parts = append(parts, pair(k.Describe.Short(), "describe"), pair(k.PortForward.Short(), "forward"), pair(k.Shell.Short(), "shell"), pair(k.SetImage.Short(), "image"), pair(k.Edit.Short(), "edit"), pair(k.Delete.Short(), "delete"),
			pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "all logs"))
		parts = append(parts, m.tableLegend()...)
	} else if m.stage == "res" {
		parts = append(parts, pair(k.Describe.Short(), "describe"))
		if kube.ForwardKind(m.kind) != "" {
			parts = append(parts, pair(k.PortForward.Short(), "forward"))
		}
		parts = append(parts, m.kindLegend()...)
		parts = append(parts, pair(k.Edit.Short(), "edit"), pair(k.Delete.Short(), "delete"))
		if kube.IsSecretKind(m.kind) {
			parts = append(parts, pair(k.Decode.Short(), "decode"))
		}
		parts = append(parts, m.tableLegend()...)
	} else if m.stage == "xray" {

		parts = append(parts, m.xrayFoot(), pair(k.Describe.Short(), "describe"), pair(k.Yaml.Short(), "yaml"),
			pair(k.MultiLogs.Short(), "logs"), pair(k.CopyName.Short(), "copy"))
	} else if m.stage == "helmhist" {
		parts = append(parts, pair(keyName(k.HelmMark.Short()), "mark"), pair(k.HelmDiff.Short(), "diff"), pair(k.HelmValues.Short(), "values"), pair(k.HelmManifest.Short(), "manifest"))
		if m.helmCLI() != "" {
			parts = append(parts, pair(k.HelmRollback.Short(), "roll back to it"))
		}
	} else if m.stage == "helm" {
		parts = append(parts, pair(k.HelmValues.Short(), "values"), pair(k.HelmComputed.Short(), "computed"), pair(k.HelmManifest.Short(), "manifest"),
			pair(k.HelmNotes.Short(), "notes"), pair(k.HelmHistory.Short(), "history"), pair(k.AllNamespaces.Short(), "all ns"))
		if m.helmCLI() != "" {
			parts = append(parts, pair(k.HelmRollback.Short(), "rollback"), pair(k.Delete.Short(), "uninstall"))
		}
		parts = append(parts, m.tableLegend()...)
	} else if m.viewing() {
		parts = append(parts, pair(m.scrollKeys(), "scroll"), pair(k.Wrap.Short(), "wrap"), pair(k.CopyMode.Short(), "copy"), pair(k.Vim.Short(), "vim"))
		if m.stage == "logs" {
			parts = append(parts, pair(k.LogMark.Short(), "mark"), pair(k.Previous.Short(), "previous"))
		}
		if m.stage == "yaml" || m.stage == "describe" {
			parts = append(parts, pair(k.Watch.Short(), m.watchLegend()))
		}
		if m.stage == "mlogs" {
			parts = append(parts, pair(k.LogMark.Short(), "mark"), pair(k.LogFilter.Short(), "filter"))
		}
	} else if m.stage == "kcmd" {
		parts = append(parts, pair(k.Complete.Short(), "complete"), pair(k.Output.Short(), "output"), pair(k.JQ.Short(), "jq"))
	} else if m.stage == "kdir" || m.stage == "kust" {
		parts = append(parts, pair(k.Apply.Short(), "apply"), pair(k.KustomizeDelete.Short(), "delete"), pair(k.Diff.Short(), "diff"), pair(k.Edit.Short(), "vim"), pair(k.KustomizePath.Short(), "path"))
	} else if m.stage == "fwds" {
		parts = append(parts, pair(k.Delete.Short(), "delete"), pair(k.DeleteAll.Short(), "delete all"))
	} else if m.stage != "nav" && m.stage != "logs" && m.stage != "yaml" {
		parts = append(parts, pair("/", "filter"))
	}
	if len(m.tabIDs()) > 1 && !m.zoom {
		parts = append(parts, pair(k.ClosePane.PaneShort(k.Prefix), "close"))
	}
	if len(m.tabs) > 1 && !m.zoom {
		tabs := k.PrevTab.PaneShort(k.Prefix) + "/" + k.NextTab.PaneShort(k.Prefix)
		if a, b := k.PrefixedFor("prev_tab"), k.PrefixedFor("next_tab"); a != "" && b != "" && k.PrevTab.PaneShort(k.Prefix) == short(k.Prefix)+" "+a {
			tabs = short(k.Prefix) + " " + a + "/" + b
		}
		parts = append(parts, pair(tabs, "tabs"))
	}
	if m.zoom {

		head := []string{}
		if m.stage != "nav" {
			head = append(head, back, cmd)
		}
		if m.viewing() {
			head = append(head, pair(k.CopyMode.Short(), "copy"), pair(k.Wrap.Short(), "wrap"), pair(k.Vim.Short(), "vim"))
			if m.stage == "logs" {
				head = append(head, pair(k.LogMark.Short(), "mark"), pair(k.Previous.Short(), "previous"))
			}
			if m.stage == "yaml" || m.stage == "describe" {
				head = append(head, pair(k.Watch.Short(), m.watchLegend()))
			}
			if m.stage == "mlogs" {
				head = append(head, pair(k.LogMark.Short(), "mark"), pair(k.LogFilter.Short(), "filter"))
			}
		}
		head = append(head, pair(k.Fullscreen.Short(), "split"), pair(k.Quit.Short(), "quit"))
		return "  " + strings.Join(head, dimStyle.Render("    "))
	}
	parts = append(parts, pair(k.Fullscreen.Short(), "full"))
	parts = append(parts, pair(k.Quit.Short(), "quit"))
	return "  " + strings.Join(parts, dimStyle.Render("    "))
}

func (m model) watchLegend() string {
	if m.vs.watchOn {
		return "watching"
	}
	return "watch"
}

func (m model) kindLegend() []string {
	k := m.cfg.Keys
	var parts []string
	switch kube.BaseKind(m.kind) {
	case "deployments", "statefulsets":
		parts = append(parts, pair(k.Scale.Short(), "scale"), pair(k.Restart.Short(), "restart"), pair(k.SetImage.Short(), "image"), pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "logs"))
	case "replicasets":
		parts = append(parts, pair(k.Scale.Short(), "scale"), pair(k.SetImage.Short(), "image"), pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "logs"))
	case "daemonsets":
		parts = append(parts, pair(k.Restart.Short(), "restart"), pair(k.SetImage.Short(), "image"), pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "logs"))
	case "cronjobs":
		parts = append(parts, pair(k.Trigger.Short(), "trigger"), pair(k.SetImage.Short(), "image"), pair(k.Xray.Short(), "xray"))
	case "services":
		parts = append(parts, pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "logs"))
	case "installplans":
		parts = append(parts, pair(k.Approve.Short(), "approve"))
	case "subscriptions":
		parts = append(parts, pair(k.Select.Short(), "csv"))
	case "clusterserviceversions":
		parts = append(parts, pair(k.Select.Short(), "owned"))
	case "customresourcedefinitions":
		parts = append(parts, pair(k.Select.Short(), "list CRs"))
	case "jobs":
		parts = append(parts, pair(k.Select.Short(), "pods"), pair(k.Xray.Short(), "xray"), pair(k.MultiLogs.Short(), "logs"))
	}
	return parts
}

func (m model) tableLegend() []string {
	k := m.cfg.Keys
	return []string{pair("/", "filter"), pair(k.SortNext.Short()+k.SortPrev.Short(), "sort"), pair(k.CopyName.Short(), "copy")}
}

func (m model) podsHint() string {
	k := m.cfg.Keys
	return k.Select.Label() + " logs, " + k.Describe.Label() + " describe, " + k.PortForward.Label() + " forward, " + k.Shell.Label() + " shell, " + k.Edit.Label() + " edit, " + k.Delete.Label() + " delete"
}

func (m model) scrollKeys() string {
	return glyph(m.cfg.Keys.ScrollDown.Short()) + glyph(m.cfg.Keys.ScrollUp.Short())
}

func glyph(k string) string {
	if a, ok := arrowGlyphs[k]; ok {
		return a
	}
	return k
}

var arrowGlyphs = map[string]string{"up": "↑", "down": "↓", "left": "←", "right": "→"}

func (m model) yn() string {
	return m.cfg.Keys.Yes.Label() + " / " + m.cfg.Keys.No.Label()
}

func (m model) statusBody(innerW int) string {
	ctx := m.context
	if ctx == "" {
		ctx = "no context"
	}
	lines := []string{
		titleStyle.Render(iconSlot("◉") + " " + ctx),
		dimStyle.Render(clipLine("   "+config.HomePath(or(m.kubeconfig, "no kubeconfig")), innerW)),
		dimStyle.Render("   " + or(m.namespace, "all namespaces")),
		"",
		titleStyle.Render(iconSlot("⇄") + " forwards"),
	}
	if len(m.forwards) == 0 {
		lines = append(lines, dimStyle.Render("   none"))
	}
	for _, f := range m.forwards {
		mark := errStyle.Render("○")
		age := "dead"
		if f.Reason != "" {
			age = "dead · " + f.Reason
		}
		if forward.Up(f) {
			mark = okStyle.Render("●")
			age = forward.Age(f.Started)
		}
		where := fitWidth(forward.Where(f), max(12, innerW/2))
		target := fitWidth(f.Namespace+"/"+f.Pod, max(8, innerW-len(f.Local)-len(where)-16))
		row := clipLine(lipgloss.JoinHorizontal(lipgloss.Top,
			mark,
			itemStyle.Render(" "+f.Local),
			dimStyle.Render("  "+where),
			dimStyle.Render("  "+target),
			dimStyle.Render("  "+age),
		), innerW)
		lines = append(lines, row)
	}

	lines = append(lines, "")
	for _, row := range wrapLine(m.status, innerW) {
		lines = append(lines, dimStyle.Render(row))
	}

	k := m.cfg.Keys
	hint := func(key, cmd, what string) string {
		return keyStyle.Render(fmt.Sprintf("%-3s", key)) + " " + dimStyle.Render(fmt.Sprintf("%-12s %s", ":"+cmd, what))
	}
	hints := []string{
		hint(k.Select.Label(), "pods", "list pods"),
		hint(k.Kubeconfig.Short(), "kubeconfig", "pick a kubeconfig file"),
		hint(k.Context.Short(), "context", "pick a context"),
		hint(k.Namespace.Short(), "ns", "pick a namespace"),
		hint(k.Kustomize.Short(), "kustomize", "browse kustomizations"),
		hint(k.Kubectl.Short(), "kubectl", "run kubectl"),
		hint(k.Forwards.Short(), "forwards", "port-forwards"),
		hint(k.Favorites.Short(), "fav", "favorites: pinned context + namespace"),
		hint(k.Theme.Short(), "theme", "theme for this context, saved to the config"),
		hint("", "pulse", "health of the namespace"),
		hint("", "helm", "helm releases"),
		hint("", "xray deploy", "owner trees (tab completes kinds and names)"),
		hint("", "logs <sel>", "logs of many pods"),
	}
	room := 0
	if m.boxH > len(lines)+1 {
		room = m.boxH - len(lines) - 1
	}
	if room > 0 && len(hints) > room {
		hints = flowDown(hints, innerW, room)
	}
	lines = append(lines, "")
	lines = append(lines, hints...)
	lines = pageLines(lines, m.vs.homeAt, m.boxH)
	for i := range lines {
		lines[i] = clipLine(lines[i], innerW)
	}
	return strings.Join(lines, "\n")
}

func flowDown(hints []string, width, rows int) []string {
	if rows < 1 || len(hints) == 0 || width < 16 {
		return hints
	}
	maxW := 1
	for _, h := range hints {
		if w := ansi.StringWidth(h); w > maxW {
			maxW = w
		}
	}
	gutter := 2
	cols := 1
	if maxW+gutter > 0 {
		cols = width / (maxW + gutter)
	}
	if cols < 2 && width >= 56 {
		cols = width / 28
		if cols < 2 {
			cols = 2
		}
		maxW = width/cols - gutter
		if maxW < 16 {
			return hints
		}
	}
	if cols < 2 {
		return hints
	}
	need := (len(hints) + rows - 1) / rows
	if need < cols {
		cols = need
	}
	height := (len(hints) + cols - 1) / cols
	out := make([]string, height)
	step := maxW + gutter
	for i, h := range hints {
		col := i / height
		row := i % height
		cell := clipLine(h, maxW)
		if col > 0 {
			out[row] = padTo(out[row], col*step)
		}
		out[row] += cell
	}
	return out
}

func padTo(s string, n int) string {
	w := ansi.StringWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func pageLines(lines []string, at, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if at < 0 {
		at = 0
	}
	if at > len(lines)-height {
		at = len(lines) - height
	}
	return lines[at:]
}

func (m model) rightBody(innerW int) string {
	switch m.stage {
	case "workpods", "res", "xray", "helm", "helmhist":
		m.content.SetDelegate(rowDelegate{width: innerW, marks: m.vs.marks})
		reflowList(&m.content, innerW)
		head := dimStyle.Bold(true).Render(clipLine(tableHeader(m.content, m.vs), innerW))
		if len(m.content.Items()) == 0 && !m.load.active() {
			if hint := m.emptyHint(); hint != "" {

				return head + "\n" + dimStyle.Render(clipLine(hint, innerW))
			}
		}
		return head + "\n" + m.content.View()
	case "logs", "yaml", "view", "kout", "describe", "mlogs", "pulse", "hdiff":
		return m.logsBody(innerW)
	case "fwds":
		head := ""
		if len(m.forwards) > 0 {
			head = dimStyle.Bold(true).Render(clipLine(" "+m.forwardHeader(), innerW)) + "\n"
		}
		return head + m.content.View()
	case "kcmd":
		return m.shellBody(innerW)
	case "nav":
		return m.statusBody(innerW)
	default:
		return m.content.View()
	}
}

func (m model) View() string {
	if m.copy != nil {
		return m.copyView()
	}
	out := m.withDialog(m.withPopup(m.screen()))
	if m.paintBg {
		out = paintBackground(out, m.width, m.height)
	}
	return out
}

func (m model) screen() string {
	if m.zoom {
		return m.zoomView()
	}
	sideW, mainW, panelH := m.layout()
	leftFocus := false
	area := m.paneArea()
	innerW, innerH := area.W, area.H
	rightBody := padLines(m.renderNode(m.tabs[m.tab].root, innerW, innerH), innerH)
	activeRight := true
	sidebar := m.panel(leftFocus, sideW).Render(crop(m.sidebarBody(sideW-2, panelH-2), sideW-2, panelH-2))
	right := m.panelBox(activeRight && !leftFocus, mainW, panelH).Render(rightBody)
	return clampWidth(m.withTabBar(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, sidebar, right),
		m.legend(),
	)), max(1, m.width))
}

func (m model) renderNode(n *layout.Node, w, h int) string {
	if n == nil {
		return ""
	}
	if n.Dir == layout.Leaf {
		i := m.paneIdx(n.Pane)
		if i < 0 {
			return padLines("", h)
		}
		return m.paneBlock(i, w, max(h, 2))
	}
	var parts []string
	if n.Dir == layout.Col {
		for i, size := range layout.Shares(h, len(n.Kids)) {
			parts = append(parts, m.renderNode(n.Kids[i], w, size))
		}
		return strings.Join(parts, "\n")
	}
	for i, size := range layout.Shares(w, len(n.Kids)) {
		parts = append(parts, m.renderNode(n.Kids[i], size, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m model) zoomView() string {
	if m.ed.live() && m.ed.kind == "shell" {
		w, h := m.editorSize(m.logID)
		body := m.ed.body(!m.pfOn && m.dlg == nil, w, h, m.cfg.Keys.Back.Label())
		return clampWidth(m.withTabBar(lipgloss.JoinVertical(lipgloss.Left, body, m.legend())), max(1, m.width))
	}
	_, mainW, panelH := m.layout()
	box := m.paneBlock(m.focus, mainW, panelH)
	return clampWidth(m.withTabBar(lipgloss.JoinVertical(lipgloss.Left, box, m.legend())), max(1, m.width))
}

func (m model) paneBlock(i, width, h int) string {
	bodyW := width - 4
	if bodyW < 4 {
		bodyW = 4
	}
	body := ""
	ed := m.ed
	if i != m.focus && i < len(m.panes) {
		ed = m.panes[i].ed
	}
	switch {
	case ed != nil:
		body = ed.body(i == m.focus && !m.pfOn && m.dlg == nil, bodyW, h-2, m.cfg.Keys.Back.Label())
	case i != m.focus && i < len(m.panes):
		body = m.snapBody(m.panes[i], bodyW, h-2)
	default:
		m.boxH = h - 2
		body = m.rightBody(bodyW)
	}
	if m.cmdOn && i == m.focus && h > 4 {
		hint := m.cmdHint
		if hint == "" {
			hint = m.cfg.Keys.Complete.Label() + " completes"
		}
		foot := clipLine(m.cmd.View(), bodyW) + "\n" + dimStyle.Render(clipLine(hint, bodyW))
		body = padLines(crop(body, bodyW, h-4), h-4) + "\n" + foot
	}
	foot := ""
	if i == m.focus {
		foot = m.toastLabel(width - 2)
	}
	hot, active := m.paneHot(i), i == m.focus
	label := m.title(i).render(width-4, borderStyle(active, hot))
	g := roundGlyphs
	if active {
		g = glyphsFor(m.cfg.UI.ActiveBorder)
	}
	return drawBoxWith(g, body, width, h, active, label, foot, hot)
}

type boxGlyphs struct{ tl, tr, bl, br, h, v string }

var (
	roundGlyphs  = boxGlyphs{"╭", "╮", "╰", "╯", "─", "│"}
	thickGlyphs  = boxGlyphs{"┏", "┓", "┗", "┛", "━", "┃"}
	doubleGlyphs = boxGlyphs{"╔", "╗", "╚", "╝", "═", "║"}
)

func glyphsFor(style string) boxGlyphs {
	switch style {
	case "thick":
		return thickGlyphs
	case "double":
		return doubleGlyphs
	}
	return roundGlyphs
}

func (m model) panelBox(focused bool, width, height int) lipgloss.Style {
	border := th.Muted
	if focused {
		border = m.accent()
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border)
}

func borderStyle(active, hot bool) lipgloss.Style {
	switch {
	case hot:
		return errStyle
	case active:
		return lipgloss.NewStyle().Foreground(th.Accent)
	}
	return dimStyle
}

func drawBox(content string, width, height int, active bool, label, foot string, hot bool) string {
	return drawBoxWith(roundGlyphs, content, width, height, active, label, foot, hot)
}

func drawBoxWith(g boxGlyphs, content string, width, height int, active bool, label, foot string, hot bool) string {
	if width < 4 {
		width = 4
	}
	if height < 2 {
		height = 2
	}
	innerW := width - 2
	textW := innerW - 2
	if textW < 1 {
		textW = 1
	}
	innerH := height - 2
	if innerH < 0 {
		innerH = 0
	}
	lines := []string{}
	if innerH > 0 {
		lines = strings.Split(padLines(crop(content, textW, innerH), innerH), "\n")
	}
	st := borderStyle(active, hot)
	mark := " " + label + " "
	if ansi.StringWidth(mark) > innerW {
		mark = clipLine(mark, innerW)
	}
	rest := innerW - ansi.StringWidth(mark)
	if rest < 0 {
		rest = 0
	}
	top := st.Render(g.tl + mark + strings.Repeat(g.h, rest) + g.tr)
	if strings.Contains(label, "\x1b[") {

		top = st.Render(g.tl+" ") + clipLine(label, innerW-2) + st.Render(" "+strings.Repeat(g.h, rest)+g.tr)
	}
	bot := st.Render(g.bl + strings.Repeat(g.h, innerW) + g.br)
	if fw := ansi.StringWidth(foot); foot != "" && fw+3 <= innerW {
		bot = st.Render(g.bl+strings.Repeat(g.h, innerW-fw-2)) + foot + st.Render(g.h+g.h+g.br)
	}
	var b strings.Builder
	b.WriteString(top)
	for _, ln := range lines {
		b.WriteString("\n")
		b.WriteString(st.Render(g.v))
		b.WriteString(" " + padPlain(ln, textW) + " ")
		b.WriteString(st.Render(g.v))
	}
	b.WriteString("\n")
	b.WriteString(bot)
	return padLines(b.String(), height)
}

package ui

import (
	"strconv"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type helpRow struct{ keys, what string }

type helpSection struct {
	title string
	rows  []helpRow
}

func (m model) openHelp() (tea.Model, tea.Cmd) {
	m.dlg = &dialog{act: "help", title: "Help · " + m.helpView()}
	return m, nil
}

func (m model) helpView() string {
	if m.ed != nil {
		return m.ed.tool()
	}
	return m.paneView()
}

func keyList(b config.Binding, n int) string {
	var out []string
	for _, k := range b {
		if len(out) == n {
			break
		}
		out = append(out, glyph(keyName(strings.TrimPrefix(k, config.DirectMark))))
	}
	return strings.Join(out, "/")
}

func duo(a, b config.Binding) string {
	if len(a) == 0 || len(b) == 0 {
		return keyList(a, 1) + keyList(b, 1)
	}
	return keyList(a, 1) + "/" + keyList(b, 1)
}

func (m model) paneKey(b config.Binding, n int) string {
	var direct, after []string
	for _, s := range b {
		if d, ok := strings.CutPrefix(s, config.DirectMark); ok {
			direct = append(direct, d)
		} else if len(after) < n {
			after = append(after, s)
		}
	}
	parts := direct
	if len(after) > 0 {
		parts = append(parts, m.cfg.Keys.Prefix.Label()+" "+strings.Join(after, "/"))
	}
	return strings.Join(parts, ", ")
}

func arrows(keys []string) string {
	pre, out := "", ""
	for i, k := range keys {
		p, s := "", k
		if j := strings.LastIndex(k, "+"); j > 0 {
			p, s = k[:j+1], k[j+1:]
		}
		g, ok := arrowGlyphs[s]
		if !ok || i > 0 && p != pre {
			return strings.Join(keys, "/")
		}
		pre, out = p, out+g
	}
	return pre + out
}

func (m model) moveRows() []helpRow {
	k := m.cfg.Keys
	var direct, after []string
	for _, b := range []config.Binding{k.PaneLeft, k.PaneRight, k.PaneUp, k.PaneDown} {
		for _, s := range b {
			if d, ok := strings.CutPrefix(s, config.DirectMark); ok {
				direct = append(direct, d)
				break
			}
		}
		for _, s := range b {
			if !strings.HasPrefix(s, config.DirectMark) {
				after = append(after, s)
				break
			}
		}
	}
	var rows []helpRow
	if len(after) == 4 {
		rows = append(rows, helpRow{k.Prefix.Label() + " " + arrows(after), "move to a pane"})
	}
	if len(direct) == 4 {
		rows = append(rows, helpRow{arrows(direct), "same, no prefix"})
	}
	return rows
}

func after(b config.Binding, last bool) string {
	out := ""
	for _, s := range b {
		if !strings.HasPrefix(s, config.DirectMark) {
			out = s
			if !last {
				break
			}
		}
	}
	return out
}

func (m model) prefixed(keys ...string) string {
	var out []string
	for _, k := range keys {
		if k == "" {
			return ""
		}
		out = append(out, k)
	}
	return m.cfg.Keys.Prefix.Label() + " " + strings.Join(out, " ")
}

func (m model) navSection() helpSection {
	k := m.cfg.Keys
	rows := []helpRow{
		{duo(k.ScrollDown, k.ScrollUp), "move down / up"},
		{duo(k.Top, k.Bottom), "top / bottom"},
		{keyList(k.Select, 1), "open / select"},
		{keyList(k.Back, 2), "back"},
		{keyList(k.Command, 1), "command line"},
		{"/", "filter the list"},
		{keyList(k.Context, 1) + "/" + keyList(k.Kubeconfig, 1) + "/" + keyList(k.Namespace, 1), "context / kubeconfig / ns"},
		{keyList(k.Kustomize, 1) + "/" + keyList(k.Kubectl, 1) + "/" + keyList(k.Forwards, 1), "kust / kubectl / forwards"},
		{m.prefixed(after(k.SplitRight, true), after(k.SplitDown, true)), "split right / down"},
	}
	rows = append(rows, m.moveRows()...)
	rows = append(rows,
		helpRow{strings.Trim(m.paneKey(k.Zoom, 1)+", "+keyList(k.Fullscreen, 1), ", "), "zoom the pane"},
		helpRow{m.paneKey(k.ClosePane, 1), "close the pane"},
		helpRow{m.prefixed(after(k.NewTab, true), after(k.NextTab, false), after(k.PrevTab, false)), "new / next / prev tab"},
		helpRow{m.prefixed(after(k.Tab[0], false) + "-" + after(k.Tab[8], false)), "go to tab"},
		helpRow{keyList(k.LastError, 1), "last error"},
		helpRow{keyList(k.ReadOnly, 1), "view-only mode"},
		helpRow{keyList(k.Help, 1) + "/" + keyList(k.Cancel, 1), "close this help"},
		helpRow{keyList(k.Quit, 1), "quit"},
	)
	return helpSection{"Navigation", rows}
}

func (m model) tableRows() []helpRow {
	k := m.cfg.Keys
	return []helpRow{
		{keyList(k.Mark, 1), "mark rows"},
		{keyList(k.Label, 1) + "/" + keyList(k.Annotate, 1), "label / annotate"},
		{keyList(k.CopyName, 1) + "/" + keyList(k.CopyRef, 1), "copy name / ref"},
		{keyList(k.SortNext, 1) + keyList(k.SortPrev, 1) + " " + keyList(k.SortReverse, 1), "sort, reverse"},
		{keyList(k.AllNamespaces, 1), "all namespaces"},
	}
}

func (m model) viewerRows() []helpRow {
	k := m.cfg.Keys
	rows := []helpRow{
		{keyList(k.Bottom, 1), "follow the end"},
		{keyList(k.Wrap, 1), "wrap lines"},
		{keyList(k.CopyMode, 1), "copy mode"},
		{keyList(k.Vim, 1), "open in vim"},
	}
	switch m.stage {
	case "mlogs":
		rows = append(rows, helpRow{keyList(k.LogFilter, 1), "filter lines"}, helpRow{keyList(k.LogMark, 1), "mark a line"},
			helpRow{keyList(k.AllNamespaces, 1), "all namespaces"})
	case "logs":
		rows = append(rows, helpRow{keyList(k.Search, 1), "search"}, helpRow{keyList(k.LogMark, 1), "mark a line"},
			helpRow{keyList(k.Previous, 1), "previous container"}, helpRow{keyList(k.Log30, 1) + "/" + keyList(k.Log60, 1), "last 30m / 60m"})
	default:
		rows = append(rows, helpRow{keyList(k.Search, 1), "search"})
	}
	if m.stage == "yaml" || m.stage == "describe" {
		rows = append(rows, helpRow{keyList(k.Watch, 1), "watch for changes"})
	}
	if m.stage == "pulse" {
		rows = append(rows, helpRow{keyList(k.AllNamespaces, 1), "all namespaces"})
	}
	return rows
}

func (m model) kindRows() []helpRow {
	k := m.cfg.Keys
	scale, restart, image := helpRow{keyList(k.Scale, 1), "scale"}, helpRow{keyList(k.Restart, 1), "rollout restart"}, helpRow{keyList(k.SetImage, 1), "set image"}
	xray, logs := helpRow{keyList(k.Xray, 1), "owner tree"}, helpRow{keyList(k.MultiLogs, 1), "logs of its pods"}
	switch kube.BaseKind(m.kind) {
	case "deployments", "statefulsets":
		return []helpRow{scale, restart, image, xray, logs}
	case "replicasets":
		return []helpRow{scale, image, xray, logs}
	case "daemonsets":
		return []helpRow{restart, image, xray, logs}
	case "cronjobs":
		return []helpRow{{keyList(k.Trigger, 1), "run a job now"}, image, xray}
	case "services", "jobs":
		return []helpRow{xray, logs}
	case "installplans":
		return []helpRow{{keyList(k.Approve, 1), "approve"}}
	}
	return nil
}

func (m model) viewSection() []helpSection {
	k := m.cfg.Keys
	r := func(b config.Binding, what string) helpRow { return helpRow{keyList(b, 1), what} }
	if m.ed != nil {
		p := k.Prefix.Label()
		rows := []helpRow{
			{"typing", "goes to " + m.ed.tool()},
			{p, "then a pane or tab key"},
			{p + " " + keyList(k.Help, 1), "this help"},
			{p + " " + p, "send a literal " + p},
		}
		rows = append(rows, m.moveRows()...)
		rows = append(rows, r(k.Fullscreen, "zoom the pane"), helpRow{m.paneKey(k.ClosePane, 1), "close the pane"},
			helpRow{keyList(k.Back, 2), "back, once it exits"})
		return []helpSection{{"Shell / vim pane", rows}}
	}
	switch m.stage {
	case "nav", "":
		return []helpSection{{"Home", []helpRow{
			r(k.Select, "list pods"),
			r(k.Theme, "theme for this context"),
			r(k.Login, "paste a token"),
			r(k.Profile, "save the layout"),
			r(k.Delete, "close pane / tab / quit"),
			{k.Command.Label() + "helm", "helm releases"},
			{k.Command.Label() + "pulse", "namespace health"},
			{k.Command.Label() + "xray deploy", "owner trees"},
			{k.Command.Label() + "logs app=x", "logs of many pods"},
		}}}
	case "workpods":
		rows := []helpRow{
			{keyList(k.Select, 1), "logs"},
			r(k.Describe, "describe"),
			{keyList(k.PortForward, 2), "port-forward"},
			r(k.Shell, "shell"),
			r(k.Edit, "edit in vim"),
			r(k.Delete, "delete"),
			r(k.Scale, "scale its owner"),
			r(k.SetImage, "set image"),
			r(k.Xray, "owner tree"),
			r(k.MultiLogs, "logs of marked pods"),
		}
		return []helpSection{{"Pods", append(rows, m.tableRows()...)}}
	case "res":
		rows := []helpRow{{keyList(k.Select, 1), "open"}, r(k.Describe, "describe")}
		if kube.ForwardKind(m.kind) != "" {
			rows = append(rows, helpRow{keyList(k.PortForward, 1), "port-forward"})
		}
		rows = append(rows, m.kindRows()...)
		rows = append(rows, r(k.Edit, "edit in vim"), r(k.Delete, "delete"))
		if kube.IsSecretKind(m.kind) {
			rows = append(rows, r(k.Decode, "decode the secret"))
		}
		return []helpSection{{"Resources · " + or(m.kind, "list"), append(rows, m.tableRows()...)}}
	case "logs", "mlogs":
		return []helpSection{{"Logs", m.viewerRows()}}
	case "yaml", "describe", "view", "kout", "hdiff", "pulse":
		return []helpSection{{"Viewer", m.viewerRows()}}
	case "kdir", "kust":
		rows := []helpRow{{keyList(k.Select, 1), "open folder / file"}}
		if m.stage == "kdir" {
			rows = append(rows, r(k.Scan, "scan for kustomizations"))
		}
		rows = append(rows,
			r(k.Apply, "apply"),
			r(k.Diff, "diff against the cluster"),
			r(k.KustomizeDelete, "delete from the cluster"),
			r(k.Edit, "edit in vim"),
			r(k.KustomizePath, "go to a directory"),
			r(k.OpenFolder, "open folder in "+m.folderApp()),
		)
		return []helpSection{{"Kustomize", rows}}
	case "fwds":
		return []helpSection{
			{"Forwards", []helpRow{
				r(k.Delete, "stop and remove"),
				r(k.DeleteAll, "stop all"),
				r(k.PfRestart, "restart"),
				r(k.PfAuto, "auto restart on / off"),
			}},
			{"New forward", []helpRow{
				{keyList(k.PortForward, 2), "in a pods list"},
				r(k.Complete, "next suggested port"),
				r(k.Submit, "start"),
				r(k.Cancel, "cancel"),
			}},
		}
	case "helm":
		rows := []helpRow{
			r(k.HelmValues, "values"),
			r(k.HelmComputed, "computed values"),
			r(k.HelmManifest, "manifest"),
			r(k.HelmNotes, "notes"),
			r(k.HelmHistory, "history"),
		}
		if m.helmCLI() != "" {
			rows = append(rows, r(k.HelmRollback, "roll back"), r(k.Delete, "uninstall"))
		}
		return []helpSection{{"Helm", append(rows, r(k.AllNamespaces, "all namespaces"), r(k.CopyName, "copy name"))}}
	case "helmhist":
		rows := []helpRow{
			r(k.HelmMark, "mark a revision"),
			r(k.HelmDiff, "diff marked"),
			r(k.HelmValues, "values"),
			r(k.HelmManifest, "manifest"),
		}
		if m.helmCLI() != "" {
			rows = append(rows, r(k.HelmRollback, "roll back to it"))
		}
		return []helpSection{{"Helm history", rows}}
	case "xray":
		return []helpSection{{"Owner tree", []helpRow{
			r(k.XrayToggle, "fold / unfold"),
			{keyList(k.XrayExpand, 1) + "/" + keyList(k.XrayCollapse, 1), "expand / collapse all"},
			r(k.XrayFilter, "filter"),
			r(k.Describe, "describe"),
			r(k.Yaml, "yaml"),
			r(k.MultiLogs, "logs"),
			r(k.CopyName, "copy name"),
		}}}
	case "kcmd":
		return []helpSection{{"kubectl", []helpRow{
			r(k.Submit, "run"),
			r(k.Complete, "complete"),
			r(k.Output, "output format"),
			r(k.JQ, "jq filter"),
			r(k.ShellBack, "back"),
		}}}
	case "files", "contexts", "projects":
		return []helpSection{{"Pick a " + m.paneView(), []helpRow{
			r(k.Select, "choose"),
			{"/", "filter"},
			r(k.Favorites, "favorites"),
			r(k.Back, "back"),
		}}}
	}
	return []helpSection{{"Actions", []helpRow{
		r(k.Select, "open"),
		{"/", "filter"},
		r(k.Back, "back"),
	}}}
}

func (m model) helpSections() []helpSection {
	var out []helpSection
	for _, s := range append([]helpSection{m.navSection()}, m.viewSection()...) {
		var rows []helpRow
		for _, r := range s.rows {
			if strings.TrimSpace(r.keys) != "" {
				rows = append(rows, r)
			}
		}
		out = append(out, helpSection{s.title, rows})
	}
	return out
}

func (s helpSection) widths() (kw, all int) {
	dw := 0
	for _, r := range s.rows {
		kw = max(kw, ansi.StringWidth(r.keys))
		dw = max(dw, ansi.StringWidth(r.what))
	}
	kw = min(kw, 16)
	return kw, max(kw+2+dw, ansi.StringWidth(s.title))
}

func (s helpSection) lines(width int) []string {
	kw, _ := s.widths()
	kw = min(kw, max(width/2, 1))
	out := []string{titleStyle.Render(clipLine(s.title, width))}
	for _, r := range s.rows {
		keys := padTo(clipLine(r.keys, kw), kw)
		out = append(out, keyStyle.Render(keys)+"  "+itemStyle.Render(clipLine(r.what, max(width-kw-2, 1))))
	}
	return out
}

func stack(secs []helpSection, width int) []string {
	var out []string
	for i, s := range secs {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, s.lines(width)...)
	}
	return out
}

func (m model) helpLayout() (lines []string, w, room int) {
	secs := m.helpSections()
	maxW := max(min(m.width-2, 110), 4)
	if m.width < 40 {
		maxW = max(m.width, 4)
	}
	inner := max(maxW-4, 1)
	_, left := secs[0].widths()
	right := 0
	for _, s := range secs[1:] {
		_, sw := s.widths()
		right = max(right, sw)
	}
	gap := 3
	if len(secs) > 1 && inner-left-gap >= 24 {
		right = min(right, inner-left-gap)
		l, r := secs[0].lines(left), stack(secs[1:], right)
		for i := 0; i < max(len(l), len(r)); i++ {
			row := ""
			if i < len(l) {
				row = l[i]
			}
			if i < len(r) {
				row = padTo(row, left+gap) + r[i]
			}
			lines = append(lines, row)
		}
	} else {
		lines = stack(secs, min(inner, max(left, right)))
	}
	content := ansi.StringWidth(m.dlg.title) + 2
	for _, l := range lines {
		content = max(content, ansi.StringWidth(l))
	}
	w = min(content+4, maxW)
	maxH := m.height
	if m.height >= 16 {
		maxH = m.height - 2
	}
	room = max(min(len(lines), maxH-2), 1)
	return lines, w, room
}

func (m model) helpBox() (string, int, int) {
	lines, w, room := m.helpLayout()
	top := min(max(m.dlg.idx, 0), max(len(lines)-room, 0))
	end := min(top+room, len(lines))
	foot := ""
	k := m.cfg.Keys
	switch {
	case end < len(lines):
		foot = dimStyle.Render(" ↓ " + strconv.Itoa(len(lines)-end) + " more ")
	case top > 0:
		foot = dimStyle.Render(" ↑ " + duo(k.ScrollDown, k.ScrollUp) + " ")
	default:
		foot = dimStyle.Render(" " + keyList(k.Help, 1) + " close ")
	}
	box := drawBox(strings.Join(lines[top:end], "\n"), w, room+2, true, m.dlg.title, foot, false)
	return box, w, room + 2
}

func (m model) helpKey(key string) (tea.Model, tea.Cmd) {
	km := m.cfg.Keys
	_, _, room := m.helpLayout()
	switch {
	case km.Help.Has(key) || km.Cancel.Has(key) || km.Back.Has(key):
		m.dlg = nil
	case km.ScrollDown.Has(key):
		m.scrollHelp(1)
	case km.ScrollUp.Has(key):
		m.scrollHelp(-1)
	case km.PageDown.Has(key):
		m.scrollHelp(room)
	case km.PageUp.Has(key):
		m.scrollHelp(-room)
	case km.Top.Has(key):
		m.scrollHelp(-1 << 20)
	case km.Bottom.Has(key):
		m.scrollHelp(1 << 20)
	}
	return m, nil
}

func (m *model) scrollHelp(step int) {
	lines, _, room := m.helpLayout()
	d := *m.dlg
	d.idx = min(max(min(d.idx, len(lines)-room)+step, 0), max(len(lines)-room, 0))
	m.dlg = &d
}

func (m model) helpWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		m.scrollHelp(3)
	case tea.MouseButtonWheelUp:
		m.scrollHelp(-3)
	}
	return m, nil
}

func (m model) withHelp(screen string) string {
	box, w, h := m.helpBox()
	return overlay(screen, box, max((m.width-w)/2, 0), max((m.height-h)/2, 0))
}

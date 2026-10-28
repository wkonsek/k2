package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) fetchYAML(id, ns string) tea.Cmd {
	kind := m.kind
	pane := m.logID
	kc, ctx := m.kubeconfig, m.context
	ns, name := kube.SplitRef(id, ns)
	if ns == "" {
		ns = m.namespace
	}
	return func() tea.Msg {
		text, err := m.k.YAML(kc, ctx, kind, name, ns)
		if err != nil {
			return yamlMsg{id: pane, err: err}
		}
		return yamlMsg{id: pane, text: text}
	}
}

func (m *model) applyYAML(msg yamlMsg) {
	set := func(stage, text string) (string, string) {
		if msg.err != nil {
			return "res", m.errLine(msg.err)
		}
		return "yaml", msg.text
	}
	if m.logID == msg.id {
		m.load = load{}
		if msg.err != nil {
			m.stage = or(m.vs.vback, "res")
			m.vs.vback = ""
			m.status = m.errLine(msg.err)
			return
		}
		m.stage = "yaml"
		m.logText = msg.text
		m.status = m.kind
		return
	}
	for i := range m.panes {
		if m.panes[i].logID != msg.id {
			continue
		}
		m.panes[i].load = load{}
		if msg.err != nil {
			m.panes[i].stage = or(m.panes[i].vs.vback, "res")
			m.panes[i].vs.vback = ""
			m.panes[i].status = m.errLine(msg.err)
			return
		}
		m.panes[i].stage = "yaml"
		m.panes[i].logText = msg.text
		return
	}
	_, _ = set("", "")
}

func (m model) viewing() bool {
	return m.stage == "logs" || m.stage == "yaml" || m.stage == "view" || m.stage == "kout" || m.stage == "describe" ||
		m.stage == "mlogs" || m.stage == "pulse" || m.stage == "hdiff"
}

func (m model) textRows() int {
	_, h := m.paneBox(m.logID)
	rows := h - 4
	if rows < 4 {
		rows = 4
	}
	return rows
}

func (m model) textLines() []string {
	if m.copy != nil && m.shellYank != nil {
		return m.shellYank
	}
	text := strings.TrimRight(m.logText, "\n")
	if text == "" {
		if m.stage == "logs" {
			return []string{"waiting for logs…"}
		}
		if m.stage == "mlogs" {
			if v := m.vs.ml; v != nil && v.filter != "" && len(v.lines) > 0 {
				return []string{"no line matches the filter " + v.filter}
			}
			return []string{"waiting for logs…"}
		}
		return []string{"empty"}
	}

	return strings.Split(sanitizeText(text), "\n")
}

func (m *model) scroll(delta int) {
	vis := m.visualLines(m.viewerWidth())
	total := len(vis)
	rows := m.textRows()
	maxTop := total - rows
	if maxTop < 1 {
		m.logStick = true
		m.logAt = 0
		m.clearLogHold()
		return
	}
	top := m.shownTop(vis, rows)
	top += delta
	if top < 0 {
		top = 0
	}
	if top >= maxTop {
		m.logStick = true
		m.logAt = 0
		m.clearLogHold()
		return
	}
	m.logStick = false
	m.logAt = top
	m.pinLog(vis, top)
}

func (m model) viewTop(total, rows int) int {
	if total <= rows {
		return 0
	}
	top := m.logAt
	if m.logStick {
		top = total - rows
	}
	return max(0, min(top, total-rows))
}

func (m model) shownTop(lines []vline, rows int) int {
	total := len(lines)
	if total <= rows {
		return 0
	}
	if m.logStick {
		return total - rows
	}
	if m.vs.logHold && (m.stage == "logs" || m.stage == "mlogs") {
		if at := findVisual(lines, m.vs.logLine, m.vs.logSub); at >= 0 {
			return max(0, min(at, total-rows))
		}
	}
	return max(0, min(m.logAt, total-rows))
}

func (m *model) clearLogHold() {
	m.vs.logHold = false
	m.vs.logLine = 0
	m.vs.logSub = 0
}

func (m *model) pinLog(vis []vline, top int) {
	if m.stage != "logs" && m.stage != "mlogs" {
		return
	}
	line, sub := anchorAt(vis, top)
	m.vs.logHold = true
	m.vs.logLine = line
	m.vs.logSub = sub
}

func anchorAt(vis []vline, top int) (int, int) {
	if len(vis) == 0 {
		return 0, 0
	}
	if top < 0 {
		top = 0
	}
	if top >= len(vis) {
		top = len(vis) - 1
	}
	sub := 0
	i := top
	for i > 0 && vis[i].num < 0 {
		i--
		sub++
	}
	if vis[i].num >= 0 {
		return vis[i].num, sub
	}
	return 0, 0
}

func findVisual(vis []vline, line, sub int) int {
	if line < 0 {
		return 0
	}
	for i, row := range vis {
		if row.num != line {
			continue
		}
		at := i + sub
		for k := i + 1; k < len(vis) && k <= at; k++ {
			if vis[k].num >= 0 {
				return k - 1
			}
		}
		if at >= len(vis) {
			return len(vis) - 1
		}
		return at
	}
	return -1
}

func rewindHold(vs *viewState, dropped int, stick bool) {
	if vs == nil || dropped <= 0 || stick || !vs.logHold {
		return
	}
	vs.logLine -= dropped
	if vs.logLine < 0 {
		vs.logLine = 0
		vs.logSub = 0
	}
}

func (m model) logsBody(innerW int) string {
	if m.load.active() {
		return m.loadingBlock(innerW)
	}
	lines := m.visualLines(innerW)
	rows := m.textRows()
	top := m.shownTop(lines, rows)
	end := min(len(lines), top+rows)
	shown := lines[top:end]
	logical := len(m.textLines())
	wrapNote := m.wrapNote()
	var b strings.Builder
	if m.stage == "mlogs" {
		b.WriteString(dimStyle.Render(m.mlogHeader(lineSpan(shown, logical))) + "\n")
		gutter := logGutter(logical)
		room := logRoom(innerW, logical)
		for _, l := range shown {
			if l.num == logMarkNum {
				b.WriteString(logMarkBar(innerW) + "\n")
				continue
			}
			num := strings.Repeat(" ", gutter)
			if l.num >= 0 {
				num = fmt.Sprintf("%*d", gutter, l.num+1)
			}
			b.WriteString(dimStyle.Render(num) + " " + clipLine(l.text, room) + "\n")
		}
		return b.String()
	}
	if m.stage == "yaml" || m.stage == "view" || m.stage == "kout" || m.stage == "describe" || m.stage == "pulse" || m.stage == "hdiff" {
		mark := m.scrollKeys()
		if len(lines) > rows {
			mark = fmt.Sprintf("%s  %d-%d", m.scrollKeys(), top+1, top+len(shown))
		}
		mark += "    " + wrapNote
		if m.stage == "kout" && m.ran.set {
			b.WriteString(m.ran.header() + dimStyle.Render("    "+mark) + "\n")
		} else {
			b.WriteString(dimStyle.Render(mark) + "\n")
		}
		for _, l := range shown {
			b.WriteString(clipLine(l.text, innerW) + "\n")
		}
		return b.String()
	}
	mode := "follow"
	if !m.logStick {
		mode = "paused"
	}
	if m.vs.logPrev {
		mode = "previous"
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("%s  %s    %s    %s    %s copy    %s mark    %s vim    %s previous", mode, lineSpan(shown, logical), m.scrollKeys(), wrapNote, m.cfg.Keys.CopyMode.Short(), m.cfg.Keys.LogMark.Short(), m.cfg.Keys.Vim.Short(), m.cfg.Keys.Previous.Short())) + "\n")
	gutter := logGutter(logical)
	room := logRoom(innerW, logical)
	for _, l := range shown {
		if l.num == logMarkNum {
			b.WriteString(logMarkBar(innerW) + "\n")
			continue
		}
		num := strings.Repeat(" ", gutter)
		if l.num >= 0 {
			num = fmt.Sprintf("%*d", gutter, l.num+1)
		}
		b.WriteString(dimStyle.Render(num) + " " + clipLine(l.text, room) + "\n")
	}
	return b.String()
}

func (m model) fetchLogs() tea.Cmd {
	return fetchLogs(m.k, m.cfg.Logs.Tail, m.logID, m.logPod, m.namespace, m.kubeconfig, m.context, m.logSince, m.vs.logOpts())
}

func (v viewState) logOpts() kube.LogOpts {
	return kube.LogOpts{Container: v.logCont, Previous: v.logPrev}
}

func logKey(podRef string, opt kube.LogOpts) string {
	return fmt.Sprintf("%s|%s|%v", podRef, opt.Container, opt.Previous)
}

func fetchLogs(k kube.Client, tail int, id, podRef, namespace, kubeconfig, context string, since time.Time, opt kube.LogOpts) tea.Cmd {
	ns, name := kube.SplitRef(podRef, namespace)
	partial := !since.IsZero()
	key := logKey(podRef, opt)
	return func() tea.Msg {
		text, err := k.Logs(name, ns, kubeconfig, context, since, tail, opt)
		if err != nil {
			return logsMsg{id: id, pod: key, err: err, partial: partial}
		}
		return logsMsg{id: id, pod: key, text: text, partial: partial}
	}
}

func (m *model) applyLogs(msg logsMsg) {
	apply := func(text *string, marks *[]int, busy *bool, since *time.Time, at *int, stick *bool, vs *viewState, key, stage, id string) {
		*busy = false
		if msg.pod != key || stage != "logs" {
			return
		}
		if msg.err != nil {
			if !msg.partial || *text == "" {
				*text = m.errLine(msg.err)
			}
			return
		}
		if msg.partial {
			merged := kube.AppendLogs(*text, msg.text)
			lines := strings.Split(merged, "\n")
			anchor := 0
			if !*stick {
				anchor = m.paneAnchor(id, stage, *text, *at, *vs)
			}
			dropped := ringDropN(len(lines), m.cfg.Logs.MaxLines, anchor, *stick)
			if dropped > 0 && !*stick {
				*at = max(0, *at-m.paneShift(id, stage, *text, *at, *vs, dropped))
				rewindHold(vs, dropped, false)
			}
			trimmed := merged
			if dropped > 0 {
				trimmed = strings.Join(lines[dropped:], "\n")
			}
			*text = trimmed
			if dropped > 0 && marks != nil {
				kept := make([]int, 0, len(*marks))
				for _, n := range *marks {
					if n -= dropped; n >= 0 {
						kept = append(kept, n)
					}
				}
				*marks = kept
			}
		} else if msg.text != "" {
			*text = msg.text
			if marks != nil {
				*marks = nil
			}
		}
		*since = time.Now().Add(-2 * time.Second)
	}
	if m.logID == msg.id {
		m.load = load{}
		apply(&m.logText, &m.vs.logMarks, &m.logBusy, &m.logSince, &m.logAt, &m.logStick, &m.vs, logKey(m.logPod, m.vs.logOpts()), m.stage, m.logID)
		return
	}
	for i := range m.panes {
		if m.panes[i].logID != msg.id {
			continue
		}
		m.panes[i].load = load{}
		p := m.panes[i]
		apply(&m.panes[i].logText, &m.panes[i].vs.logMarks, &m.panes[i].logBusy, &m.panes[i].logSince, &m.panes[i].logAt, &m.panes[i].logStick, &m.panes[i].vs, logKey(p.logPod, p.vs.logOpts()), p.stage, p.logID)
		return
	}
}

func (m model) vimView() tea.Cmd {
	id := m.logID
	if m.stage != "logs" {
		text := m.logText
		return func() tea.Msg {
			f, err := os.CreateTemp("", "k2-*.txt")
			if err != nil {
				return vimReadyMsg{pane: id, err: err}
			}
			if _, err := f.WriteString(text); err != nil {
				f.Close()
				os.Remove(f.Name())
				return vimReadyMsg{pane: id, err: err}
			}
			f.Close()
			return vimReadyMsg{pane: id, path: f.Name()}
		}
	}
	ns, name := kube.SplitRef(m.logPod, m.namespace)
	kc, ctx, opt := m.kubeconfig, m.context, m.vs.logOpts()
	return func() tea.Msg {
		out, err := m.k.AllLogs(name, ns, kc, ctx, opt)
		if err != nil {
			return vimReadyMsg{pane: id, err: err}
		}
		f, err := os.CreateTemp("", "k2-*.log")
		if err != nil {
			return vimReadyMsg{pane: id, err: err}
		}
		if _, err := f.Write(out); err != nil {
			f.Close()
			os.Remove(f.Name())
			return vimReadyMsg{pane: id, err: err}
		}
		f.Close()
		return vimReadyMsg{pane: id, path: f.Name()}
	}
}

func (m model) editSelected() (tea.Model, tea.Cmd) {
	sel := m.content.SelectedItem()
	if sel == nil {
		return m, nil
	}
	it := sel.(item)
	kind := "pod"
	ns, name := kube.SplitRef(it.id, m.namespace)
	if m.stage == "res" {
		kind = m.kind
		ns, name = kube.SplitRef(it.id, it.desc)
		if ns == "" {
			ns = m.namespace
		}
	}
	args := kube.EditArgs(kind, name, m.kubeconfig, m.context, ns)
	mk := func() *exec.Cmd {
		cmd := exec.Command("kubectl", args...)
		env := os.Environ()
		if _, err := exec.LookPath("vim"); err == nil {
			env = append(env, "KUBE_EDITOR=vim")
		}
		cmd.Env = env
		return cmd
	}
	return m.runEditor("edit", "edit "+kind+"/"+name, name, "", mk, func(err error) tea.Msg {
		return editDoneMsg{what: "edited " + name, err: err}
	})
}

func (m model) deleteSelectedPod() tea.Cmd {
	sel := m.content.SelectedItem()
	if sel == nil {
		return nil
	}
	ns, name := kube.SplitRef(sel.(item).id, m.namespace)
	kc, ctx := m.kubeconfig, m.context
	return func() tea.Msg {
		return editDoneMsg{what: "deleted " + name, err: m.k.DeletePod(kc, ctx, ns, name)}
	}
}

func (m model) decodeSelected() tea.Cmd {
	sel := m.content.SelectedItem()
	if sel == nil {
		return nil
	}
	it := sel.(item)
	ns, name := it.desc, it.id
	if ns == "" {
		ns = m.namespace
	}
	kc, ctx, pane := m.kubeconfig, m.context, m.logID
	return func() tea.Msg {
		text, err := m.k.Secret(kc, ctx, ns, name)
		if err != nil {
			return viewMsg{id: pane, err: err}
		}
		return viewMsg{id: pane, text: text}
	}
}

func (m *model) applyView(msg viewMsg) {
	text := msg.text
	if msg.err != nil {

		m.noteErr(msg.err.Error())
		text = strings.TrimSpace(msg.err.Error())
		if !isAlert(text) {
			text = "error: " + text
		}
	}
	if m.logID == msg.id {
		m.load = load{}
		m.stage = "view"
		m.logText = text
		m.status = "decoded secret"
		return
	}
	for i := range m.panes {
		if m.panes[i].logID != msg.id {
			continue
		}
		m.panes[i].load = load{}
		m.panes[i].stage = "view"
		m.panes[i].logText = text
		m.panes[i].status = "decoded secret"
		return
	}
}

const logMarkNum = -2

func insertLogMarks(rows []vline, marks []int) []vline {
	want := map[int]bool{}
	for _, n := range marks {
		if n >= 0 {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return rows
	}
	out := make([]vline, 0, len(rows)+len(want))
	cur := -1
	flush := func() {
		if cur >= 0 && want[cur] {
			out = append(out, vline{num: logMarkNum})
		}
	}
	for _, row := range rows {
		if row.num >= 0 && row.num != cur {
			flush()
			cur = row.num
		}
		out = append(out, row)
	}
	flush()
	return out
}

func logMarkBar(width int) string {
	const label = " mark "
	width = max(width, 1)
	if width <= len(label) {
		return warnStyle.Render(clipLine(label, width))
	}
	left := (width - len(label)) / 2
	bar := strings.Repeat("─", left) + label + strings.Repeat("─", width-left-len(label))
	return warnStyle.Render(bar)
}

func (m *model) toggleLogMark() {
	if m.stage == "mlogs" {
		m.toggleMlogMark()
		return
	}
	if strings.TrimSpace(m.logText) == "" {
		m.status = "nothing to mark"
		return
	}
	idx := len(strings.Split(strings.TrimRight(m.logText, "\n"), "\n")) - 1
	if !m.logStick {
		vis := m.visualLines(m.viewerWidth())
		end := min(len(vis), m.viewTop(len(vis), m.textRows())+m.textRows()) - 1
		for end >= 0 && vis[end].num < 0 {
			end--
		}
		if end >= 0 {
			idx = vis[end].num
		}
	}
	next := make([]int, 0, len(m.vs.logMarks)+1)
	found := false
	for _, n := range m.vs.logMarks {
		if n == idx {
			found = true
			continue
		}
		next = append(next, n)
	}
	if found {
		m.vs.logMarks = next
		m.status = "mark cleared"
		return
	}
	m.vs.logMarks = append(next, idx)
	m.status = "mark"
}

func (m *model) toggleMlogMark() {
	v := m.vs.ml
	if v == nil || len(v.lines) == 0 {
		m.status = "nothing to mark"
		return
	}
	idx := -1
	if m.logStick {
		idx = v.lastShown()
	} else {
		vis := m.visualLines(m.viewerWidth())
		end := min(len(vis), m.viewTop(len(vis), m.textRows())+m.textRows()) - 1
		for end >= 0 && vis[end].num < 0 {
			end--
		}
		if end >= 0 {
			idx = v.bufferIndex(vis[end].num)
		}
	}
	if idx < 0 {
		m.status = "nothing to mark"
		return
	}
	marks := make([]int, 0, len(v.marks)+1)
	found := false
	for _, n := range v.marks {
		if n == idx {
			found = true
			continue
		}
		marks = append(marks, n)
	}
	next := *v
	next.marks = marks
	m.vs.ml = &next
	if found {
		m.status = "mark cleared"
		return
	}
	next.marks = append(marks, idx)
	m.vs.ml = &next
	m.status = "mark"
}

func lineSpan(shown []vline, total int) string {
	first, last := -1, -1
	for _, l := range shown {
		if l.num < 0 {
			continue
		}
		if first < 0 {
			first = l.num
		}
		last = l.num
	}
	if first < 0 || total <= 0 {
		return fmt.Sprintf("%d lines", total)
	}
	return fmt.Sprintf("%d-%d/%d", first+1, last+1, total)
}

// ringDropN is how many leading lines to drop so the buffer stays near limit.
// While paused, lines at and below the anchor stay, so the viewport does not jump.
// Past twice the limit the anchor gives way, so a paused view cannot grow without bound.
func ringDropN(n, limit, anchor int, stick bool) int {
	if limit <= 0 || n <= limit {
		return 0
	}
	excess := n - limit
	if stick {
		return excess
	}
	drop := min(excess, max(anchor, 0))
	hard := limit * 2
	if n-drop > hard {
		drop = n - hard
	}
	if drop < 0 {
		drop = 0
	}
	if drop >= n && n > 0 {
		drop = n - 1
	}
	return drop
}

func (m model) logicalAnchor() int {
	vis := m.visualLines(m.viewerWidth())
	if len(vis) == 0 {
		return 0
	}
	top := m.viewTop(len(vis), m.textRows())
	if top >= len(vis) {
		top = len(vis) - 1
	}
	for top > 0 && vis[top].num < 0 {
		top--
	}
	if vis[top].num >= 0 {
		return vis[top].num
	}
	return 0
}

func (m model) rawAnchor() int {
	if m.vs.logHold && (m.stage == "logs" || m.stage == "mlogs") {
		n := m.vs.logLine
		if m.stage == "mlogs" && m.vs.ml != nil && m.vs.ml.filter != "" {
			if i := m.vs.ml.bufferIndex(n); i >= 0 {
				return i
			}
		}
		return max(n, 0)
	}
	n := m.logicalAnchor()
	if m.stage == "mlogs" && m.vs.ml != nil && m.vs.ml.filter != "" {
		if i := m.vs.ml.bufferIndex(n); i >= 0 {
			return i
		}
	}
	return n
}

func (m model) droppedVisual(droppedLogical int) int {
	if droppedLogical <= 0 {
		return 0
	}
	vis := m.visualLines(m.viewerWidth())
	shift := 0
	for _, row := range vis {
		if row.num >= droppedLogical {
			break
		}
		shift++
	}
	return shift
}

func (m model) withLog(id, stage, text string, at int, vs viewState) model {
	alt := m
	alt.logID = id
	alt.stage = stage
	alt.logText = text
	alt.logStick = false
	alt.logAt = at
	alt.vs = vs
	return alt
}

func (m model) paneAnchor(id, stage, text string, at int, vs viewState) int {
	return m.withLog(id, stage, text, at, vs).rawAnchor()
}

func (m model) paneShift(id, stage, text string, at int, vs viewState, dropped int) int {
	return m.withLog(id, stage, text, at, vs).droppedVisual(dropped)
}

func trimLogDrop(s string, max int) (string, int) {
	if max <= 0 {
		return s, 0
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s, 0
	}
	drop := len(lines) - max
	return strings.Join(lines[drop:], "\n"), drop
}

package ui

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"
	"sync"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mlogView struct {
	q       kube.LogQuery
	title   string
	from    string
	lines   []kube.LogLine
	cur     kube.LogCursor
	pods    []string
	filter  string
	re      *regexp.Regexp
	dropped int
	marks   []int
	note    string
	polls   int
	colors  map[string]color
}

type color = lipgloss.Color

type mlogMsg struct {
	pane  string
	key   string
	pods  []string
	lines []kube.LogLine
	note  string
	err   error
}

func podsRegex(pods []string) *regexp.Regexp {
	quoted := make([]string, len(pods))
	for i, p := range pods {
		quoted[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile("^(" + strings.Join(quoted, "|") + ")$")
}

func (m model) openMultiLogs(q kube.LogQuery, title, from string) (tea.Model, tea.Cmd) {
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	m.vs.ml = &mlogView{q: q, title: title, from: from, cur: kube.LogCursor{}, colors: map[string]color{}}
	m.logText = ""
	m.logStick = true
	m.logAt = 0
	m.stage = "mlogs"
	m.status = "logs of " + title + " · " + m.cfg.Keys.LogFilter.Label() + " filter"
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	m.rowBusy["ml"+m.logID] = true
	return m, tea.Batch(m.startLoad("reading logs", false), m.mlogPoll())
}

func (m *model) resumeStern() []tea.Cmd {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	var cmds []tea.Cmd
	start := func(id, kc, ctx, ns string, v *mlogView) {
		if v == nil || kc == "" || ctx == "" || m.rowBusy["ml"+id] {
			return
		}
		m.rowBusy["ml"+id] = true
		cmds = append(cmds, mlogCmd(m.k, id, v.q, v.cur, kc, ctx, ns, m.cfg.Logs.Tail, m.cfg.Logs.MultiMaxPods))
	}
	if m.stage == "mlogs" {
		start(m.logID, m.kubeconfig, m.context, m.namespace, m.vs.ml)
	}
	for i := range m.panes {
		if i == m.focus {
			continue
		}
		p := m.panes[i]
		if p.stage == "mlogs" {
			start(p.logID, p.kubeconfig, p.context, p.namespace, p.vs.ml)
		}
	}
	return cmds
}

func logQueryArgs(q kube.LogQuery) string {
	var parts []string
	switch {
	case q.Selector != "":
		parts = append(parts, q.Selector)
	case q.Regex != nil:
		parts = append(parts, q.Regex.String())
	default:
		return ""
	}
	if q.Container != "" {
		parts = append(parts, "-c", q.Container)
	}
	if q.All {
		parts = append(parts, "--all=true")
	} else {
		parts = append(parts, "--all=false")
	}
	if q.Since > 0 {
		parts = append(parts, "--since="+q.Since.String())
	}
	if q.Tail >= 0 {
		parts = append(parts, fmt.Sprintf("--tail=%d", q.Tail))
	}
	return strings.Join(parts, " ")
}

func (m model) mlogPoll() tea.Cmd {
	v := m.vs.ml
	if v == nil {
		return nil
	}
	return mlogCmd(m.k, m.logID, v.q, v.cur, m.kubeconfig, m.context, m.namespace, m.cfg.Logs.Tail, m.cfg.Logs.MultiMaxPods)
}

func mlogCmd(k kube.Client, pane string, q kube.LogQuery, cur kube.LogCursor, kc, ctx, ns string, tail, maxPods int) tea.Cmd {
	key := mlogKey(ns, q)
	return func() tea.Msg {
		objs, err := k.ObjectsWhere(kc, ctx, "pods", ns, q.Selector)
		if err != nil {
			return mlogMsg{pane: pane, key: key, err: err}
		}
		pods := kube.MatchPods(objs, q)

		sort.SliceStable(pods, func(i, j int) bool {
			return pods[i].Metadata.CreationTimestamp.After(pods[j].Metadata.CreationTimestamp)
		})
		note := ""
		if len(pods) > maxPods {
			note = fmt.Sprintf("%d pods match, reading the newest %d (logs.multi_max_pods)", len(pods), maxPods)
			pods = pods[:maxPods]
		}
		if q.Tail > 0 {
			tail = q.Tail
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		var lines []kube.LogLine
		var failed []string
		slots := make(chan struct{}, 6)
		names := make([]string, len(pods))
		for i, p := range pods {
			names[i] = p.Metadata.Name
			wg.Add(1)
			go func(p kube.Object) {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()

				src := p.Metadata.Name
				if ns == "" && p.Metadata.Namespace != "" {
					src = p.Metadata.Namespace + "/" + src
				}
				got, err := k.MultiLogs(p.Metadata.Name, or(p.Metadata.Namespace, ns), kc, ctx, q, cur.Since(src), tail)
				if ns == "" {
					for i := range got {
						got[i].Namespace = p.Metadata.Namespace
					}
				}
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed = append(failed, p.Metadata.Name)
					return
				}
				lines = append(lines, got...)
			}(p)
		}
		wg.Wait()
		if len(failed) > 0 {
			sort.Strings(failed)
			note = strings.TrimPrefix(note+" · could not read "+strings.Join(failed, ", "), " · ")
		}
		kube.SortLines(lines)
		sort.Strings(names)
		return mlogMsg{pane: pane, key: key, pods: names, lines: lines, note: note}
	}
}

func mlogKey(ns string, q kube.LogQuery) string { return ns + "|" + q.String() }

func (m *model) applyMlog(msg mlogMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		delete(m.rowBusy, "ml"+msg.pane)
		v := m.vs.ml
		if m.stage != "mlogs" || v == nil || mlogKey(m.namespace, v.q) != msg.key {
			return
		}
		first := v.polls == 0
		if m.load.active() {
			m.load = load{}
		}
		next := *v
		next.polls++
		if msg.err != nil {
			next.note = m.errLine(msg.err)
			m.vs.ml = &next
			m.logText = next.render()
			if first {
				cmd = m.notify(toneErr, "could not list the pods")
			}
			return
		}
		fresh, cur := v.cur.Fresh(msg.lines)
		next.cur, next.pods, next.note = cur, msg.pods, msg.note

		next.lines = append(v.lines[:len(v.lines):len(v.lines)], fresh...)
		anchor := 0
		if !m.logStick {
			anchor = m.rawAnchor()
		}
		if over := ringDropN(len(next.lines), m.cfg.Logs.MultiMaxLines, anchor, m.logStick); over > 0 {
			shown := v.shownPrefix(over)
			if !m.logStick {
				m.holdLogAt(shown)
				rewindHold(&m.vs, shown, false)
			}
			next.lines = next.lines[over:]
			next.dropped += over
			kept := make([]int, 0, len(v.marks))
			for _, n := range v.marks {
				if n -= over; n >= 0 {
					kept = append(kept, n)
				}
			}
			next.marks = kept
		}
		next.colors = v.colors
		for _, l := range fresh {
			if _, ok := next.colors[l.Source()]; !ok {
				if len(next.colors) == len(v.colors) {
					next.colors = copyColors(v.colors)
				}
				next.colors[l.Source()] = sourceColor(l.Source())
			}
		}
		m.vs.ml = &next
		m.logText = next.render()
	})
	return cmd
}

func copyColors(in map[string]color) map[string]color {
	out := make(map[string]color, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	return out
}

var mlogPalette = []color{"39", "208", "170", "42", "214", "75", "203", "141", "79", "221", "33", "167"}

func sourceColor(src string) color {
	h := fnv.New32a()
	h.Write([]byte(src))
	return mlogPalette[h.Sum32()%uint32(len(mlogPalette))]
}

func (v *mlogView) render() string {
	var b strings.Builder
	for _, l := range v.lines {
		line := l.Source() + " " + l.Text
		if v.filter != "" && !v.matches(line) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func (v *mlogView) matches(line string) bool {
	if v.re != nil {
		return v.re.MatchString(line)
	}
	return strings.Contains(line, v.filter)
}

func (v *mlogView) renderedMarks() []int {
	if v == nil || len(v.marks) == 0 {
		return nil
	}
	want := map[int]bool{}
	for _, n := range v.marks {
		if n >= 0 && n < len(v.lines) {
			want[n] = true
		}
	}
	var out []int
	shown := 0
	for i, l := range v.lines {
		if v.filter != "" && !v.matches(l.Source()+" "+l.Text) {
			continue
		}
		if want[i] {
			out = append(out, shown)
		}
		shown++
	}
	return out
}

func (v *mlogView) shownPrefix(n int) int {
	if v == nil || n <= 0 {
		return 0
	}
	if n > len(v.lines) {
		n = len(v.lines)
	}
	if v.filter == "" {
		return n
	}
	c := 0
	for i := 0; i < n; i++ {
		l := v.lines[i]
		if v.matches(l.Source() + " " + l.Text) {
			c++
		}
	}
	return c
}

func (m *model) holdLogAt(droppedLogical int) {
	if m.logStick || droppedLogical <= 0 {
		return
	}
	m.logAt = max(0, m.logAt-m.droppedVisual(droppedLogical))
}

func (v *mlogView) lastShown() int {
	last := -1
	for i, l := range v.lines {
		if v.filter != "" && !v.matches(l.Source()+" "+l.Text) {
			continue
		}
		last = i
	}
	return last
}

func (v *mlogView) bufferIndex(rendered int) int {
	shown := 0
	for i, l := range v.lines {
		if v.filter != "" && !v.matches(l.Source()+" "+l.Text) {
			continue
		}
		if shown == rendered {
			return i
		}
		shown++
	}
	return -1
}

func (m model) colorMlog(line string) string {
	v := m.vs.ml
	if v == nil {
		return line
	}
	src, rest, ok := strings.Cut(line, " ")
	if !ok {
		src, rest = line, ""
	}
	c, known := v.colors[src]
	if !known {
		return line
	}
	return m.coloredSource(c, src) + " " + rest
}

func (m model) mlogStyle(c color) lipgloss.Style {
	if m.mlogStyles == nil {
		return lipgloss.NewStyle().Foreground(c)
	}
	if s, ok := m.mlogStyles[c]; ok {
		return s
	}
	s := lipgloss.NewStyle().Foreground(c)
	m.mlogStyles[c] = s
	return s
}

func (m model) coloredSource(c color, src string) string {
	if m.mlogPaint != nil {
		if s, ok := m.mlogPaint[src]; ok {
			return s
		}
	}
	s := m.mlogStyle(c).Render(src)
	if m.mlogPaint != nil {
		m.mlogPaint[src] = s
	}
	return s
}

// paintMlogRow colors the pod prefix on the first visual row. The line is
// wrapped plain, so continuation rows stay uncolored and the fast path applies.
func (m model) paintMlogRow(line string, rows []string) {
	if len(rows) == 0 || m.vs.ml == nil {
		return
	}
	src, _, ok := strings.Cut(line, " ")
	if !ok {
		src = line
	}
	if src == "" {
		return
	}
	c, known := m.vs.ml.colors[src]
	if !known {
		return
	}
	head := rows[0]
	if strings.HasPrefix(head, src) {
		rows[0] = m.coloredSource(c, src) + head[len(src):]
		return
	}
	if strings.HasPrefix(src, head) && head != "" {
		rows[0] = m.mlogStyle(c).Render(head)
	}
}

func (m model) mlogHeader(span string) string {
	v := m.vs.ml
	if v == nil {
		return ""
	}
	mode := "follow"
	if !m.logStick {
		mode = "paused"
	}
	parts := []string{mode, plural(len(v.pods), "pod", "pods"), span}
	if v.dropped > 0 {
		parts = append(parts, fmt.Sprintf("%d dropped", v.dropped))
	}
	if v.filter != "" {
		parts = append(parts, m.cfg.Keys.LogFilter.Label()+v.filter)
	}
	parts = append(parts, m.cfg.Keys.CopyMode.Short()+" copy", m.cfg.Keys.LogMark.Short()+" mark", m.wrapNote())
	if v.note != "" {
		parts = append(parts, v.note)
	}
	return strings.Join(parts, "    ")
}

func (m model) openLogFilter() (tea.Model, tea.Cmd) {
	v := m.vs.ml
	if v == nil {
		return m, nil
	}
	m.dlg = m.newDialog("logfilter", "⌕ filter logs", "", m.namespace, "").
		withField(v.filter, "text or regex", "lines containing it (a regex when it is one); empty shows all")
	return m, nil
}

func (m model) submitLogFilter() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.dlg.input.Value())
	m.dlg = nil
	v := m.vs.ml
	if v == nil {
		return m, nil
	}
	next := *v
	next.filter, next.re = text, nil
	if text != "" {
		if re, err := regexp.Compile(text); err == nil {
			next.re = re
		}
	}
	m.vs.ml = &next
	m.logText = next.render()
	m.logStick, m.logAt = true, 0
	if text == "" {
		m.status = "filter cleared"
	} else {
		m.status = "filter " + text
	}
	return m, nil
}

func (m model) multiLogsSelected() (tea.Model, tea.Cmd) {
	kind, ns, name, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	q := kube.LogQuery{All: m.cfg.Logs.MultiAllContainers}
	if m.stage == "workpods" || kube.BaseKind(kind) == "pods" {

		q.Regex = siblingsRegex(name)
		return m.openMultiLogs(q, "pods like "+name, m.stage)
	}
	switch kube.BaseKind(kind) {
	case "deployments", "statefulsets", "daemonsets", "replicasets", "jobs", "services":
	default:
		m.status = "multi-pod logs need a workload, service or pod (not " + kube.BaseKind(kind) + ")"
		return m, m.notify(toneWarn, "no pods for "+kube.BaseKind(kind))
	}
	k, kc, ctx := m.k, m.kubeconfig, m.context
	m.status = "reading the selector of " + name + "…"
	pane, from := m.logID, m.stage
	title := kube.BaseKind(kind) + "/" + name
	return m, func() tea.Msg {
		o, err := k.Object(kc, ctx, kind, ns, name)
		if err != nil {
			return selectorMsg{pane: pane, err: err}
		}
		sel := kube.SelectorOf(o)
		if sel == "" {
			return selectorMsg{pane: pane, err: fmt.Errorf("%s has no pod selector", title)}
		}
		q.Selector = sel
		return selectorMsg{pane: pane, q: q, title: title, from: from}
	}
}

type selectorMsg struct {
	pane, title, from string
	q                 kube.LogQuery
	err               error
}

func (m model) applySelector(msg selectorMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var out tea.Model = m
	m.withPane(msg.pane, func() {
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "no pod selector")
			out = m
			return
		}
		next, c := m.openMultiLogs(msg.q, msg.title, msg.from)
		m = next.(model)
		out, cmd = m, c
	})
	if mm, ok := out.(model); ok {
		return mm, cmd
	}
	return m, cmd
}

func siblingsRegex(name string) *regexp.Regexp {
	parts := strings.Split(name, "-")
	switch {
	case len(parts) >= 3 && len(parts[len(parts)-1]) == 5:

		return regexp.MustCompile("^" + regexp.QuoteMeta(strings.Join(parts[:len(parts)-2], "-")) + "-[a-z0-9]+-[a-z0-9]{5}$")
	case len(parts) >= 2 && isDigits(parts[len(parts)-1]):

		return regexp.MustCompile("^" + regexp.QuoteMeta(strings.Join(parts[:len(parts)-1], "-")) + "-[0-9]+$")
	case len(parts) >= 2 && len(parts[len(parts)-1]) == 5:

		return regexp.MustCompile("^" + regexp.QuoteMeta(strings.Join(parts[:len(parts)-1], "-")) + "-[a-z0-9]{5}$")
	}
	return regexp.MustCompile("^" + regexp.QuoteMeta(name) + "$")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (m *model) mlogTick() []tea.Cmd {
	if m.beat%max(1, m.cfg.Logs.MultiEvery) != 0 {
		return nil
	}
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	shown := map[string]bool{}
	for _, id := range m.tabIDs() {
		shown[id] = true
	}
	var cmds []tea.Cmd
	poll := func(id, stage string, v *mlogView, kc, ctx, ns string) {
		if stage != "mlogs" || v == nil || !shown[id] || m.rowBusy["ml"+id] {
			return
		}
		m.rowBusy["ml"+id] = true
		cmds = append(cmds, mlogCmd(m.k, id, v.q, v.cur, kc, ctx, ns, m.cfg.Logs.Tail, m.cfg.Logs.MultiMaxPods))
	}
	poll(m.logID, m.stage, m.vs.ml, m.kubeconfig, m.context, m.namespace)
	for i := range m.panes {
		if i != m.focus {
			p := m.panes[i]
			poll(p.logID, p.stage, p.vs.ml, p.kubeconfig, p.context, p.namespace)
		}
	}
	return cmds
}

func (m model) runLogsCommand(args string) (tea.Model, tea.Cmd) {
	q, err := kube.ParseLogQuery(args, m.cfg.Logs.MultiAllContainers)
	if err != nil {
		m.status = "logs: " + err.Error()
		return m, m.notify(toneWarn, "logs: "+err.Error())
	}
	return m.openMultiLogs(q, q.String(), m.stage)
}

func (m model) mlogTitle() string {
	if m.vs.ml == nil {
		return "logs"
	}
	return "logs · " + m.vs.ml.title
}

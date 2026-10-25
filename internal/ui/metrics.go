package ui

import (
	"strconv"
	"strings"
	"time"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type podMetrics struct {
	usage   map[string]kube.Usage
	err     string
	at      int
	done    bool
	loading bool
}

type metricsMsg struct {
	key   string
	usage map[string]kube.Usage
	err   error
}

func metricsKey(kc, ctx, ns string) string { return kc + "\x00" + ctx + "\x00" + ns }

func (m *model) metricsCmds() []tea.Cmd {
	if !m.cfg.UI.PodMetrics {
		return nil
	}
	shown := map[string]bool{}
	for _, id := range m.tabIDs() {
		shown[id] = true
	}
	var cmds []tea.Cmd
	try := func(id, stage, kind, kc, ctx, ns string) {
		if !shown[id] || !showsPods(stage, kind) || kc == "" || ctx == "" {
			return
		}
		key := metricsKey(kc, ctx, ns)
		cur := m.metrics[key]
		if cur.loading || (cur.done && m.beat-cur.at < max(5, m.cfg.UI.MetricsEvery)) {
			return
		}
		cur.loading = true
		m.setMetrics(key, cur)
		k := m.k
		cmds = append(cmds, func() tea.Msg {
			ch := make(chan metricsMsg, 1)
			go func() {
				usage, err := k.TopPods(kc, ctx, ns)
				ch <- metricsMsg{key: key, usage: usage, err: err}
			}()
			select {
			case msg := <-ch:
				return msg
			case <-time.After(kube.InfoTimeout + infoGrace):
				return metricsMsg{key: key, err: errTimeout}
			}
		})
	}
	try(m.logID, m.stage, m.kind, m.kubeconfig, m.context, m.namespace)
	for i, p := range m.panes {
		if i != m.focus {
			try(p.logID, p.stage, p.kind, p.kubeconfig, p.context, p.namespace)
		}
	}
	return cmds
}

var errTimeout = timeoutError{}

type timeoutError struct{}

func (timeoutError) Error() string { return "timed out" }

func showsPods(stage, kind string) bool {
	return stage == "workpods" || (stage == "res" && kube.BaseKind(kind) == "pods")
}

func (m *model) setMetrics(key string, pm podMetrics) {
	next := make(map[string]podMetrics, len(m.metrics)+1)
	for k, v := range m.metrics {
		next[k] = v
	}
	next[key] = pm
	m.metrics = next
}

func (m *model) applyMetrics(msg metricsMsg) {
	pm := podMetrics{usage: msg.usage, at: m.beat, done: true}
	if msg.err != nil {
		pm.err = shell.OneLine(msg.err.Error())
	}
	m.setMetrics(msg.key, pm)
	parts := strings.SplitN(msg.key, "\x00", 3)
	if pm.err != "" && len(parts) == 3 {
		ctxKey := parts[0] + "\x00" + parts[1]
		if !m.metNoted[ctxKey] {
			next := map[string]bool{ctxKey: true}
			for k, v := range m.metNoted {
				next[k] = v
			}
			m.metNoted = next
			if parts[1] == m.context {
				m.status = "pod metrics N/A on " + parts[1] + " (" + metricsReason(pm.err) + ")"
			}
		}
	}
	merge := func(lst *list.Model, vs viewState, id, stage, kind, kc, ctx, ns string) {
		if !showsPods(stage, kind) || metricsKey(kc, ctx, ns) != msg.key || lst.FilterState() != list.Unfiltered || len(lst.Items()) == 0 {
			return
		}
		header, items := m.withMetrics(lst.Title, lst.Items(), kc, ctx, ns, m.paneTableWidth(id))
		setRows(lst, header, items, vs, "")
	}
	merge(&m.content, m.vs, m.logID, m.stage, m.kind, m.kubeconfig, m.context, m.namespace)
	for i := range m.panes {
		if i != m.focus {
			p := &m.panes[i]
			merge(&p.content, p.vs, p.logID, p.stage, p.kind, p.kubeconfig, p.context, p.namespace)
		}
	}
}

func metricsReason(err string) string {
	l := strings.ToLower(err)
	switch {
	case strings.Contains(l, "forbidden"):
		return "forbidden"
	case strings.Contains(l, "metrics api not available"), strings.Contains(l, "the server could not find the requested resource"),
		strings.Contains(l, "metrics.k8s.io"):
		return "no metrics API"
	case strings.Contains(l, "timed out"), strings.Contains(l, "timeout"), strings.Contains(l, "deadline"):
		return "timed out"
	}
	return "kubectl top failed"
}

func (m model) withMetrics(header string, items []list.Item, kc, ctx, ns string, width int) (string, []list.Item) {
	if !m.cfg.UI.PodMetrics || len(items) == 0 {
		return header, items
	}
	first, ok := items[0].(item)
	if !ok || !isPodHeads(first.heads) {
		return header, items
	}
	pm := m.metrics[metricsKey(kc, ctx, ns)]
	out := make([]list.Item, len(items))
	var natural []int
	for i, raw := range items {
		it, ok := raw.(item)
		if !ok || !isPodHeads(it.heads) {
			out[i] = raw
			continue
		}
		it = stripMetrics(it)
		cpu, mem := "…", "…"
		tones := map[int]tone{}
		pos := metricsPos(it.heads)
		switch {
		case pm.done && pm.err != "":
			cpu, mem = "N/A", "N/A"
		case pm.done:
			u, found := pm.usage[podRef(it, ns)]
			if found {
				cpu, mem = u.CPU, u.Mem
			} else {
				cpu, mem = "-", "-"
			}
		}
		for j, v := range []string{cpu, mem} {
			if v == "…" || v == "N/A" || v == "-" {
				tones[pos+j] = toneMuted
			}
		}
		warn, crit := m.cfg.UI.MetricsWarn, m.cfg.UI.MetricsCrit
		var meters map[int]meter
		if m.cfg.UI.MetricsBars {
			meters = map[int]meter{
				pos:     newMeter(cpu, it.extra["cpuLimit"], it.extra["cpuRequest"], kube.ParseCPU, warn, crit),
				pos + 1: newMeter(mem, it.extra["memLimit"], it.extra["memRequest"], kube.ParseMem, warn, crit),
			}
		} else {

			if t, ok := limitTone(cpu, it.extra["cpuLimit"], kube.ParseCPU, warn, crit); ok {
				tones[pos] = t
			}
			if t, ok := limitTone(mem, it.extra["memLimit"], kube.ParseMem, warn, crit); ok {
				tones[pos+1] = t
			}
		}
		it = insertCols(it, pos, []string{"CPU", "MEM"}, []string{cpu, mem})
		it.tones = shiftTones(it.tones, pos, 2)
		for k, v := range tones {
			it.tones[k] = v
		}
		it.meters = meters
		out[i] = it
		natural = widen(natural, it)
	}

	var rows []map[int]meter
	var vals [2][]string
	var idx []int
	for i, raw := range out {
		if it, ok := raw.(item); ok && it.meters != nil {
			rows = append(rows, it.meters)
			idx = append(idx, i)
		}
	}
	if len(rows) > 0 {
		first := out[idx[0]].(item)
		cpuCol := -1
		for c, h := range first.heads {
			if h == "CPU" {
				cpuCol = c
				break
			}
		}
		for j := 0; j < 2 && cpuCol >= 0; j++ {
			vals[j] = nil
			for _, i := range idx {
				it := out[i].(item)
				vals[j] = append(vals[j], it.cells[cpuCol+j])
			}
			if w := lineUp(rows, vals[j], cpuCol+j); w > 0 && cpuCol+j < len(natural) {
				natural[cpuCol+j] = max(natural[cpuCol+j], w)
			}
		}
	}

	var widths []int
	var heads []string
	for i, raw := range out {
		it, ok := raw.(item)
		if !ok || !isPodHeads(it.heads) {
			continue
		}
		if widths == nil {
			widths = fitWidths(natural, it.heads, it.flex, it.ageIdx, width)
			heads = it.heads
		}
		it.natural = append([]int(nil), natural...)
		it.widths = append([]int(nil), widths...)
		it.title, it.plain = paintRow(it.cells, it.widths, it.heads, it.colorCol, it.flex, it.dimCol, it.ageIdx, it.tones, it.meters)
		out[i] = it
	}
	if heads != nil {
		header = headerLine(heads, widths)
	}
	return header, out
}

func isPodHeads(heads []string) bool {
	need := map[string]bool{"READY": false, "STATUS": false, "RESTARTS": false, "NODE": false}
	for _, h := range heads {
		if _, ok := need[h]; ok {
			need[h] = true
		}
	}
	for _, ok := range need {
		if !ok {
			return false
		}
	}
	return true
}

func metricsPos(heads []string) int {
	for i, h := range heads {
		if h == "RESTARTS" {
			return i + 1
		}
	}
	return len(heads)
}

func podRef(it item, ns string) string {
	rns, name := kube.SplitRef(it.id, ns)
	if it.desc != "" {
		rns = it.desc
	}
	return rns + "/" + name
}

func stripMetrics(it item) item {
	it.meters = nil
	for i := len(it.heads) - 1; i >= 0; i-- {
		if isMetricHead(it.heads[i]) {
			it = removeCol(it, i)
		}
	}
	return it
}

func removeCol(it item, i int) item {
	cut := func(s []string) []string {
		if i >= len(s) {
			return s
		}
		return append(append([]string(nil), s[:i]...), s[i+1:]...)
	}
	cutInt := func(s []int) []int {
		if i >= len(s) {
			return s
		}
		return append(append([]int(nil), s[:i]...), s[i+1:]...)
	}
	it.heads, it.cells = cut(it.heads), cut(it.cells)
	it.natural, it.widths = cutInt(it.natural), cutInt(it.widths)
	tones := map[int]tone{}
	for k, v := range it.tones {
		switch {
		case k < i:
			tones[k] = v
		case k > i:
			tones[k-1] = v
		}
	}
	it.tones = tones
	shift := func(c int) int {
		if c > i {
			return c - 1
		}
		return c
	}
	it.flex, it.ageIdx, it.colorCol, it.dimCol = shift(it.flex), shift(it.ageIdx), shift(it.colorCol), shift(it.dimCol)
	return it
}

func insertCols(it item, pos int, heads, cells []string) item {
	ins := func(s, add []string) []string {
		p := min(pos, len(s))
		out := append(append([]string(nil), s[:p]...), add...)
		return append(out, s[p:]...)
	}
	insInt := func(s []int, add int) []int {
		p := min(pos, len(s))
		out := append([]int(nil), s[:p]...)
		for range heads {
			out = append(out, add)
		}
		return append(out, s[p:]...)
	}
	it.heads = ins(it.heads, heads)
	it.cells = ins(it.cells, cells)
	it.natural = insInt(it.natural, 3)
	it.widths = insInt(it.widths, 0)
	n := len(heads)
	shift := func(c int) int {
		if c >= pos {
			return c + n
		}
		return c
	}
	it.flex, it.ageIdx, it.colorCol, it.dimCol = shift(it.flex), shift(it.ageIdx), shift(it.colorCol), shift(it.dimCol)
	return it
}

func shiftTones(tones map[int]tone, pos, n int) map[int]tone {
	out := map[int]tone{}
	for k, v := range tones {
		if k >= pos {
			k += n
		}
		out[k] = v
	}
	return out
}

func widen(natural []int, it item) []int {
	if len(natural) != len(it.heads) {
		natural = make([]int, len(it.heads))
	}
	for i := range natural {
		if i < len(it.natural) {
			natural[i] = max(natural[i], it.natural[i])
		}
		if i < len(it.cells) && isMetricHead(it.heads[i]) {
			natural[i] = max(natural[i], ansi.StringWidth(it.cells[i]), ansi.StringWidth(it.heads[i]))
		}
	}
	return natural
}

func limitTone(used, limit string, parse func(string) (float64, bool), warn, crit int) (tone, bool) {
	if limit == "" {
		return 0, false
	}
	u, ok := parse(used)
	if !ok {
		return 0, false
	}
	l, err := strconv.ParseFloat(limit, 64)
	if err != nil || l <= 0 {
		return 0, false
	}
	if t := thresholdTone(u/l*100, warn, crit); t != toneOK {
		return t, true
	}
	return 0, false
}

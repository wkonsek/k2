package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"k2/internal/kube"
	"k2/internal/kustomize"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func contentDelegate(width int) list.DefaultDelegate {
	if width < 8 {
		width = 8
	}
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	d.Styles.SelectedTitle = selectedItemStyle.Padding(0, 1)
	d.Styles.NormalTitle = itemStyle.Padding(0, 1)
	d.Styles.DimmedTitle = dimStyle.Padding(0, 1)
	return d
}

func isAgeHead(h string) bool { return h == "AGE" || h == "LAST SEEN" }

func ageIndex(headers []string) int {
	for i, h := range headers {
		if isAgeHead(h) {
			return i
		}
	}
	return -1
}

func flexIndex(headers []string) int {
	for _, want := range []string{"NAME", "MESSAGE"} {
		for i, h := range headers {
			if h == want {
				return i
			}
		}
	}
	return 0
}

func balanceWidths(widths []int, heads []string, flex, protect, width int) []int {
	const nameFloor, wideFloor = 24, 12
	n := len(widths)
	if n == 0 {
		return widths
	}
	w := append([]int(nil), widths...)
	if flex < 0 || flex >= n {
		flex = 0
	}
	budget := width - (n-1)*2
	if budget < n {
		budget = n
	}
	sum := 0
	for i := range w {
		if w[i] < 1 {
			w[i] = 1
		}
		sum += w[i]
	}
	headW := func(i int) int {
		if i < len(heads) {
			return max(ansi.StringWidth(heads[i]), 3)
		}
		return 3
	}

	shrinkFlex := func(floor int) bool {
		if w[flex] > floor {
			w[flex]--
			return true
		}
		return false
	}

	shrinkWidest := func(floor int) bool {
		best := -1
		for i := range w {
			if i != flex && i != protect && w[i] > floor && (best < 0 || w[i] > w[best]) {
				best = i
			}
		}
		if best < 0 {
			return false
		}
		w[best]--
		return true
	}

	shrinkRight := func(floor func(int) int) bool {
		for i := n - 1; i >= 0; i-- {
			if i != flex && i != protect && w[i] > floor(i) {
				w[i]--
				return true
			}
		}
		return false
	}
	steps := []func() bool{
		func() bool { return shrinkFlex(nameFloor) },
		func() bool { return shrinkWidest(wideFloor) },
		func() bool { return shrinkFlex(10) },
		func() bool { return shrinkRight(headW) },
		func() bool { return shrinkFlex(4) },
		func() bool { return shrinkRight(func(int) int { return 3 }) },
		func() bool {
			if protect >= 0 && protect < n && w[protect] > 3 {
				w[protect]--
				return true
			}
			return false
		},
		func() bool { return shrinkFlex(1) },
	}
	for _, step := range steps {
		for sum > budget && step() {
			sum--
		}
	}
	return w
}

func reflowList(lst *list.Model, width int) {
	if width < 10 {
		width = 10
	}
	raws := lst.Items()
	if len(raws) == 0 {
		return
	}
	changed := false
	next := make([]list.Item, len(raws))
	var heads []string
	var widths []int
	for i, raw := range raws {
		it, ok := raw.(item)
		if !ok || len(it.cells) == 0 || len(it.widths) == 0 {
			next[i] = raw
			continue
		}
		src := it.natural
		if len(src) != len(it.widths) {
			src = it.widths
		}
		fitted := fitWidths(src, it.heads, it.flex, it.ageIdx, width)
		title, plain := paintRow(it.cells, fitted, it.heads, it.colorCol, it.flex, it.dimCol, it.ageIdx, it.tones, it.meters)
		if title != it.title || plain != it.plain {
			changed = true
		}
		it.widths = fitted
		it.title = title
		it.plain = plain
		next[i] = it
		if len(it.heads) > 0 {
			heads = it.heads
			widths = fitted
		}
	}
	if !changed {
		return
	}
	idx := lst.Index()
	lst.SetItems(next)
	lst.Select(idx)
	if len(heads) > 0 && len(widths) == len(heads) {
		lst.Title = headerLine(heads, widths)
	}
}

func fitCell(t string, w int) string {
	if w < 1 {
		return ""
	}

	t = sanitizeLine(t)
	if ansi.StringWidth(t) > w {
		if w < 2 {
			return ansi.Truncate(t, w, "")
		}
		return ansi.Truncate(t, w, "…")
	}
	return t + strings.Repeat(" ", w-ansi.StringWidth(t))
}

func naturalWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = ansi.StringWidth(h)
	}
	for _, r := range rows {
		for i := 0; i < len(headers) && i < len(r); i++ {
			if l := ansi.StringWidth(r[i]); l > widths[i] {
				widths[i] = l
			}
		}
	}
	for i, h := range headers {
		if isAgeHead(h) && widths[i] < 7 {
			widths[i] = 7
		}
	}
	return widths
}

func renderTable(headers []string, rows [][]string, colorCol, width int) (string, []string, []string, []int, int, int, int) {
	n := len(headers)
	if n == 0 {
		return "", nil, nil, nil, 0, -1, -1
	}
	widths := naturalWidths(headers, rows)
	flex := flexIndex(headers)
	widths = fitWidths(widths, headers, flex, ageIndex(headers), width)
	header := headerLine(headers, widths)
	var colored, plain []string
	dimCol := -1
	if len(headers) > 0 && headers[0] == "NAMESPACE" {
		dimCol = 0
	}
	ageIdx := ageIndex(headers)
	for _, r := range rows {
		title, plainRow := paintRow(r, widths, headers, colorCol, flex, dimCol, ageIdx, nil)
		colored = append(colored, title)
		plain = append(plain, plainRow)
	}
	return header, colored, plain, widths, flex, dimCol, ageIdx
}

func paintRow(cells []string, widths []int, heads []string, colorCol, flex, dimCol, ageIdx int, over map[int]tone, meters ...map[int]meter) (string, string) {
	var mt map[int]meter
	if len(meters) > 0 {
		mt = meters[0]
	}
	n := len(widths)
	cparts := make([]string, n)
	pparts := make([]string, n)
	marks := rowProblems(heads, cells, colorCol)
	for i := 0; i < n; i++ {
		t := ""
		if i < len(cells) {
			t = cells[i]
		}
		w := widths[i]
		meterTone, metered := tone(0), false
		if me, ok := mt[i]; ok && me.ok && w > 0 {

			t, meterTone, metered = me.cell(t, w)
		}
		cell := fitCell(t, w)
		pparts[i] = ansi.Strip(cell)
		st := itemStyle
		if i == flex {
			st = lipgloss.NewStyle().Foreground(th.Text)
			if marks.name {
				st = warnStyle
			}
		}
		if i == dimCol || i == ageIdx {
			st = dimStyle
		}
		if tn, ok := marks.cells[i]; ok {
			st = toneStyle(tn)
		}
		if i == colorCol {
			st = statusStyle(strings.TrimSpace(t))
		}
		if tn, ok := over[i]; ok {
			st = toneStyle(tn)
		}
		if metered && meterTone != 0 {
			st = toneStyle(meterTone)
		}
		cparts[i] = st.Render(cell)
	}
	return joinCols(cparts, widths), joinCols(pparts, widths)
}

func joinCols(parts []string, widths []int) string {
	shown := make([]string, 0, len(parts))
	for i, p := range parts {
		if i < len(widths) && widths[i] == 0 {
			continue
		}
		shown = append(shown, p)
	}
	return strings.Join(shown, "  ")
}

func headerLine(heads []string, widths []int) string {
	parts := make([]string, len(heads))
	for i, h := range heads {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		parts[i] = fitCell(h, w)
	}
	return joinCols(parts, widths)
}

func isMetricHead(h string) bool { return h == "CPU" || h == "MEM" }

func fitWidths(natural []int, heads []string, flex, protect, width int) []int {
	var keep []int
	dropped := false
	for i := range natural {
		if i < len(heads) && isMetricHead(heads[i]) {
			dropped = true
			continue
		}
		keep = append(keep, i)
	}
	if !dropped {
		return balanceWidths(natural, heads, flex, protect, width)
	}

	fits := func(nat []int) bool {
		need := 0
		for i, w := range nat {
			if i == flex {
				w = min(w, 24)
			}
			need += w
		}
		return need+(len(nat)-1)*2 <= width
	}
	for _, cap := range []int{0, meterMediumMax, meterValueMax} {
		nat := natural
		if cap > 0 {
			nat = append([]int(nil), natural...)
			for i := range nat {
				if i < len(heads) && isMetricHead(heads[i]) {
					nat[i] = min(nat[i], cap)
				}
			}
		}
		if fits(nat) {
			return balanceWidths(nat, heads, flex, protect, width)
		}
	}

	sub := make([]int, len(keep))
	subHeads := make([]string, len(keep))
	subFlex, subProtect := -1, -1
	for j, i := range keep {
		sub[j] = natural[i]
		if i < len(heads) {
			subHeads[j] = heads[i]
		}
		if i == flex {
			subFlex = j
		}
		if i == protect {
			subProtect = j
		}
	}
	fitted := balanceWidths(sub, subHeads, subFlex, subProtect, width)
	out := make([]int, len(natural))
	for j, i := range keep {
		out[i] = fitted[j]
	}
	return out
}

type rowMarks struct {
	name  bool
	cells map[int]tone
}

const recentRestart = time.Hour

func rowProblems(heads, cells []string, statusCol int) rowMarks {
	mk := rowMarks{cells: map[int]tone{}}

	status := ""
	if statusCol >= 0 && statusCol < len(cells) {
		status = strings.TrimSpace(cells[statusCol])
	}
	st := toneNormal
	if status != "" {
		st = statusTone(status)
	}
	if st == toneWarn || st == toneErr {
		mk.name = true
	}
	for i, h := range heads {
		if i >= len(cells) {
			break
		}
		v := strings.TrimSpace(cells[i])
		switch h {
		case "RESTARTS":
			if n, err := strconv.Atoi(strings.Fields(v + " x")[0]); err == nil && n > 0 {
				mk.cells[i] = toneWarn
				if d, ok := restartAgo(v); ok && d < recentRestart {
					mk.cells[i] = toneErr
				}
			}
		case "READY":
			var a, b int
			if _, err := fmt.Sscanf(v, "%d/%d", &a, &b); err == nil && a < b && st != toneDone && !strings.EqualFold(status, "succeeded") {
				mk.cells[i] = toneWarn
				mk.name = true
			}
		case "NAME":
			if strings.HasSuffix(v, " terminating") {
				mk.cells[i] = toneWarn
				mk.name = true
			}
		}
	}
	return mk
}

func restartAgo(cell string) (time.Duration, bool) {
	i, j := strings.Index(cell, "("), strings.Index(cell, " ago)")
	if i < 0 || j <= i {
		return 0, false
	}
	age := cell[i+1 : j]
	if strings.HasSuffix(age, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(age, "d"))
		return time.Duration(n) * 24 * time.Hour, err == nil
	}
	d, err := time.ParseDuration(age)
	return d, err == nil
}

func (it item) aged() item {
	if it.born.IsZero() || it.ageIdx < 0 || it.ageIdx >= len(it.cells) {
		return it
	}
	age := kube.Age(it.born)
	if it.cells[it.ageIdx] == age {
		return it
	}
	cells := append([]string(nil), it.cells...)
	cells[it.ageIdx] = age
	it.cells = cells
	it.title, it.plain = paintRow(cells, it.widths, it.heads, it.colorCol, it.flex, it.dimCol, it.ageIdx, it.tones, it.meters)
	return it
}

func bumpList(lst *list.Model) {
	if lst.FilterState() != list.Unfiltered {
		return
	}
	raws := lst.Items()
	changed := false
	next := make([]list.Item, len(raws))
	for i, raw := range raws {
		it, ok := raw.(item)
		if !ok {
			next[i] = raw
			continue
		}
		aged := it.aged()
		if aged.title != it.title {
			changed = true
		}
		next[i] = aged
	}
	if !changed {
		return
	}
	idx := lst.Index()
	lst.SetItems(next)
	lst.Select(idx)
}

type dirDelegate struct{ width int }

func (d dirDelegate) Height() int { return 1 }

func (d dirDelegate) Spacing() int { return 0 }

func (d dirDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d dirDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	name := it.title
	if name == "" {
		name = it.id
	}
	mark := it.desc == "kust" || it.desc == "kust-file"

	if (it.desc == "kust" || it.desc == "") && name != ".." {
		name += "/"
	}
	if index == m.Index() {
		prefix := "  "
		if mark {
			prefix = "● "
		}
		fmt.Fprint(w, selectedItemStyle.Render(fitCell(prefix+name, d.width)))
		return
	}
	if mark {
		fmt.Fprint(w, okStyle.Render("●")+" "+itemStyle.Render(name))
		return
	}
	if it.desc == "plain-file" && !kustomize.IsYAML(name) {
		fmt.Fprint(w, dimStyle.Render("  "+name))
		return
	}
	fmt.Fprint(w, itemStyle.Render("  "+name))
}

type rowDelegate struct{ width int }

func (d rowDelegate) Height() int { return 1 }

func (d rowDelegate) Spacing() int { return 0 }

func (d rowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d rowDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	if index == m.Index() {
		text := ansi.Strip(it.plain)
		if text == "" {
			text = ansi.Strip(it.title)
		}
		text = fitCell(text, d.width)
		fmt.Fprint(w, selectedItemStyle.Render(text))
		return
	}
	fmt.Fprint(w, clipLine(it.title, d.width))
}

func getResourceItems(k kube.Client, kubeconfig, context, namespace, kind string, width int) (string, []list.Item, error) {
	objs, err := k.Objects(kubeconfig, context, kind, namespace)
	if err != nil {
		if msg := kube.MissingAPI(kind, err.Error()); msg != "" {
			return "", nil, fmt.Errorf("%s", msg)
		}
		return "", nil, err
	}
	return packKube(kube.TableFor(kind, objs, namespace == ""), width, namespace)
}

func getPodItems(k kube.Client, kubeconfig, context, namespace, selector string, width int) (string, []list.Item, error) {
	objs, err := k.ObjectsWhere(kubeconfig, context, "pods", namespace, selector)
	if err != nil {
		return "", nil, err
	}
	return packKube(kube.PodTable(objs, namespace == ""), width, namespace)
}

func packKube(t kube.Table, width int, namespace string) (string, []list.Item, error) {
	header, items, err := packTable(t.Headers, t.Rows, t.Refs, t.Born, t.ColorCol, width, namespace)
	for i := range items {
		if i < len(t.Extra) && t.Extra[i] != nil {
			it := items[i].(item)
			it.extra = t.Extra[i]
			items[i] = it
		}
	}
	return header, items, err
}

func packTable(headers []string, cells [][]string, metas [][2]string, born []time.Time, colorCol, width int, fallbackNS string) (string, []list.Item, error) {
	if width < 20 {
		width = 20
	}
	header, colored, plain, widths, flex, dimCol, ageIdx := renderTable(headers, cells, colorCol, width)
	natural := naturalWidths(headers, cells)
	items := make([]list.Item, len(colored))
	for i := range colored {
		ns, name := metas[i][0], metas[i][1]
		id := name
		if fallbackNS == "" && ns != "" {
			id = ns + "/" + name
		}
		var when time.Time
		if i < len(born) {
			when = born[i]
		}
		row := append([]string(nil), cells[i]...)
		items[i] = item{
			title: colored[i], plain: plain[i], id: id, desc: ns,
			born: when, cells: row, heads: append([]string(nil), headers...), widths: append([]int(nil), widths...),
			natural: natural, ord: i,
			ageIdx: ageIdx, colorCol: colorCol, flex: flex, dimCol: dimCol,
		}
	}
	return header, items, nil
}

func (m *model) refreshVisible() []tea.Cmd {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	var cmds []tea.Cmd
	shown := map[string]bool{}
	for _, id := range m.tabIDs() {
		shown[id] = true
	}
	try := func(id, stage, kind, kc, ctx, ns, sel string, filtering bool) {

		if filtering || (stage != "workpods" && stage != "res") || kc == "" || ctx == "" || m.rowBusy[id] || !shown[id] {
			return
		}
		m.rowBusy[id] = true
		cmds = append(cmds, loadRows(m.k, id, stage, kind, kc, ctx, ns, sel, m.paneTableWidth(id)))
	}
	try(m.logID, m.stage, m.kind, m.kubeconfig, m.context, m.namespace, m.vs.podSel, m.content.FilterState() != list.Unfiltered)
	for i := range m.panes {
		if i == m.focus {
			continue
		}
		p := m.panes[i]
		try(p.logID, p.stage, p.kind, p.kubeconfig, p.context, p.namespace, p.vs.podSel, p.content.FilterState() != list.Unfiltered)
	}
	return cmds
}

func loadRows(k kube.Client, id, stage, kind, kc, ctx, ns, sel string, width int) tea.Cmd {
	return func() tea.Msg {
		var header string
		var items []list.Item
		var err error
		if stage == "workpods" {
			header, items, err = getPodItems(k, kc, ctx, ns, sel, width)
		} else {
			header, items, err = getResourceItems(k, kc, ctx, ns, kind, width)
		}
		return rowsMsg{id: id, header: header, items: items, err: err}
	}
}

func (m *model) applyRows(msg rowsMsg) {
	if m.rowBusy == nil {
		m.rowBusy = map[string]bool{}
	}
	delete(m.rowBusy, msg.id)
	if msg.err != nil {
		return
	}
	apply := func(lst *list.Model, vs viewState, kc, ctx, ns string) {
		if lst.FilterState() != list.Unfiltered {
			return
		}
		header, items := m.withMetrics(msg.header, msg.items, kc, ctx, ns, m.paneTableWidth(msg.id))
		setRows(lst, header, items, vs, "")
	}
	if m.logID == msg.id && (m.stage == "workpods" || m.stage == "res") {
		apply(&m.content, m.vs, m.kubeconfig, m.context, m.namespace)
		return
	}
	for i := range m.panes {
		if p := &m.panes[i]; p.logID == msg.id && (p.stage == "workpods" || p.stage == "res") {
			apply(&p.content, p.vs, p.kubeconfig, p.context, p.namespace)
			return
		}
	}
}

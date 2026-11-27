package ui

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	kindIconRunning = "■"
	kindIconStopped = "□"
	kindIconFailed  = "×"
	kindIconActive  = "⬢"
	kindIconIdle    = "⬡"
	kindIconKube    = "⊚"
	kindIconCharts  = "▦"
	kindIconStep    = "›"
	kindIconSel     = "▸"
	kindTileSafe    = 1
	kindBarCells    = 5
	kindSplitMin    = 54
	kindStatGap     = 3
)

func kindSpinFrames() []string {
	return spinner.MiniDot.Frames
}

func kindSpinGlyph(beat int) string {
	frames := kindSpinFrames()
	if len(frames) == 0 {
		return "⠋"
	}
	if beat < 0 {
		beat = -beat
	}
	return frames[beat%len(frames)]
}

func kindLabel(icon, text string) string {
	return icon + "  " + text
}

func kindJoinRight(left, right string, inner, reserve int) string {
	if inner < 1 {
		return ""
	}
	rw := lipgloss.Width(right)
	if reserve < rw {
		reserve = rw
	}
	if reserve >= inner {
		return clipLine(right, inner)
	}
	room := inner - reserve - 1
	if room < 1 {
		room = 1
	}
	left = clipLine(left, room)
	pad := inner - lipgloss.Width(left) - reserve
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func kindCardWidth(innerW int) int {
	w := innerW - kindTileSafe
	if w < 1 {
		w = max(1, innerW)
	}
	return w
}

func kindStatColWidth(inner int) int {
	w := len("control-plane") + 2 + 8
	if cap := inner / 2; cap < w {
		w = cap
	}
	if w < 16 {
		w = min(16, max(8, inner-kindStatGap-12))
	}
	return w
}

func kindJoinCols(left, right string, leftW, inner int) string {
	if inner < 1 {
		return ""
	}
	if right == "" {
		return clipLine(left, inner)
	}
	left = padPlain(clipLine(left, leftW), leftW)
	room := inner - leftW - kindStatGap
	if room < 1 {
		return clipLine(left, inner)
	}
	return clipLine(left+strings.Repeat(" ", kindStatGap)+clipLine(right, room), inner)
}

type kindTile struct {
	name, status, step, last, kubeconfig string
	active, kcOK, usageOK, chartsOK      bool
	nodes                                kube.KindNodes
	usage                                kube.KindUsage
	pods                                 kube.KindPods
	charts                               []string
	started                              time.Time
	err                                  error
}

type kindLineMsg struct {
	pane, name, line, step string
	ch                     chan tea.Msg
}

type kindConfigDoneMsg struct{}

type kindValuesDoneMsg struct{}

type kindNodeImagesMsg struct {
	images []string
}

type kindInstalledMsg struct {
	pane, name, text, notes string
	err                     error
}

func (m model) kindBusy() bool {
	for _, j := range m.kindJobs {
		if j != nil && (j.status == "deploying" || j.status == "deleting") {
			return true
		}
	}
	return false
}

func (m *model) kindJob(name string) *kindJob {
	if m.kindJobs == nil {
		m.kindJobs = map[string]*kindJob{}
	}
	return m.kindJobs[name]
}

func (m *model) ensureKindJob(name, status, step string) *kindJob {
	if m.kindJobs == nil {
		m.kindJobs = map[string]*kindJob{}
	}
	j := m.kindJobs[name]
	if j == nil {
		j = &kindJob{name: name, started: now()}
		m.kindJobs[name] = j
	}
	if status != "" {
		j.status = status
	}
	if step != "" {
		j.step = step
	}
	if j.started.IsZero() {
		j.started = now()
	}
	return j
}

func kindStep(line string) string {
	l := strings.ToLower(strings.TrimSpace(line))
	switch {
	case l == "":
		return ""
	case strings.Contains(l, "ipaddresspool") || strings.Contains(l, "l2advertisement") || strings.Contains(l, "metallb pool"):
		return "configuring metallb"
	case strings.Contains(l, "uninstall"):
		return "uninstalling " + kindStepName(l, "uninstall")
	case strings.Contains(l, "helm install") || strings.HasPrefix(l, "install ") || strings.Contains(l, "installing "):
		return "installing " + kindStepName(l, "install")
	case strings.Contains(l, "repo add") || strings.Contains(l, "repo update"):
		return "adding helm repos"
	case strings.Contains(l, "deleting"):
		return "deleting cluster"
	case strings.Contains(l, "creating cluster") || strings.Contains(l, "creating node") || strings.Contains(l, "starting control-plane") || strings.Contains(l, "installing storage") || strings.Contains(l, "installing cni"):
		return "creating cluster"
	default:
		return ""
	}
}

func kindStepName(line, verb string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if strings.EqualFold(f, verb) || strings.EqualFold(f, "helm") && i+1 < len(fields) && strings.EqualFold(fields[i+1], verb) {
			start := i + 1
			if strings.EqualFold(f, "helm") {
				start = i + 2
			}
			for _, n := range fields[start:] {
				if strings.HasPrefix(n, "-") {
					continue
				}
				n = strings.Trim(n, `"'`)
				if n != "" && n != verb {
					return n
				}
			}
		}
	}
	return "charts"
}

func (m model) kindTiles() []kindTile {
	seen := map[string]bool{}
	var tiles []kindTile
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		tiles = append(tiles, m.kindTileFor(name))
	}
	for _, name := range m.vs.kindNames {
		add(name)
	}
	for name, j := range m.kindJobs {
		if j != nil && !seen[name] && (j.status == "deploying" || j.status == "deleting" || j.status == "failed") {
			add(name)
		}
	}
	return tiles
}

func (m model) kindTileFor(name string) kindTile {
	t := kindTile{name: name, nodes: m.vs.kindNodes[name]}
	if path, ok := m.kindKubeconfigFor(name); ok {
		t.kubeconfig, t.kcOK = path, true
		t.active = m.kubeconfig == path && (m.context == kube.KindContext(name) || strings.HasPrefix(m.context, "kind-"+name))
	} else {
		home, _ := os.UserHomeDir()
		t.kubeconfig = kube.KindKubeconfig(name, home)
	}
	if u, ok := m.kindUsage[name]; ok {
		t.usage, t.usageOK = u, u.OK
	}
	if names, ok := m.kindCatalog[name]; ok {
		t.charts, t.chartsOK = names, true
	}
	if p, ok := m.kindPods[name]; ok {
		t.pods = p
	}
	if j := m.kindJobs[name]; j != nil && (j.status == "deploying" || j.status == "deleting" || j.status == "failed") {
		t.status, t.step, t.last, t.started, t.err = j.status, j.step, j.last, j.started, j.err
		return t
	}
	if t.nodes.Total > 0 && t.nodes.Running == 0 {
		t.status = "stopped"
		return t
	}
	t.status = "running"
	return t
}

func (m model) selectedKind() (string, bool) {
	tiles := m.kindTiles()
	if len(tiles) == 0 {
		return "", false
	}
	i := m.vs.kindSel
	if i < 0 || i >= len(tiles) {
		i = 0
	}
	return tiles[i].name, true
}

func (m *model) selectKind(name string) {
	for i, t := range m.kindTiles() {
		if t.name == name {
			m.vs.kindSel = i
			m.ensureKindVisible()
			return
		}
	}
}

func (m *model) moveKind(delta int) {
	tiles := m.kindTiles()
	n := len(tiles)
	if n == 0 {
		return
	}
	i := m.vs.kindSel
	if i < 0 || i >= n {
		i = 0
	}
	i += delta
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	m.vs.kindSel = i
	m.ensureKindVisible()
}

func kindKeepVisible(starts, heights []int, sel, at, viewH int) int {
	if viewH <= 0 || len(starts) == 0 {
		return 0
	}
	last := len(starts) - 1
	total := starts[last] + heights[last]
	if total <= viewH {
		return 0
	}
	if sel < 0 {
		sel = 0
	}
	if sel > last {
		sel = last
	}
	top, h := starts[sel], heights[sel]
	bot := top + h
	switch {
	case h >= viewH:
		at = top
	case top < at:
		at = top
	case bot > at+viewH:
		at = bot - viewH
	}
	if maxAt := total - viewH; at > maxAt {
		at = maxAt
	}
	if at < 0 {
		at = 0
	}
	return at
}

func (m model) kindViewHeight() int {
	if m.boxH > 0 {
		return m.boxH
	}
	_, h := m.paneBox(m.logID)
	return max(h-2, 1)
}

func (m *model) ensureKindVisible() {
	if m.stage != "kind" {
		return
	}
	_, starts, heights := m.kindCardBlocks(m.tableWidth())
	m.vs.kindAt = kindKeepVisible(starts, heights, m.vs.kindSel, m.vs.kindAt, m.kindViewHeight())
}

func (m model) kindCardBlocks(innerW int) (blocks []string, starts, heights []int) {
	tiles := m.kindTiles()
	tileW := kindCardWidth(innerW)
	sel := m.vs.kindSel
	if sel < 0 || sel >= len(tiles) {
		sel = 0
	}
	line := 0
	for i, t := range tiles {
		if i > 0 {
			line++
		}
		starts = append(starts, line)
		card := strings.TrimRight(m.renderKindTile(t, i == sel, tileW, 0, nil), "\n")
		h := 0
		if card != "" {
			h = strings.Count(card, "\n") + 1
		}
		heights = append(heights, h)
		blocks = append(blocks, card)
		line += h
	}
	return blocks, starts, heights
}

func (m model) kindBody(innerW int) string {
	tiles := m.kindTiles()
	if len(tiles) == 0 && !m.load.active() {
		k := m.cfg.Keys
		msg := "no kind clusters"
		hint := "press " + k.HelmInstall.Label() + " to create one"
		return "\n" + dimStyle.Render(msg) + "\n" + dimStyle.Render(hint) + "\n"
	}
	if len(tiles) == 0 {
		return m.loadingBlock(innerW)
	}
	blocks, starts, heights := m.kindCardBlocks(innerW)
	body := strings.Join(blocks, "\n")
	if m.boxH <= 0 {
		return body
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	at := kindKeepVisible(starts, heights, m.vs.kindSel, m.vs.kindAt, m.boxH)
	return strings.Join(pageLines(lines, at, m.boxH), "\n")
}

func (m model) kindSideLines(t kindTile, width int) []string {
	busy := t.status == "deploying" || t.status == "deleting"
	switch {
	case busy && t.step != "":
		return []string{clipLine(dimStyle.Render(kindLabel(kindIconStep, ""))+itemStyle.Render(t.step), width)}
	case t.err != nil:
		return []string{clipLine(errStyle.Render(kindLabel(kindIconStep, t.err.Error())), width)}
	case t.status == "failed" && t.last != "":
		return []string{clipLine(errStyle.Render(kindLabel(kindIconCharts, t.last)), width)}
	case busy && t.last != "":
		return []string{clipLine(dimStyle.Render(kindLabel(kindIconCharts, t.last)), width)}
	default:
		return m.kindUsageLines(t, width)
	}
}

func (m model) kindExtraLines(t kindTile, width int) []string {
	busy := t.status == "deploying" || t.status == "deleting"
	switch {
	case busy && t.last != "" && t.step != "":
		return []string{clipLine(dimStyle.Render(kindLabel(kindIconCharts, t.last)), width)}
	case t.status == "failed" && t.last != "" && t.err != nil:
		return []string{clipLine(errStyle.Render(kindLabel(kindIconCharts, t.last)), width)}
	default:
		return m.kindChartsLines(t, width)
	}
}

func (m model) kindWideDetails(t kindTile, inner int) []string {
	leftW := kindStatColWidth(inner)
	info := m.kindInfoLines(t, leftW)
	side := m.kindSideLines(t, max(8, inner-leftW-kindStatGap))
	n := max(len(info), len(side))
	var lines []string
	for i := 0; i < n; i++ {
		left, right := "", ""
		if i < len(info) {
			left = info[i]
		}
		if i < len(side) {
			right = side[i]
		}
		lines = append(lines, kindJoinCols(left, right, leftW, inner))
	}
	return append(lines, m.kindExtraLines(t, inner)...)
}

func (m model) kindNarrowDetails(t kindTile, inner int) []string {
	var lines []string
	lines = append(lines, m.kindInfoLines(t, inner)...)
	lines = append(lines, m.kindSideLines(t, inner)...)
	return append(lines, m.kindExtraLines(t, inner)...)
}

func (m model) kindTileContent(t kindTile, selected bool, inner int) []string {
	name := t.name
	if selected {
		name = kindLabel(kindIconSel, t.name)
	}
	title := titleStyle.Render(name)
	if t.active {
		title = title + " " + keyStyle.Render(kindIconActive+" active")
	}
	status := m.kindStatusLine(t, inner)
	icon, iconSt := kindIconIdle, dimStyle
	if t.active {
		icon, iconSt = kindIconActive, keyStyle
	}
	iw := lipgloss.Width(icon)
	if iw < 1 {
		iw = 1
	}
	title = kindJoinRight(title, iconSt.Render(icon), inner, iw+kindTileSafe)
	var lines []string
	lines = append(lines, title, status)
	if inner >= kindSplitMin {
		lines = append(lines, m.kindWideDetails(t, inner)...)
	} else {
		lines = append(lines, m.kindNarrowDetails(t, inner)...)
	}
	kc := config.HomePath(t.kubeconfig)
	if kc == "" {
		kc = "—"
	}
	lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconKube, kc)), inner))
	return lines
}

func (m model) renderKindTile(t kindTile, selected bool, width, contentH int, lines []string) string {
	inner := max(8, width-2)
	if lines == nil {
		lines = m.kindTileContent(t, selected, inner)
	}
	if contentH > 0 {
		for len(lines) < contentH {
			lines = append(lines, "")
		}
		if len(lines) > contentH {
			lines = lines[:contentH]
		}
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Muted).
		Width(inner)
	if selected {
		border = border.BorderForeground(th.Accent)
	} else if t.status == "failed" {
		border = border.BorderForeground(th.Danger)
	}
	return border.Render(strings.Join(lines, "\n"))
}

func (m model) kindStatusIcon(status string) string {
	switch status {
	case "stopped":
		return kindIconStopped
	case "deploying":
		if m.cfg.UI.Animations {
			return kindSpinGlyph(m.beat)
		}
		return kindSpinGlyph(0)
	case "deleting":
		if m.cfg.UI.Animations {
			return kindSpinGlyph(m.beat + 2)
		}
		return kindSpinGlyph(0)
	case "failed":
		return kindIconFailed
	default:
		return kindIconRunning
	}
}

func (m model) kindStatusLine(t kindTile, _ int) string {
	icon, st, label := m.kindStatusIcon(t.status), okStyle, t.status
	switch t.status {
	case "stopped":
		st = dimStyle
	case "deploying":
		st = warnStyle
		label = t.status + "  " + elapsed(now().Sub(t.started))
	case "deleting":
		st = warnStyle
		label = t.status + "  " + elapsed(now().Sub(t.started))
	case "failed":
		st, label = errStyle, "failed"
	default:
		st, label = okStyle, "running"
		if t.nodes.Uptime != "" {
			label += " · " + t.nodes.Uptime
		}
	}
	return st.Render(kindLabel(icon, label))
}

func kindMiniBar(pct float64, n int) string {
	if n < 1 {
		n = kindBarCells
	}
	filled := int(pct/100*float64(n) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > n {
		filled = n
	}
	if pct > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("■", filled) + strings.Repeat("□", n-filled)
}

func kindMetricStyle(m model, pct float64) lipgloss.Style {
	st := okStyle
	switch {
	case pct >= float64(max(1, m.cfg.UI.MetricsCrit)):
		st = errStyle
	case pct >= float64(max(1, m.cfg.UI.MetricsWarn)):
		st = warnStyle
	}
	return st
}

func kindStatLine(label, value string, inner int) string {
	pad := len("control-plane") + 2 - len(label)
	if pad < 1 {
		pad = 1
	}
	return clipLine(dimStyle.Render(label+strings.Repeat(" ", pad))+itemStyle.Render(value), inner)
}

func (m model) kindInfoLines(t kindTile, inner int) []string {
	ver := t.nodes.Version
	if ver == "" {
		ver = "—"
	}
	cp, wr := t.nodes.Planes, t.nodes.Workers
	if cp == 0 && wr == 0 && t.nodes.Total > 0 {
		cp = t.nodes.Total
	}
	pods := "—"
	if t.status == "running" && t.pods.OK {
		pods = t.pods.Label()
	}
	return []string{
		kindStatLine("k8s", ver, inner),
		kindStatLine("control-plane", strconv.Itoa(cp), inner),
		kindStatLine("workers", strconv.Itoa(wr), inner),
		kindStatLine("pods", pods, inner),
	}
}

func (m model) kindUsageLines(t kindTile, width int) []string {
	if t.status == "stopped" || !t.usageOK {
		return []string{
			clipLine(dimStyle.Render("CPU  —"), width),
			clipLine(dimStyle.Render("RAM  —"), width),
		}
	}
	cpuBar := kindMiniBar(t.usage.CPU, kindBarCells)
	ramBar := kindMiniBar(t.usage.MemPct, kindBarCells)
	cpu := "CPU  " + kube.FormatCPU(t.usage.CPU) + "  " + kindMetricStyle(m, t.usage.CPU).Render(cpuBar)
	ram := "RAM  " + kube.FormatBytes(t.usage.MemUsed) + " (" + kube.FormatCPU(t.usage.MemPct) + ")  " + kindMetricStyle(m, t.usage.MemPct).Render(ramBar)
	return []string{clipLine(cpu, width), clipLine(ram, width)}
}

func (m model) kindChartsLines(t kindTile, inner int) []string {
	if !t.chartsOK {
		if t.status != "running" {
			return nil
		}
		return []string{dimStyle.Render(kindLabel(kindIconCharts, "—"))}
	}
	if len(t.charts) == 0 {
		return []string{dimStyle.Render(kindLabel(kindIconCharts, "none"))}
	}
	prefix := kindLabel(kindIconCharts, "")
	padW := lipgloss.Width(prefix)
	room := inner - padW
	if room < 8 {
		room = 8
	}
	parts := wrapCSV(t.charts, room)
	indent := strings.Repeat(" ", padW)
	var lines []string
	for i, p := range parts {
		head := prefix
		if i > 0 {
			head = indent
		}
		lines = append(lines, clipLine(dimStyle.Render(head)+itemStyle.Render(p), inner))
	}
	return lines
}

func wrapCSV(items []string, width int) []string {
	width = max(width, 2)
	var lines []string
	var cur string
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cand := item
		if cur != "" {
			cand = cur + ", " + item
		}
		if lipgloss.Width(cand) <= width {
			cur = cand
			continue
		}
		flush()
		if lipgloss.Width(item) <= width {
			cur = item
			continue
		}
		parts := wrapLine(item, width)
		if len(parts) == 0 {
			continue
		}
		lines = append(lines, parts[:len(parts)-1]...)
		cur = parts[len(parts)-1]
	}
	flush()
	return lines
}

func kindDetailParts(line string) (prefix, value string, ok bool) {
	if len(line) < kindDetailCol+1 || line[kindDetailCol] != ' ' {
		return "", "", false
	}
	return line[:kindDetailCol+1], line[kindDetailCol+1:], true
}

func wrapKindDetail(line string, width int) []string {
	prefix, value, ok := kindDetailParts(line)
	if !ok {
		return wrapLine(line, width)
	}
	width = max(width, 2)
	if lipgloss.Width(line) <= width {
		return []string{line}
	}
	padW := lipgloss.Width(prefix)
	vw := max(width-padW, 2)
	var parts []string
	if strings.Contains(value, ", ") {
		parts = wrapCSV(strings.Split(value, ", "), vw)
	} else {
		parts = wrapLine(value, vw)
	}
	if len(parts) == 0 {
		return []string{strings.TrimRight(prefix, " ")}
	}
	pad := strings.Repeat(" ", padW)
	out := make([]string, len(parts))
	out[0] = prefix + parts[0]
	for i := 1; i < len(parts); i++ {
		out[i] = pad + parts[i]
	}
	return out
}

func (m *model) applyKindLine(msg kindLineMsg) {
	if msg.name == "" {
		return
	}
	j := m.ensureKindJob(msg.name, "", "")
	if msg.step != "" {
		j.step = msg.step
	} else if step := kindStep(msg.line); step != "" {
		j.step = step
	}
	if msg.line != "" {
		j.last = strings.TrimSpace(msg.line)
		if j.log != "" && !strings.HasSuffix(j.log, "\n") {
			j.log += "\n"
		}
		j.log += msg.line
		if !strings.HasSuffix(j.log, "\n") {
			j.log += "\n"
		}
	}
	m.syncKindLog(msg.name)
}

func (m *model) syncKindLog(name string) {
	if m.stage != "kindlog" || m.vs.kindLog != name {
		return
	}
	if j := m.kindJobs[name]; j != nil {
		m.logText = j.log
		m.subject = "kind " + name
	}
}

func (m model) openKindLogs(name string) (tea.Model, tea.Cmd) {
	if m.kindJobs == nil {
		m.kindJobs = map[string]*kindJob{}
	}
	j := m.kindJobs[name]
	if j == nil {
		m.status = "no logs for " + name
		return m, m.notify(toneWarn, m.status)
	}
	if m.stage != "kindlog" {
		m.rememberView()
	}
	m.stage = "kindlog"
	m.vs.kindLog = name
	m.vs.vback = "kind"
	m.subject = "kind " + name
	m.logText = j.log
	m.logStick, m.logAt = true, 0
	switch j.status {
	case "deploying":
		m.status = "deploying " + name + " · " + m.cfg.Keys.Back.Label() + " back to tiles"
	case "deleting":
		m.status = "deleting " + name
	case "failed":
		m.status = m.errLine(j.err)
		if m.status == "" {
			m.status = name + " failed"
		}
	default:
		m.status = name + " · " + m.cfg.Keys.Back.Label() + " back"
	}
	return m, nil
}

func (m model) backKindLog() (tea.Model, tea.Cmd, bool) {
	name := m.vs.kindLog
	m.stopKindLiveLog()
	if j := m.kindJobs[name]; j != nil && j.status == "logs" {
		delete(m.kindJobs, name)
	}
	m.stage = "kind"
	m.vs.kindLog = ""
	m.vs.vback = ""
	if name != "" {
		m.selectKind(name)
	}
	m.restoreKindSnap()
	m.showKind("")
	m.status = m.kindHint()
	cmd := m.kindRefreshCmd()
	return m, cmd, true
}

func (m *model) stopKindLiveLog() {
	if m.kindLogCancel != nil {
		m.kindLogCancel()
		m.kindLogCancel = nil
	}
}

type kindLiveLogDoneMsg struct {
	name, text string
	err        error
}

func (m *model) applyKindLiveLog(msg kindLiveLogDoneMsg) {
	if m.vs.kindLog == msg.name {
		m.kindLogCancel = nil
	}
	if msg.err == nil || errors.Is(msg.err, context.Canceled) {
		return
	}
	if j := m.kindJobs[msg.name]; j != nil && j.status == "logs" {
		j.last = lastNonEmpty(msg.text)
	}
	if m.stage == "kindlog" && m.vs.kindLog == msg.name {
		m.status = m.errLine(msg.err)
		m.syncKindLog(msg.name)
	}
}

func (m model) kindRefreshCmd() tea.Cmd {
	bin := m.kindCLI()
	if bin == "" {
		return nil
	}
	return kindListCmd(bin, m.dockerCLI(), m.run, m.logID, 0)
}

func (m model) useKindTile() (tea.Model, tea.Cmd) {
	name, ok := m.selectedKind()
	if !ok {
		return m, nil
	}
	t := m.kindTileFor(name)
	switch t.status {
	case "deploying", "deleting", "failed":
		return m.openKindLogs(name)
	}
	if j := m.kindJobs[name]; j != nil && j.log != "" && !t.kcOK {
		return m.openKindLogs(name)
	}
	return m.useKindCluster(name)
}

func (m model) enterKindTile() (tea.Model, tea.Cmd) {
	name, ok := m.selectedKind()
	if !ok {
		return m, nil
	}
	t := m.kindTileFor(name)
	switch t.status {
	case "deploying", "deleting", "failed":
		return m.openKindLogs(name)
	}
	if j := m.kindJobs[name]; j != nil && j.log != "" && !t.kcOK {
		return m.openKindLogs(name)
	}
	return m.openKindControlPlaneLogs(name)
}

func (m model) openKindControlPlaneLogs(name string) (tea.Model, tea.Cmd) {
	docker, next, cmd, ok := m.needDockerCLI("cluster logs")
	if !ok {
		return next, cmd
	}
	m.stopKindLiveLog()
	ctx, cancel := context.WithCancel(context.Background())
	m.kindLogCancel = cancel
	j := m.ensureKindJob(name, "logs", "control-plane")
	j.status, j.step, j.log, j.last, j.err, j.started = "logs", "control-plane", "", "", nil, now()
	if m.stage != "kindlog" {
		m.rememberView()
	}
	m.stage = "kindlog"
	m.vs.kindLog = name
	m.vs.vback = "kind"
	m.subject = "kind " + name
	m.logText = ""
	m.logStick, m.logAt = true, 0
	m.status = name + " control-plane · " + m.cfg.Keys.Back.Label() + " back to tiles"
	return m, m.kindStreamCtx(name, m.run, docker, kube.DockerControlPlaneLogsArgs(name), "", ctx, func(text string, err error) tea.Msg {
		return kindLiveLogDoneMsg{name: name, text: text, err: err}
	})
}

package ui

import (
	"os"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	kindIconRunning = "■"
	kindIconStopped = "□"
	kindIconDeploy  = "◧"
	kindIconDelete  = "◨"
	kindIconFailed  = "×"
	kindIconActive  = "◆"
	kindIconNodes   = "▣"
	kindIconKube    = "⊚"
	kindIconCharts  = "▦"
	kindIconStep    = "›"
	kindIconSel     = "▸"
	kindTileMinW    = 28
	kindTileMaxW    = 44
	kindTileGap     = 1
	kindTileSafe    = 1
	kindTileRows    = 8
	kindBarCells    = 5
)

var kindSpinSquares = []string{"◧", "◨", "◩", "◪"}

func kindLabel(icon, text string) string {
	return icon + "  " + text
}

func kindTileWidth(innerW, cols int) int {
	if cols < 1 {
		cols = 1
	}
	w := (innerW - (cols-1)*kindTileGap - cols*kindTileSafe) / cols
	if w < 1 {
		w = 1
	}
	if w > kindTileMaxW {
		w = kindTileMaxW
	}
	return w
}

type kindTile struct {
	name, status, step, last, kubeconfig string
	active, kcOK, usageOK, chartsOK      bool
	nodes                                kube.KindNodes
	usage                                kube.KindUsage
	charts                               []string
	started                              time.Time
	err                                  error
}

type kindLineMsg struct {
	pane, name, line, step string
	ch                     chan tea.Msg
}

type kindConfigDoneMsg struct{}

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

func (m model) kindColumns(width int) int {
	n := 1
	for c := 4; c >= 1; c-- {
		if kindTileWidth(width, c) >= kindTileMinW {
			n = c
			break
		}
	}
	return n
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
			return
		}
	}
}

func (m *model) moveKind(delta, cols int) {
	tiles := m.kindTiles()
	n := len(tiles)
	if n == 0 {
		return
	}
	if cols < 1 {
		cols = 1
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
	cols := m.kindColumns(innerW)
	if cols > len(tiles) {
		cols = len(tiles)
	}
	if cols < 1 {
		cols = 1
	}
	tileW := kindTileWidth(innerW, cols)
	if tileW < kindTileMinW && cols > 1 {
		cols = 1
		tileW = kindTileWidth(innerW, 1)
	}
	sel := m.vs.kindSel
	if sel < 0 || sel >= len(tiles) {
		sel = 0
	}
	var rows [][]string
	var row []string
	for i, t := range tiles {
		row = append(row, m.renderKindTile(t, i == sel, tileW))
		if len(row) == cols {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		for len(row) < cols {
			row = append(row, strings.Repeat(" ", tileW))
		}
		rows = append(rows, row)
	}
	gap := strings.Repeat(" ", kindTileGap)
	var b strings.Builder
	for r, cells := range rows {
		if r > 0 {
			b.WriteByte('\n')
		}
		lines := make([][]string, kindTileRows)
		for _, cell := range cells {
			cs := strings.Split(strings.TrimRight(cell, "\n"), "\n")
			for i := 0; i < kindTileRows; i++ {
				s := ""
				if i < len(cs) {
					s = cs[i]
				}
				lines[i] = append(lines[i], padPlain(s, tileW))
			}
		}
		for i, parts := range lines {
			b.WriteString(strings.Join(parts, gap))
			if i < len(lines)-1 || r < len(rows)-1 {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func (m model) renderKindTile(t kindTile, selected bool, width int) string {
	inner := max(8, width-2)
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Muted).
		Width(inner)
	if selected {
		border = border.Border(lipgloss.ThickBorder()).BorderForeground(th.Accent)
	} else if t.status == "failed" {
		border = border.BorderForeground(th.Danger)
	}

	name := t.name
	if selected {
		name = kindLabel(kindIconSel, t.name)
	}
	title := titleStyle.Render(name)
	if t.active {
		title += " " + keyStyle.Render(kindLabel(kindIconActive, "ACTIVE"))
	}
	status := m.kindStatusLine(t, inner)
	var lines []string
	lines = append(lines, clipLine(title, inner), clipLine(status, inner))
	if sum := t.nodes.Summary(); sum != "" {
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconNodes, ""))+itemStyle.Render(sum), inner))
	} else if t.status == "deploying" || t.status == "deleting" {
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconNodes, "nodes coming up")), inner))
	} else {
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconNodes, "no node info")), inner))
	}
	busy := t.status == "deploying" || t.status == "deleting"
	switch {
	case busy && t.step != "":
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconStep, ""))+itemStyle.Render(t.step), inner))
	case t.err != nil:
		lines = append(lines, clipLine(errStyle.Render(kindLabel(kindIconStep, t.err.Error())), inner))
	case t.status == "failed" && t.last != "":
		lines = append(lines, clipLine(errStyle.Render(kindLabel(kindIconCharts, t.last)), inner))
	case busy && t.last != "":
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconCharts, t.last)), inner))
	default:
		lines = append(lines, clipLine(m.kindUsageLine(t, inner), inner))
	}
	switch {
	case busy && t.last != "" && t.step != "":
		lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconCharts, t.last)), inner))
	case t.status == "failed" && t.last != "" && t.err != nil:
		lines = append(lines, clipLine(errStyle.Render(kindLabel(kindIconCharts, t.last)), inner))
	default:
		lines = append(lines, clipLine(m.kindChartsLine(t), inner))
	}
	kc := config.HomePath(t.kubeconfig)
	if kc == "" {
		kc = "—"
	}
	lines = append(lines, clipLine(dimStyle.Render(kindLabel(kindIconKube, kc)), inner))
	for len(lines) < kindTileRows-2 {
		lines = append(lines, "")
	}
	if len(lines) > kindTileRows-2 {
		lines = lines[:kindTileRows-2]
	}
	return border.Render(strings.Join(lines, "\n"))
}

func (m model) kindStatusIcon(status string) string {
	switch status {
	case "stopped":
		return kindIconStopped
	case "deploying":
		if m.cfg.UI.Animations && len(kindSpinSquares) > 0 {
			return kindSpinSquares[m.beat%len(kindSpinSquares)]
		}
		return kindIconDeploy
	case "deleting":
		if m.cfg.UI.Animations && len(kindSpinSquares) > 0 {
			return kindSpinSquares[(m.beat+2)%len(kindSpinSquares)]
		}
		return kindIconDelete
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

func (m model) kindUsageLine(t kindTile, width int) string {
	if t.status == "stopped" || !t.usageOK {
		return dimStyle.Render("CPU  —    RAM —")
	}
	cpu := kube.FormatCPU(t.usage.CPU)
	mem := kube.FormatBytes(t.usage.MemUsed)
	bar := kindMiniBar(t.usage.MemPct, kindBarCells)
	if t.usage.MemPct <= 0 && t.nodes.Running > 0 {
		bar = kindMiniBar(t.usage.CPU/float64(100*t.nodes.Running)*100, kindBarCells)
	}
	st := okStyle
	switch {
	case t.usage.MemPct >= float64(max(1, m.cfg.UI.MetricsCrit)):
		st = errStyle
	case t.usage.MemPct >= float64(max(1, m.cfg.UI.MetricsWarn)):
		st = warnStyle
	}
	full := "CPU  " + cpu + "  " + st.Render(bar) + "  RAM " + mem
	if lipgloss.Width(full) <= width {
		return full
	}
	return cpu + "  " + st.Render(bar) + "  " + mem
}

func (m model) kindChartsLine(t kindTile) string {
	if !t.chartsOK {
		if t.status != "running" {
			return ""
		}
		return dimStyle.Render(kindLabel(kindIconCharts, "—"))
	}
	if len(t.charts) == 0 {
		return dimStyle.Render(kindLabel(kindIconCharts, "none"))
	}
	return dimStyle.Render(kindLabel(kindIconCharts, "")) + itemStyle.Render(strings.Join(t.charts, ", "))
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
	m.stage = "kind"
	m.vs.kindLog = ""
	m.vs.vback = ""
	if name != "" {
		m.selectKind(name)
	}
	m.status = m.kindHint()
	cmd := m.kindRefreshCmd()
	return m, cmd, true
}

func (m model) kindRefreshCmd() tea.Cmd {
	bin := m.kindCLI()
	if bin == "" {
		return nil
	}
	return kindListCmd(bin, m.dockerCLI(), m.run, m.logID, 0)
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
	return m.useKindCluster(name)
}

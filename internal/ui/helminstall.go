package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type helmOp struct {
	act                          string
	name, chart, version         string
	ns, values                   string
	createNS                     bool
	catalog                      string
	autoName, autoNS, autoValues bool
	manualCreate                 bool
	current                      kube.HelmRelease
	charts                       []kube.HelmChart
	versions                     []string
	preview, summary             string
}

type helmField struct {
	key, label string
	input      textinput.Model
	locked     bool
	toggle     bool
	action     bool
	on         bool
}

type helmSearchMsg struct {
	seq    int
	charts []kube.HelmChart
	err    error
}

type helmVersMsg struct {
	seq      int
	chart    string
	versions []string
	err      error
}

type helmTplMsg struct {
	pane          string
	token         int
	text, summary string
	err           error
}

func (m model) needHelmCLI(act string) (string, tea.Model, tea.Cmd, bool) {
	bin := m.helmCLI()
	if bin != "" {
		return bin, m, nil, true
	}
	why := "helm.binary is empty"
	if m.cfg.Helm.Binary != "" {
		why = m.cfg.Helm.Binary + " is not in PATH"
	}
	m.status = act + " needs the helm CLI (" + why + ")"
	return "", m, m.notify(toneWarn, act+" needs the helm CLI"), false
}

func (m model) openHelmInstall(op helmOp) (tea.Model, tea.Cmd) {
	if op.act == "" {
		op.act = "install"
	}
	if next, cmd, blocked := m.readOnlyBlock(op.act); blocked {
		return next, cmd
	}
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	_, next, cmd, ok := m.needHelmCLI(op.act)
	if !ok {
		return next, cmd
	}
	if op.ns == "" {
		op.ns = m.namespace
	}
	if op.act == "install" && op.ns == "" {
		op.ns = "default"
	}
	title := "✚ helm install"
	if op.act == "upgrade" {
		title = "↑ helm upgrade"
	}
	d := m.newDialog("helminstall", title, "helm", op.ns, op.name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillHelmForm(d, &op)
	m.vs.hop = &op
	m.dlg = d
	m.maybeFillCatalog()
	k, bin, seq := m.k, m.cfg.Helm.Binary, d.seq
	return m, func() tea.Msg {
		out, err := k.HelmOut(bin, kube.HelmSearchArgs("", false)...)
		if err != nil {
			return helmSearchMsg{seq: seq, err: err}
		}
		return helmSearchMsg{seq: seq, charts: kube.ParseHelmSearch(out)}
	}
}

func (m model) openHelmUpgrade(r kube.HelmRelease) (tea.Model, tea.Cmd) {
	op := helmOp{act: "upgrade", name: r.Name, ns: r.Namespace, current: r}
	op.chart, op.version = splitReleaseChart(r.Chart)
	if dir, err := os.MkdirTemp("", "k2-helm-"); err == nil {
		path := filepath.Join(dir, r.Name+"-values.yaml")
		if os.WriteFile(path, []byte(kube.ValuesYAML(r.Config)), 0o600) == nil {
			op.values = path
		}
	}
	return m.openHelmInstall(op)
}

func splitReleaseChart(chart string) (name, version string) {
	name = chart
	if i := strings.LastIndex(chart, "-"); i > 0 {
		v := chart[i+1:]
		if v != "" && (v[0] >= '0' && v[0] <= '9') {
			return chart[:i], v
		}
	}
	return chart, ""
}

func helmFieldInput(value, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle, ti.TextStyle = keyStyle, itemStyle
	ti.CharLimit = 512
	ti.SetValue(value)
	ti.CursorEnd()
	return ti
}

func (m *model) fillHelmForm(d *dialog, op *helmOp) {
	locked := op.act == "upgrade"
	fields := []helmField{
		{key: "name", label: "release", input: helmFieldInput(op.name, "kafbat-ui"), locked: locked},
		{key: "chart", label: "chart", input: helmFieldInput(op.chart, "repo/chart"), locked: locked},
		{key: "version", label: "version", input: helmFieldInput(op.version, "latest")},
		{key: "ns", label: "namespace", input: helmFieldInput(op.ns, "default"), locked: locked},
	}
	if op.act == "install" {
		fields = append(fields, helmField{key: "create", label: "create ns", input: helmFieldInput("", ""), toggle: true})
	}
	fields = append(fields,
		helmField{key: "values", label: "values", input: helmFieldInput(op.values, "path to values.yaml")},
		helmField{key: "editvals", label: "edit values", input: helmFieldInput("", ""), action: true},
		helmField{key: "defaults", label: "chart defaults", input: helmFieldInput("", ""), action: true},
	)
	focus := 0
	if locked {
		focus = 2
	}
	helmFocus(&fields[focus])
	d.hfields = fields
	d.hcreate = op.createNS
	d.idx = focus
}

func (m model) helmFormHint(act string) string {
	k := m.cfg.Keys
	if m.dlg != nil && m.dlg.idx >= 0 && m.dlg.idx < len(m.dlg.hfields) && m.dlg.hfields[m.dlg.idx].key == "chart" {
		return "tab/shift+tab cycle charts · ↑/↓ move · enter previews · " + k.Cancel.Short() + " cancel"
	}
	return "tab moves · " + k.Complete.Short() + " completes · enter previews · " + k.Cancel.Short() + " cancel"
}

func (m *model) stashHelmForm() {
	if m.dlg == nil || m.vs.hop == nil {
		return
	}
	op := *m.vs.hop
	for _, f := range m.dlg.hfields {
		switch f.key {
		case "name":
			op.name = strings.TrimSpace(f.input.Value())
		case "chart":
			op.chart = strings.TrimSpace(f.input.Value())
		case "version":
			op.version = strings.TrimSpace(f.input.Value())
		case "ns":
			op.ns = strings.TrimSpace(f.input.Value())
		case "values":
			op.values = strings.TrimSpace(f.input.Value())
		}
	}
	op.createNS = m.dlg.hcreate
	m.vs.hop = &op
}

func (m model) applyHelmSearch(msg helmSearchMsg) (tea.Model, tea.Cmd) {
	if m.dlg == nil || m.dlg.act != "helminstall" || m.dlg.seq != msg.seq {
		return m, nil
	}
	if msg.err != nil {
		m.dlg.msg = msg.err.Error()
		return m, nil
	}
	if m.vs.hop != nil {
		op := *m.vs.hop
		op.charts = msg.charts
		if op.act == "upgrade" && !strings.Contains(op.chart, "/") {
			if hit := matchChart(msg.charts, op.chart); hit != "" {
				op.chart = hit
				setHelmField(m.dlg, "chart", hit)
			}
		}
		m.vs.hop = &op
		m.syncHelmChartSuggest()
		return m, m.fetchHelmVersions(op.chart, msg.seq)
	}
	return m, nil
}

func matchChart(charts []kube.HelmChart, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var loose string
	for _, c := range kube.ChartNames(charts) {
		if c == name || strings.HasSuffix(c, "/"+name) {
			return c
		}
		if loose == "" && (strings.Contains(c, name) || strings.HasSuffix(name, c)) {
			loose = c
		}
	}
	return loose
}

func setHelmField(d *dialog, key, value string) {
	for i := range d.hfields {
		if d.hfields[i].key == key {
			d.hfields[i].input.SetValue(value)
			d.hfields[i].input.CursorEnd()
		}
	}
}

func helmFieldValue(d *dialog, key string) string {
	for _, f := range d.hfields {
		if f.key == key {
			return strings.TrimSpace(f.input.Value())
		}
	}
	return ""
}

func (m model) fetchHelmVersions(chart string, seq int) tea.Cmd {
	chart = strings.TrimSpace(chart)
	if chart == "" {
		return nil
	}
	k, bin := m.k, m.cfg.Helm.Binary
	return func() tea.Msg {
		out, err := k.HelmOut(bin, kube.HelmSearchArgs(chart, true)...)
		if err != nil {
			return helmVersMsg{seq: seq, chart: chart, err: err}
		}
		return helmVersMsg{seq: seq, chart: chart, versions: kube.ChartVersions(kube.ParseHelmSearch(out), chart)}
	}
}

func (m model) applyHelmVers(msg helmVersMsg) (tea.Model, tea.Cmd) {
	if m.dlg == nil || m.dlg.act != "helminstall" || m.dlg.seq != msg.seq {
		return m, nil
	}
	if m.vs.hop == nil || helmFieldValue(m.dlg, "chart") != msg.chart {
		return m, nil
	}
	op := *m.vs.hop
	op.versions = msg.versions
	m.vs.hop = &op
	cur := helmFieldValue(m.dlg, "version")
	if cur == "" && len(msg.versions) > 0 {
		setHelmField(m.dlg, "version", msg.versions[0])
	}
	return m, nil
}

func (f helmField) typed() bool {
	return !f.toggle && !f.action && !f.locked
}

func helmBlur(f *helmField) {
	if f.typed() {
		f.input.Blur()
	}
}

func helmFocus(f *helmField) {
	if f.typed() {
		f.input.Focus()
	}
}

func (m model) helmFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := m.dlg
	if d == nil || len(d.hfields) == 0 {
		return m, nil
	}
	if d.idx < 0 || d.idx >= len(d.hfields) {
		d.idx = 0
	}
	switch {
	case key == "tab" || key == "down" || km.ScrollDown.Has(key):
		if key == "tab" && d.hfields[d.idx].key == "chart" {
			return m.cycleHelmChart(1)
		}
		if key == "tab" && d.hfields[d.idx].typed() {
			if next, cmd, done := m.completeHelmField(); done {
				return next, cmd
			}
		}
		return m.moveHelmField(1), nil
	case key == "shift+tab" || key == "up" || km.ScrollUp.Has(key):
		if key == "shift+tab" && d.hfields[d.idx].key == "chart" {
			return m.cycleHelmChart(-1)
		}
		return m.moveHelmField(-1), nil
	case (key == " " || key == "c") && d.hfields[d.idx].toggle:
		d.hcreate = !d.hcreate
		if m.vs.hop != nil {
			m.vs.hop.manualCreate = true
		}
		return m, nil
	}
	if d.hfields[d.idx].locked || d.hfields[d.idx].toggle || d.hfields[d.idx].action {
		return m, nil
	}
	var cmd tea.Cmd
	d.hfields[d.idx].input, cmd = d.hfields[d.idx].input.Update(msg)
	d.msg = ""
	if d.hfields[d.idx].key == "chart" {
		m.refreshHelmChartSuggest(true)
		return m, tea.Batch(cmd, m.liveHelmVersions())
	}
	d.cands, d.candBase, d.candAt = nil, "", -1
	if m.vs.hop != nil {
		switch d.hfields[d.idx].key {
		case "name":
			m.vs.hop.autoName = false
		case "ns":
			m.vs.hop.autoNS = false
		case "values":
			m.vs.hop.autoValues = false
		}
	}
	return m, cmd
}

func (m model) liveHelmVersions() tea.Cmd {
	if m.dlg == nil {
		return nil
	}
	chart := helmFieldValue(m.dlg, "chart")
	if m.vs.hop != nil && len(m.vs.hop.versions) > 0 && m.vs.hop.chart == chart {
		return nil
	}
	return m.fetchHelmVersions(chart, m.dlg.seq)
}

func (m model) moveHelmField(delta int) model {
	d := m.dlg
	if d == nil || len(d.hfields) == 0 {
		return m
	}
	n := len(d.hfields)
	if d.idx < 0 || d.idx >= n {
		d.idx = 0
	}
	helmBlur(&d.hfields[d.idx])
	if delta != 0 {
		for i := 0; i < n; i++ {
			d.idx = (d.idx + delta%n + n) % n
			if !d.hfields[d.idx].locked {
				break
			}
		}
	}
	helmFocus(&d.hfields[d.idx])
	d.cands, d.candBase, d.candAt = nil, "", -1
	d.msg = ""
	m.maybeFillCatalog()
	if d.hfields[d.idx].key == "chart" {
		m.syncHelmChartSuggest()
	}
	return m
}

func (m model) completeHelmField() (tea.Model, tea.Cmd, bool) {
	d := m.dlg
	f := d.hfields[d.idx]
	switch f.key {
	case "chart":
		next, cmd := m.cycleHelmChart(1)
		return next, cmd, true
	case "version":
		vers := []string{}
		if m.vs.hop != nil {
			vers = m.vs.hop.versions
		}
		return m.completeHelmList(vers, false)
	case "values":
		return m.completeHelmPath()
	}
	return m, nil, false
}

func (m model) completeHelmList(cands []string, _ bool) (tea.Model, tea.Cmd, bool) {
	d := m.dlg
	typed := strings.TrimSpace(d.hfields[d.idx].input.Value())
	if len(d.cands) > 1 && d.candBase == typed || (d.candBase != "" && strings.HasPrefix(typed, d.candBase) && len(d.cands) > 1) {
		d.candAt = (d.candAt + 1) % len(d.cands)
		d.hfields[d.idx].input.SetValue(d.cands[d.candAt])
		d.hfields[d.idx].input.CursorEnd()
		return m, m.liveHelmVersions(), true
	}
	var hit []string
	for _, c := range cands {
		if typed == "" || strings.HasPrefix(c, typed) || strings.Contains(c, typed) {
			hit = append(hit, c)
		}
	}
	if len(hit) == 0 {
		d.msg = "no match"
		return m, nil, true
	}
	if typed != "" {
		for _, c := range hit {
			if c == typed {
				d.cands = nil
				d.hfields[d.idx].input.SetValue(c)
				d.hfields[d.idx].input.CursorEnd()
				return m, m.liveHelmVersions(), true
			}
		}
	}
	if len(hit) == 1 {
		d.cands = nil
		d.hfields[d.idx].input.SetValue(hit[0])
		d.hfields[d.idx].input.CursorEnd()
		return m, m.liveHelmVersions(), true
	}
	d.cands, d.candAt, d.candBase = hit, -1, typed
	return m, nil, true
}

func (m model) completeHelmPath() (tea.Model, tea.Cmd, bool) {
	d := m.dlg
	value, cands := completeFile(d.hfields[d.idx].input.Value())
	switch {
	case value == "" && len(cands) == 0:
		d.msg = "no matching path"
	case len(cands) > 1:
		d.cands, d.candAt, d.candBase = cands, -1, splitBase(value)
		d.hfields[d.idx].input.SetValue(value)
	default:
		d.cands = nil
		d.hfields[d.idx].input.SetValue(value)
	}
	d.hfields[d.idx].input.CursorEnd()
	return m, nil, true
}

func completeFile(typed string) (value string, cands []string) {
	home, _ := os.UserHomeDir()
	dirPart := splitBase(typed)
	prefix := typed[len(dirPart):]
	dir := expandPath(dirPart, home)
	if dirPart == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		} else {
			dir = home
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		if e.IsDir() {
			cands = append(cands, name+"/")
		} else {
			cands = append(cands, name)
		}
	}
	switch len(cands) {
	case 0:
		return "", nil
	case 1:
		return dirPart + cands[0], nil
	}
	plain := make([]string, len(cands))
	for i, c := range cands {
		plain[i] = strings.TrimSuffix(c, "/")
	}
	return dirPart + commonPrefix(plain), cands
}

func (m model) editHelmValues(defaults bool) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("edit"); blocked {
		return next, cmd
	}
	m.stashHelmForm()
	op := m.vs.hop
	if op == nil {
		return m, nil
	}
	path := strings.TrimSpace(op.values)
	if defaults {
		bin, next, cmd, ok := m.needHelmCLI("chart values")
		if !ok {
			return next, cmd
		}
		if op.chart == "" {
			m.dlg.msg = "set the chart first"
			return m, nil
		}
		text, err := m.k.HelmOut(bin, kube.HelmShowValuesArgs(op.chart, op.version)...)
		if err != nil {
			m.dlg.msg = err.Error()
			return m, m.notify(toneErr, "helm show values failed")
		}
		if path == "" {
			dir, err := os.MkdirTemp("", "k2-helm-")
			if err != nil {
				m.dlg.msg = err.Error()
				return m, nil
			}
			name := op.name
			if name == "" {
				name = "chart"
			}
			path = filepath.Join(dir, name+"-values.yaml")
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			m.dlg.msg = err.Error()
			return m, nil
		}
		op.values = path
		setHelmField(m.dlg, "values", path)
	}
	if path == "" {
		dir, err := os.MkdirTemp("", "k2-helm-")
		if err != nil {
			m.dlg.msg = err.Error()
			return m, nil
		}
		name := op.name
		if name == "" {
			name = "chart"
		}
		path = filepath.Join(dir, name+"-values.yaml")
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			m.dlg.msg = err.Error()
			return m, nil
		}
		op.values = path
		setHelmField(m.dlg, "values", path)
	}
	path = expandPath(path, "")
	op.values = path
	m.vs.hop = op
	setHelmField(m.dlg, "values", path)
	m.status = "opening vim…"
	return m.runEditor("helmvalues", "vim "+filepath.Base(path), filepath.Base(path), "",
		func() *exec.Cmd { return vimCommand(path) },
		func(error) tea.Msg { return helmValuesDoneMsg{} })
}

type helmValuesDoneMsg struct{}

func (m model) submitHelmForm() (tea.Model, tea.Cmd) {
	if m.dlg != nil && m.dlg.idx >= 0 && m.dlg.idx < len(m.dlg.hfields) {
		switch m.dlg.hfields[m.dlg.idx].key {
		case "editvals":
			return m.editHelmValues(false)
		case "defaults":
			return m.editHelmValues(true)
		}
	}
	if next, cmd, blocked := m.readOnlyBlock(m.helmAct()); blocked {
		return next, cmd
	}
	m.maybeFillCatalog()
	m.stashHelmForm()
	op := m.vs.hop
	if op == nil {
		return m, nil
	}
	switch {
	case op.name == "":
		m.dlg.msg = "release name is empty"
		return m, nil
	case op.chart == "":
		m.dlg.msg = "chart is empty"
		return m, nil
	case op.ns == "":
		m.dlg.msg = "namespace is empty"
		return m, nil
	}
	if op.values != "" {
		p := expandPath(op.values, "")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			m.dlg.msg = "values file not found: " + config.HomePath(p)
			return m, nil
		}
		op.values = p
		m.vs.hop = op
	}
	steps := m.helmSteps(op)
	if err := m.ensureHelmRepos(steps); err != nil {
		m.dlg.msg = err.Error()
		return m, m.notify(toneErr, "helm repo add failed")
	}
	bin := m.cfg.Helm.Binary
	m.dlg = nil
	spin := m.startLoad("rendering "+op.act, false)
	k, kc, ctx, pane, token := m.k, m.kubeconfig, m.context, m.logID, m.load.token
	formValues := op.values
	cur := op.current
	act := op.act
	return m, tea.Batch(spin, func() tea.Msg {
		var parts, summaries []string
		var last string
		for _, step := range steps {
			values := formValues
			if values == "" && len(step.Values) > 0 {
				path, err := config.WriteValuesFile(step.Values)
				if err != nil {
					return helmTplMsg{pane: pane, token: token, err: err}
				}
				values = path
			}
			args := kube.HelmTemplateArgs(step.HelmRelease(), step.HelmChart(), step.Version, step.Namespace, values, kc, ctx)
			out, err := k.HelmOut(bin, args...)
			if err != nil {
				return helmTplMsg{pane: pane, token: token, err: err}
			}
			last = out
			parts = append(parts, "# "+step.HelmRelease()+" "+step.HelmChart()+"\n"+out)
			summaries = append(summaries, step.HelmRelease()+": "+kube.ManifestKindCounts(out))
		}
		text := strings.Join(parts, "\n")
		summary := kube.ManifestKindCounts(last)
		if len(summaries) > 1 {
			summary = strings.Join(summaries, " · ")
		}
		if act == "upgrade" && len(steps) == 1 {
			text = kube.UpgradeDiff(cur, last, steps[0].HelmChart(), steps[0].Version)
		}
		return helmTplMsg{pane: pane, token: token, text: text, summary: summary}
	})
}

func (m model) helmAct() string {
	if m.vs.hop != nil {
		return m.vs.hop.act
	}
	return "install"
}

func (m *model) applyHelmTpl(msg helmTplMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if msg.token != 0 && m.load.token != msg.token {
			return
		}
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			cmd = tea.Batch(m.notify(toneErr, m.helmAct()+" preview failed"), m.openErrCmd())
			m.reopenHelmForm()
			return
		}
		if m.vs.hop != nil {
			m.vs.hop.preview = msg.text
			m.vs.hop.summary = msg.summary
		}
		m.subject = m.helmPreviewTitle()
		m.vs.vback = "helminstall"
		m.logText = m.helmPreviewText(msg.summary, msg.text)
		m.logStick, m.logAt = false, 0
		m.stage = "htpl"
		m.status = m.subject + " · " + m.cfg.Keys.Select.Label() + " confirm · " + m.cfg.Keys.Back.Label() + " back"
	})
	return cmd
}

func (m model) openErrCmd() tea.Cmd {
	return func() tea.Msg { return helmErrPopupMsg{} }
}

type helmErrPopupMsg struct{}

func (m model) helmPreviewTitle() string {
	op := m.vs.hop
	if op == nil {
		return "helm preview"
	}
	return op.act + " " + op.name + " · " + kube.ChartAt(op.chart, op.version)
}

func (m model) helmPreviewText(summary, body string) string {
	op := m.vs.hop
	head := "# " + summary + "\n"
	if op != nil {
		head = fmt.Sprintf("# %s %s  %s\n# namespace %s  context %s\n# %s\n\n",
			op.act, op.name, kube.ChartAt(op.chart, op.version), or(op.ns, "-"), m.context, summary)
		if op.values != "" {
			head = fmt.Sprintf("# %s %s  %s\n# namespace %s  values %s  context %s\n# %s\n\n",
				op.act, op.name, kube.ChartAt(op.chart, op.version), or(op.ns, "-"), config.HomePath(op.values), m.context, summary)
		}
	}
	return head + body
}

func (m *model) reopenHelmForm() {
	if m.vs.hop == nil {
		return
	}
	op := *m.vs.hop
	title := "✚ helm install"
	if op.act == "upgrade" {
		title = "↑ helm upgrade"
	}
	d := m.newDialog("helminstall", title, "helm", op.ns, op.name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillHelmForm(d, &op)
	m.dlg = d
	m.stage = "helm"
}

func (m model) askHelmApply() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock(m.helmAct()); blocked {
		return next, cmd
	}
	op := m.vs.hop
	if op == nil {
		return m, nil
	}
	bin, next, cmd, ok := m.needHelmCLI(op.act)
	if !ok {
		return next, cmd
	}
	steps := m.helmSteps(op)
	what := op.act + "ed " + op.name
	if op.act == "install" {
		what = "installed " + op.name
	} else {
		what = "upgraded " + op.name
	}
	details := []string{
		"release " + op.ns + "/" + op.name,
		"chart " + kube.ChartAt(op.chart, op.version),
		m.contextLine(),
	}
	if len(steps) > 1 {
		var names []string
		for _, s := range steps {
			names = append(names, s.HelmRelease())
		}
		details[0] = "releases " + strings.Join(names, ", ")
		details[1] = "charts " + strings.Join(names, " then ")
	}
	if op.values != "" {
		details = append(details, "values "+config.HomePath(op.values))
	}
	if op.summary != "" {
		details = append(details, op.summary)
	}
	details = append(details, bin+" "+map[bool]string{true: "upgrade", false: "install"}[op.act == "upgrade"]+" "+op.name)
	title := "✚ helm install"
	question := "install " + op.name + " into " + op.ns + "?"
	running := "installing…"
	if op.act == "upgrade" {
		title = "↑ helm upgrade"
		question = "upgrade " + op.name + " in " + op.ns + "?"
		running = "upgrading…"
	}
	return m.ask(askSpec{
		title: title, question: question, details: details, danger: true,
		kind: "helm", ns: op.ns, name: op.name, running: running,
		cmd: m.helmApplySteps(what, *op, bin, steps),
	}), nil
}

func (m model) helmApplyAction(what, ns, name, bin string, args []string) tea.Cmd {
	run := m.run
	return func() tea.Msg {
		out, err := shell.Combined(run, bin, args...)
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return editDoneMsg{what: what, err: fmt.Errorf("%s", text), helmNS: ns, helmName: name}
		}
		return editDoneMsg{what: what, helmNS: ns, helmName: name, helmNotes: helmNotesFrom(string(out))}
	}
}

func (m model) helmApplySteps(what string, op helmOp, bin string, steps []config.Chart) tea.Cmd {
	k, run, kc, ctx := m.k, m.run, m.kubeconfig, m.context
	formValues := op.values
	docker := m.dockerCLI()
	upgrade := op.act == "upgrade" && len(steps) == 1
	return func() tea.Msg {
		if err := ensureRepos(k, bin, steps, func(string) {}); err != nil {
			return editDoneMsg{what: what, err: err, helmNS: op.ns, helmName: op.name}
		}
		var notes, extra string
		var last config.Chart
		for _, step := range steps {
			last = step
			values := formValues
			if values == "" && len(step.Values) > 0 {
				path, err := config.WriteValuesFile(step.Values)
				if err != nil {
					return editDoneMsg{what: what, err: err, helmNS: step.Namespace, helmName: step.HelmRelease()}
				}
				values = path
			}
			args := kube.HelmApplyArgs(upgrade, step.HelmRelease(), step.HelmChart(), step.Version, step.Namespace, step.CreatesNamespace(), values, kc, ctx)
			out, err := k.HelmOut(bin, args...)
			if err != nil {
				text := strings.TrimSpace(out)
				if text == "" {
					text = err.Error()
				}
				return editDoneMsg{what: what, err: fmt.Errorf("%s", text), helmNS: step.Namespace, helmName: step.HelmRelease()}
			}
			notes = helmNotesFrom(out)
			if (strings.EqualFold(step.Name, "metallb") || strings.EqualFold(step.HelmRelease(), "metallb") || strings.EqualFold(step.Chart, "metallb")) && strings.HasPrefix(ctx, "kind-") {
				n, w := setupMetalLB(run, docker, kc, ctx, step.Namespace, func(string) {})
				if n != "" {
					extra = strings.TrimSpace(extra + "\n" + n)
				}
				if w != "" {
					extra = strings.TrimSpace(extra + "\n" + w)
				}
			}
		}
		if extra != "" {
			notes = strings.TrimSpace(notes + "\n\n" + extra)
		}
		ns, name := op.ns, op.name
		if last.HelmRelease() != "" {
			ns, name = last.Namespace, last.HelmRelease()
		}
		return editDoneMsg{what: what, helmNS: ns, helmName: name, helmNotes: notes}
	}
}

func helmNotesFrom(out string) string {
	if i := strings.Index(out, "NOTES:"); i >= 0 {
		return strings.TrimSpace(out[i+6:])
	}
	return strings.TrimSpace(out)
}

func parseHelmCmd(rest string) (helmOp, bool, string) {
	fields := splitHelmArgs(rest)
	if len(fields) == 0 {
		return helmOp{}, false, ""
	}
	act := strings.ToLower(fields[0])
	if act != "install" && act != "upgrade" {
		return helmOp{}, false, ""
	}
	op := helmOp{act: act}
	var pos []string
	for i := 1; i < len(fields); i++ {
		a := fields[i]
		take := func() string {
			if i+1 < len(fields) {
				i++
				return fields[i]
			}
			return ""
		}
		switch {
		case a == "-n" || a == "--namespace":
			op.ns = take()
		case strings.HasPrefix(a, "--namespace="):
			op.ns = strings.TrimPrefix(a, "--namespace=")
		case a == "-f" || a == "--values":
			op.values = take()
		case strings.HasPrefix(a, "--values="):
			op.values = strings.TrimPrefix(a, "--values=")
		case a == "--version":
			op.version = take()
		case strings.HasPrefix(a, "--version="):
			op.version = strings.TrimPrefix(a, "--version=")
		case a == "--create-namespace":
			op.createNS = true
		case strings.HasPrefix(a, "-"):
			return helmOp{}, false, "unknown flag: " + a
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 0 {
		op.name = pos[0]
	}
	if len(pos) > 1 {
		op.chart = pos[1]
	}
	if act == "install" && (op.name == "" || op.chart == "") {
		return op, false, "helm install NAME CHART [-n NS] [--create-namespace] [-f FILE] [--version V]"
	}
	if act == "upgrade" && op.name == "" {
		return op, false, "helm upgrade NAME [CHART] [-n NS] [-f FILE] [--version V]"
	}
	return op, true, ""
}

func splitHelmArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quote := rune(0)
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case quote == 0 && r == ' ' || r == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (d *dialog) helmFormRows(inner int) []string {
	nameW := 0
	for _, f := range d.hfields {
		nameW = max(nameW, len(f.label))
	}
	cap := max(8, inner/3)
	if d.act == "kindcreate" || d.act == "kindcharts" {
		cap = max(cap, inner-20)
	}
	nameW = min(nameW, cap)
	var out []string
	for i, f := range d.hfields {
		mark, st := "  ", dimStyle
		if i == d.idx {
			mark, st = "▸ ", keyStyle
		}
		label := st.Render(mark + fitWidth(f.label, nameW) + strings.Repeat(" ", max(0, nameW-len(f.label))) + " ")
		if f.toggle {
			val := "no"
			on := f.on
			if f.key == "create" {
				on = d.hcreate
			}
			if on {
				val = "yes"
			}
			if i == d.idx {
				out = append(out, label+itemStyle.Render(val)+dimStyle.Render("  space toggles"))
			} else {
				out = append(out, label+itemStyle.Render(val))
			}
			continue
		}
		if f.action {
			hint := "enter"
			if f.key == "defaults" {
				hint = "enter · helm show values"
			} else {
				hint = "enter · vim"
			}
			if i == d.idx {
				out = append(out, label+itemStyle.Render(hint))
			} else {
				out = append(out, label+dimStyle.Render(hint))
			}
			continue
		}
		in := f.input
		avail := max(8, inner-ansi.StringWidth(ansi.Strip(label)))
		valW := max(1, avail-ansi.StringWidth(in.Prompt))
		in.Width = valW
		shown := in.Value()
		if f.key == "values" && ansi.StringWidth(shown) > valW {
			shown = fitMiddle(shown, valW)
		} else if ansi.StringWidth(shown) > avail {
			shown = fitMiddle(shown, avail)
		}
		if f.locked {
			out = append(out, label+dimStyle.Render(shown))
			continue
		}
		if i != d.idx && shown != in.Value() {
			out = append(out, label+itemStyle.Render(in.Prompt+shown))
			continue
		}
		out = append(out, label+in.View())
		if f.key == "chart" && i == d.idx {
			out = append(out, d.helmChartSuggestRows(inner, ansi.StringWidth(ansi.Strip(label)))...)
		}
	}
	if len(d.cands) > 0 && !(d.idx >= 0 && d.idx < len(d.hfields) && d.hfields[d.idx].key == "chart") {
		var parts []string
		for i, c := range d.cands {
			if i == maxCands {
				parts = append(parts, dimStyle.Render("…"))
				break
			}
			if i == d.candAt {
				parts = append(parts, selectedItemStyle.Render(c))
			} else {
				parts = append(parts, itemStyle.Render(c))
			}
		}
		out = append(out, strings.Join(parts, "  "))
	}
	return out
}

func (m *model) applyHelmDone(msg editDoneMsg) {
	m.stage = "helm"
	if msg.helmNS != "" && m.namespace != msg.helmNS && !m.vs.helmAll {
		m.namespace = msg.helmNS
	}
	m.helmRefreshNow()
	pick := msg.helmName
	if m.helmNS() == "" && msg.helmNS != "" {
		pick = msg.helmNS + "/" + msg.helmName
	}
	m.showHelm(pick)
	notes := msg.helmNotes
	status := msg.what
	ref := msg.helmNS + "/" + msg.helmName
	if r, ok := m.selectedRelease(); ok {
		if notes == "" {
			notes = r.Notes
		}
		status = msg.what + " · " + or(r.Status, "deployed")
		ref = r.Namespace + "/" + r.Name
	}
	m.vs.hop = nil
	m.status = status
	if notes != "" {
		next := m.helmText("view", "notes "+ref, notes)
		*m = next
		m.status = status + " · notes"
	}
}

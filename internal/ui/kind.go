package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

type kindOp struct {
	name, config, image, kubeconfig, form, valuesChart string
	planes, workers                                    int
	charts                                             []string
	edited, autoConfig, cache, cacheSet                bool
	uninstall, upgrade                                 []string
	values                                             map[string]string
	masterKeys                                         map[string]string
	namespaces                                         map[string]string
	installed                                          []string
	deployed                                           map[string]string
	releases                                           map[string]kindRelease
}

type kindRelease struct{ name, namespace string }

var fetchKindNodeImages = func() ([]string, error) {
	return kube.FetchKindNodeImages(nil, "")
}

type kindJob struct {
	name, status, step, last, log string
	started                       time.Time
	err                           error
	op                            kindOp
	create                        bool
}

type kindListMsg struct {
	pane        string
	token       int
	names       []string
	nodes       map[string]kube.KindNodes
	kubeconfigs map[string]string
	err         error
}

type kindRunMsg struct {
	pane string
	text string
	err  error
	op   kindOp
}

type kindChartsMsg struct {
	pane, name, text, notes string
	err                     error
}

type kindDeleteMsg struct {
	pane, name, text string
	err              error
	gone             bool
}

var metalRetryWait = 2 * time.Second
var metalRetryN = 8

func (m model) kindCLI() string {
	if m.cfg.Kind.Binary == "" {
		return ""
	}
	if _, err := lookPath(m.cfg.Kind.Binary); err != nil {
		return ""
	}
	return m.cfg.Kind.Binary
}

func (m model) dockerCLI() string {
	if _, err := lookPath("docker"); err != nil {
		return ""
	}
	return "docker"
}

func (m model) needKindCLI(act string) (string, tea.Model, tea.Cmd, bool) {
	bin := m.kindCLI()
	if bin != "" {
		return bin, m, nil, true
	}
	why := "kind.binary is empty"
	if m.cfg.Kind.Binary != "" {
		why = m.cfg.Kind.Binary + " is not in PATH"
	}
	m.status = act + " needs the kind CLI (" + why + ")"
	return "", m, m.notify(toneWarn, act+" needs the kind CLI"), false
}

func (m model) needDockerCLI(act string) (string, tea.Model, tea.Cmd, bool) {
	bin := m.dockerCLI()
	if bin != "" {
		return bin, m, nil, true
	}
	m.status = act + " needs docker in PATH"
	return "", m, m.notify(toneWarn, m.status), false
}

func (m model) openKind() (tea.Model, tea.Cmd) {
	if m.stage == "kindlog" {
		m.stopKindLiveLog()
	}
	if m.stage != "kind" && m.stage != "kindlog" {
		m.rememberView()
	}
	m.stage = "kind"
	m.restoreKindSnap()
	m.showKind("")
	_, next, cmd, ok := m.needKindCLI("kind")
	if !ok {
		nm := next.(model)
		nm.stage = "kind"
		nm.restoreKindSnap()
		nm.showKind("")
		return nm, tea.Batch(cmd, nm.kindProxyCmd())
	}
	if m.status == "" || strings.HasPrefix(m.status, "no kind clusters") {
		if len(m.kindTiles()) > 0 {
			m.status = m.kindHint()
		}
	}
	if m.status == "" {
		m.status = m.kindHint()
	}
	return m, m.kindRefreshCmd()
}

func kindListCmd(bin, docker string, run shell.Runner, pane string, token int) tea.Cmd {
	return func() tea.Msg {
		out, err := shell.Combined(run, bin, kube.KindGetArgs()...)
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return kindListMsg{pane: pane, token: token, err: fmt.Errorf("%s", text)}
		}
		msg := kindListMsg{pane: pane, token: token, names: kube.ParseKindClusters(string(out))}
		if docker != "" {
			nout, nerr := shell.Combined(run, docker, kube.DockerKindNodesArgs()...)
			if nerr == nil {
				msg.nodes = kube.ParseKindNodesAt(string(nout), now())
			}
		}
		msg.kubeconfigs = kindKubeconfigMap(msg.names, run)
		return msg
	}
}

func (m *model) applyKindList(msg kindListMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if m.load.token != msg.token && msg.token != 0 {
			return
		}
		m.load = load{}
		if m.stage != "kind" && m.stage != "kindlog" {
			m.vs.sortCol, m.vs.sortDesc = "", false
			m.content.ResetFilter()
			m.content.SetItems(nil)
		}
		if m.stage != "kindlog" {
			m.stage = "kind"
		}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			m.restoreKindSnap()
			m.showKind("")
			cmd = m.notify(toneErr, "could not list kind clusters")
			return
		}
		m.vs.kindNames = msg.names
		if msg.nodes != nil {
			kube.KeepKindStartedAts(m.vs.kindNodes, msg.nodes, now())
			m.vs.kindNodes = msg.nodes
		}
		if msg.kubeconfigs != nil {
			if m.kindSnap.kc == nil {
				m.kindSnap.kc = map[string]string{}
			}
			for name, path := range msg.kubeconfigs {
				m.kindSnap.kc[name] = path
			}
			for name := range m.kindSnap.kc {
				keep := false
				for _, n := range msg.names {
					if n == name {
						keep = true
						break
					}
				}
				if !keep {
					delete(m.kindSnap.kc, name)
				}
			}
		}
		m.storeKindSnap()
		m.showKind("")
		if len(m.kindTiles()) == 0 {
			m.status = "no kind clusters — " + m.cfg.Keys.HelmInstall.Label() + " creates one"
			return
		}
		if m.stage == "kind" {
			m.status = m.kindHint()
		}
		if m.kindExtrasDue() {
			cmd = tea.Batch(cmd, m.kindExtrasCmds())
		}
	})
	return cmd
}

func (m model) kindHint() string {
	k := m.cfg.Keys
	return "space use, " + k.Select.Label() + " " + m.kindEnterHint() + ", " + k.HelmInstall.Label() + " create, " + k.Edit.Label() + " charts, " + k.KindToggle.Label() + " " + m.kindToggleHint() + ", " + k.Delete.Label() + " delete, " + k.KindProxy.Label() + " registry proxy"
}

func (m *model) showKind(pick string) {
	if pick != "" {
		m.selectKind(pick)
	}
	n := m.kindCardCount()
	if m.vs.kindSel < 0 {
		m.vs.kindSel = 0
	}
	if n == 0 {
		m.vs.kindSel = 0
		return
	}
	if m.vs.kindSel >= n {
		m.vs.kindSel = n - 1
	}
	m.ensureKindVisible()
}

func (m model) kindKubeconfigFor(name string) (string, bool) {
	home, _ := os.UserHomeDir()
	path := kube.KindKubeconfig(name, home)
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path, true
	}
	if path, ok := m.kindSnap.kc[name]; ok && path != "" {
		return path, true
	}
	return "", false
}

func kindKubeconfigMap(names []string, run shell.Runner) map[string]string {
	home, _ := os.UserHomeDir()
	out := map[string]string{}
	var missing []string
	for _, name := range names {
		if name == "" {
			continue
		}
		path := kube.KindKubeconfig(name, home)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			out[name] = path
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) == 0 {
		return out
	}
	def := filepath.Join(home, ".kube", "config")
	if st, err := os.Stat(def); err != nil || st.IsDir() {
		return out
	}
	raw, err := shell.Combined(run, "kubectl", "config", "get-contexts", "-o", "name", "--kubeconfig", def)
	if err != nil {
		return out
	}
	have := map[string]bool{}
	for _, c := range strings.Split(string(raw), "\n") {
		c = strings.TrimSpace(c)
		if c != "" {
			have[c] = true
		}
	}
	for _, name := range missing {
		if have[kube.KindContext(name)] {
			out[name] = def
		}
	}
	return out
}

func (m model) kindKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	if km.KindProxy.Has(key) {
		next, cmd := m.askKindProxyStart()
		return next, cmd, true
	}
	if km.KindToggle.Has(key) {
		next, cmd := m.kindToggleKey()
		return next, cmd, true
	}
	if m.kindProxySelected() {
		if next, cmd, ok := m.kindProxyKey(key); ok {
			return next, cmd, true
		}
	}
	switch {
	case key == "tab":
		m.moveKind(1)
		return m, nil, true
	case key == "shift+tab":
		m.moveKind(-1)
		return m, nil, true
	case km.Left.Has(key) || key == "left" || key == "h":
		m.moveKind(-1)
		return m, nil, true
	case key == "right":
		m.moveKind(1)
		return m, nil, true
	case key == "l" && !km.Submit.Has(key):
		m.moveKind(1)
		return m, nil, true
	case km.ScrollDown.Has(key):
		m.moveKind(1)
		return m, nil, true
	case km.ScrollUp.Has(key):
		m.moveKind(-1)
		return m, nil, true
	case km.HelmInstall.Has(key) || km.Apply.Has(key):
		next, cmd := m.openKindCreate(kindOp{name: m.freeKindName("k2"), planes: 1})
		return next, cmd, true
	case km.Edit.Has(key):
		name, ok := m.selectedKind()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.openKindCharts(name)
		return next, cmd, true
	case km.Delete.Has(key):
		name, ok := m.selectedKind()
		if !ok {
			return m, nil, true
		}
		if j := m.kindJobs[name]; j != nil && (j.status == "deploying" || j.status == "deleting") {
			m.status = name + " is still " + j.status
			return m, m.notify(toneWarn, m.status), true
		}
		next, cmd := m.askKindDelete(name)
		return next, cmd, true
	case key == " ":
		next, cmd := m.useKindTile()
		return next, cmd, true
	case km.Select.Has(key) && key != "l" && key != "right":
		next, cmd := m.enterKindTile()
		return next, cmd, true
	case km.Watch.Has(key):
		m.clearStaleKindJobs()
		m.kindExtrasForce = true
		next, cmd := m.openKind()
		nm := next.(model)
		return nm, tea.Batch(cmd, nm.kindExtrasCmds()), true
	}
	return m, nil, false
}

func (m model) useKindCluster(name string) (tea.Model, tea.Cmd) {
	path, ok := m.kindKubeconfigFor(name)
	if !ok {
		m.status = "no kubeconfig for " + name
		return m, m.notify(toneWarn, m.status)
	}
	m.kubeconfig = path
	m.context = kube.KindContext(name)
	m.namespace = m.k.NamespaceOf(path, m.context)
	m.clearHist()
	m.save()
	m.showKind(name)
	m.status = "using " + m.context + " · " + config.HomePath(path)
	return m, fetchResources(m.k, m.kubeconfig, m.context)
}

func (m model) openKindCreate(op kindOp) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("create cluster"); blocked {
		return next, cmd
	}
	_, next, cmd, ok := m.needKindCLI("create cluster")
	if !ok {
		return next, cmd
	}
	if _, next, cmd, ok = m.needDockerCLI("create cluster"); !ok {
		return next, cmd
	}
	if op.name == "" {
		op.name = "k2"
	}
	if op.planes < 1 {
		op.planes = 1
	}
	if op.workers < 0 {
		op.workers = 0
	}
	if op.form == "" {
		op.form = "kindcreate"
	}
	if !op.cacheSet {
		op.cache, op.cacheSet = m.kindProxy.Running(), true
	}
	if op.image == "" && len(m.kindNodeImages) > 0 {
		op.image = m.kindNodeImages[0]
	}
	if op.config == "" {
		if err := m.writeKindGeneratedConfig(&op); err != nil {
			m.status = err.Error()
		}
	}
	title := "✚ kind create"
	d := m.newDialog("kindcreate", title, "kind", "", op.name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillKindForm(d, &op)
	m.vs.kop = &op
	m.dlg = d
	images := m.kindNodeImagesCmd()
	return m, images
}

func (m *model) kindNodeImagesCmd() tea.Cmd {
	if len(m.kindNodeImages) > 0 || m.kindNodeImagesBusy {
		return nil
	}
	m.kindNodeImagesBusy = true
	return func() tea.Msg {
		tags, _ := fetchKindNodeImages()
		return kindNodeImagesMsg{images: tags}
	}
}

func (m *model) applyKindNodeImages(msg kindNodeImagesMsg) {
	m.kindNodeImagesBusy = false
	if len(msg.images) == 0 {
		return
	}
	m.kindNodeImages = msg.images
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		return
	}
	if helmFieldValue(m.dlg, "image") != "" {
		return
	}
	setHelmField(m.dlg, "image", msg.images[0])
	if m.vs.kop != nil {
		m.vs.kop.image = msg.images[0]
	}
	m.refreshKindGeneratedConfig()
	m.syncKindImageSuggest()
}

func (m *model) fillKindForm(d *dialog, op *kindOp) {
	m.ensureKindMasterKeys(op)
	cfgHint := "path to kind config"
	if op.edited && op.config != "" {
		cfgHint = "(edited)"
	}
	fields := []helmField{
		{key: "name", label: "name", input: helmFieldInput(op.name, "k2")},
		{key: "planes", label: "control-planes", input: helmFieldInput(strconv.Itoa(max(1, op.planes)), "1")},
		{key: "workers", label: "workers", input: helmFieldInput(strconv.Itoa(max(0, op.workers)), "0")},
		{key: "config", label: "config", input: helmFieldInput(op.config, cfgHint)},
		{key: "image", label: "image", input: helmFieldInput(op.image, "kindest/node:…")},
		{key: "cache", label: "registry cache", input: helmFieldInput("", ""), toggle: true, on: op.cache},
	}
	fields = append(fields, m.kindChartFields(op)...)
	idx := 0
	if op.valuesChart != "" {
		idx = kindFocusField(fields, "c:"+op.valuesChart)
	}
	if len(fields) > 0 {
		helmFocus(&fields[idx])
	}
	d.hfields = fields
	d.idx = idx
}

func (m model) kindChartFields(op *kindOp) []helmField {
	selected := map[string]bool{}
	for _, n := range op.charts {
		selected[n] = true
	}
	var fields []helmField
	for _, c := range m.cfg.Charts() {
		label := c.Name
		upgrading := selected[c.Name] && op.isInstalled(c.Name) && op.chartUpgrade(c.Name)
		if op.values != nil && op.values[c.Name] != "" && (!op.isInstalled(c.Name) || upgrading) {
			label += " (values)"
		}
		note := ""
		if path := strings.TrimSpace(m.chartMasterKey(c.Name, op.masterKeys)); path != "" {
			note = config.HomePath(path)
		}
		fields = append(fields, helmField{
			key:    "c:" + c.Name,
			label:  label,
			input:  helmFieldInput("", ""),
			toggle: true,
			on:     selected[c.Name],
			note:   note,
			tag:    kindUpgradeTag(upgrading),
		})
		if selected[c.Name] {
			fields = append(fields, m.kindNsField(op, c, op.isInstalled(c.Name)))
		}
	}
	return fields
}

func kindFocusField(fields []helmField, key string) int {
	for i, f := range fields {
		if f.key == key {
			return i
		}
	}
	return 0
}

func (m *model) fillKindChartsForm(d *dialog, op *kindOp) {
	m.ensureKindMasterKeys(op)
	fields := m.kindChartFields(op)
	idx := 0
	if op.valuesChart != "" {
		idx = kindFocusField(fields, "c:"+op.valuesChart)
	}
	if len(fields) > 0 {
		helmFocus(&fields[idx])
	}
	d.hfields, d.idx = fields, idx
}

func (m model) kindFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km, key := m.cfg.Keys, msg.String()
	d := m.dlg
	if d == nil || len(d.hfields) == 0 {
		return m, nil
	}
	if d.idx < 0 || d.idx >= len(d.hfields) {
		d.idx = 0
	}
	switch {
	case key == "tab" || key == "down":
		if key == "tab" && d.hfields[d.idx].key == "image" {
			return m.cycleKindImageList(1)
		}
		if key == "tab" && d.hfields[d.idx].key == "config" {
			if next, cmd, done := m.completeKindPath(); done {
				return next, cmd
			}
		}
		return m.moveHelmField(1), nil
	case key == "shift+tab" || key == "up":
		if key == "shift+tab" && d.hfields[d.idx].key == "image" {
			return m.cycleKindImageList(-1)
		}
		return m.moveHelmField(-1), nil
	case key == "ctrl+e" && d.hfields[d.idx].key == "config":
		return m.editKindConfig()
	case km.HelmValues.Has(key) && strings.HasPrefix(d.hfields[d.idx].key, "c:"):
		return m.editKindChartValues()
	case key == "m" && kindFieldIsSealedSecrets(d.hfields[d.idx].key):
		return m.openKindMasterKeyPicker()
	case (key == " " || key == "c") && d.hfields[d.idx].toggle:
		d.hfields[d.idx].on = !d.hfields[d.idx].on
		if strings.HasPrefix(d.hfields[d.idx].key, "c:") {
			m.syncKindNsRows()
		}
		return m, nil
	}
	if d.hfields[d.idx].toggle || d.hfields[d.idx].action || d.hfields[d.idx].locked {
		return m, nil
	}
	var cmd tea.Cmd
	d.hfields[d.idx].input, cmd = d.hfields[d.idx].input.Update(msg)
	d.msg = ""
	switch d.hfields[d.idx].key {
	case "image":
		m.refreshKindImageSuggest(true)
		if m.vs.kop != nil {
			m.vs.kop.image = helmFieldValue(d, "image")
			m.refreshKindGeneratedConfig()
		}
		return m, cmd
	case "config":
		if m.vs.kop != nil {
			name := helmFieldValue(d, "name")
			if name == "" {
				name = m.vs.kop.name
			}
			cfg := helmFieldValue(d, "config")
			m.vs.kop.autoConfig = generatedKindConfigPath(cfg, name)
		}
	case "name", "planes", "workers":
		m.refreshKindGeneratedConfig()
	}
	d.cands, d.candBase, d.candAt = nil, "", -1
	return m, cmd
}

func kindConfigPath(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "k2"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return filepath.Join(os.TempDir(), "k2-kind.yaml")
	}
	return filepath.Join(os.TempDir(), "k2-kind-"+b.String()+".yaml")
}

func generatedKindConfigPath(path, name string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return true
	}
	return filepath.Clean(expandPath(path, "")) == filepath.Clean(kindConfigPath(name))
}

func isKindGeneratedFile(path string) bool {
	path = filepath.Clean(expandPath(strings.TrimSpace(path), ""))
	base := filepath.Base(path)
	return filepath.Dir(path) == filepath.Clean(os.TempDir()) && strings.HasPrefix(base, "k2-kind-") && strings.HasSuffix(base, ".yaml")
}

func (m *model) writeKindGeneratedConfig(op *kindOp) error {
	if op == nil {
		return nil
	}
	path := kindConfigPath(op.name)
	src := strings.TrimSpace(op.config)
	if src == "" {
		src = path
	}
	src = expandPath(src, "")
	var text string
	var err error
	if op.edited {
		if raw, rerr := os.ReadFile(src); rerr == nil && len(strings.TrimSpace(string(raw))) > 0 {
			text, err = kube.SyncKindConfigNodes(string(raw), max(1, op.planes), max(0, op.workers), op.image)
		} else {
			text, err = kube.KindConfigYAML(max(1, op.planes), max(0, op.workers), op.image)
		}
	} else {
		text, err = kube.KindConfigYAML(max(1, op.planes), max(0, op.workers), op.image)
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return err
	}
	op.config, op.autoConfig = path, true
	return nil
}

func (m *model) refreshKindGeneratedConfig() {
	if m.dlg == nil || m.vs.kop == nil {
		return
	}
	old := strings.TrimSpace(m.vs.kop.config)
	m.stashKindForm()
	if !m.vs.kop.autoConfig && !generatedKindConfigPath(m.vs.kop.config, m.vs.kop.name) {
		return
	}
	if err := m.writeKindGeneratedConfig(m.vs.kop); err != nil {
		return
	}
	if old != "" && old != m.vs.kop.config && isKindGeneratedFile(old) {
		_ = os.Remove(old)
	}
	setHelmField(m.dlg, "config", m.vs.kop.config)
}

func kindImageKnown(images []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	for _, img := range images {
		if img == value {
			return true
		}
	}
	return false
}

func (m model) kindImageSuggestions(typed string) []string {
	typed = strings.TrimSpace(typed)
	if kindImageKnown(m.kindNodeImages, typed) {
		return append([]string{}, m.kindNodeImages...)
	}
	var out []string
	for _, img := range m.kindNodeImages {
		if strings.Contains(img, typed) {
			out = append(out, img)
		}
	}
	return out
}

func (m *model) syncKindImageSuggest() {
	m.refreshKindImageSuggest(false)
}

func (m *model) refreshKindImageSuggest(fromType bool) {
	if m.dlg == nil || m.dlg.idx < 0 || m.dlg.idx >= len(m.dlg.hfields) || m.dlg.hfields[m.dlg.idx].key != "image" {
		return
	}
	typed := helmFieldValue(m.dlg, "image")
	if fromType && !kindImageKnown(m.kindNodeImages, typed) {
		m.dlg.candBase = typed
		m.dlg.cands = m.kindImageSuggestions(typed)
		m.dlg.candAt = m.helmChartCandIndex(m.dlg.cands, typed)
		return
	}
	m.dlg.candBase = ""
	m.dlg.cands = m.kindImageSuggestions(typed)
	m.dlg.candAt = m.helmChartCandIndex(m.dlg.cands, typed)
}

func (m model) cycleKindImageList(delta int) (tea.Model, tea.Cmd) {
	d := m.dlg
	if d == nil {
		return m, nil
	}
	typed := helmFieldValue(d, "image")
	if len(d.cands) == 0 || (m.helmChartCandIndex(d.cands, typed) < 0 && typed != d.candBase && !kindImageKnown(d.cands, typed)) {
		d.cands = m.kindImageSuggestions(typed)
		if kindImageKnown(m.kindNodeImages, typed) {
			d.candBase = ""
		} else {
			d.candBase = typed
		}
	}
	if len(d.cands) == 0 {
		d.msg = "no match"
		return m, nil
	}
	at := m.helmChartCandIndex(d.cands, typed)
	if at < 0 {
		if delta < 0 {
			at = len(d.cands) - 1
		} else {
			at = 0
		}
	} else {
		at = (at + delta) % len(d.cands)
		if at < 0 {
			at += len(d.cands)
		}
	}
	d.candAt = at
	d.msg = ""
	setHelmField(d, "image", d.cands[at])
	if m.vs.kop != nil {
		m.vs.kop.image = d.cands[at]
	}
	m.refreshKindGeneratedConfig()
	return m, nil
}

func (m model) kindFormFooter(d *dialog) string {
	k := m.cfg.Keys
	key := ""
	if d != nil && d.idx >= 0 && d.idx < len(d.hfields) {
		key = d.hfields[d.idx].key
	}
	foot := keyStyle.Render("↑↓") + dimStyle.Render(" move  ")
	switch {
	case key == "image":
		foot = keyStyle.Render("tab/shift+tab") + dimStyle.Render(" cycle images  ") + foot
	case key == "config":
		foot = keyStyle.Render("ctrl+e") + dimStyle.Render(" edit  ") + keyStyle.Render("tab") + dimStyle.Render(" complete  ") + foot
	case key == "cache":
		foot += keyStyle.Render("space") + dimStyle.Render(" toggle  ")
	case strings.HasPrefix(key, "c:"):
		foot += keyStyle.Render("space") + dimStyle.Render(" toggle  ") + keyStyle.Render(k.HelmValues.Short()) + dimStyle.Render(" values  ")
		if kindFieldIsSealedSecrets(key) {
			foot += keyStyle.Render("m") + dimStyle.Render(" master key  ")
		}
	default:
		foot = keyStyle.Render("tab/↑↓") + dimStyle.Render(" move  ")
	}
	foot += keyStyle.Render(k.Submit.Short()) + dimStyle.Render(" confirm  ") + keyStyle.Render(k.Cancel.Short()) + dimStyle.Render(" cancel")
	return foot
}

func (m model) completeKindPath() (tea.Model, tea.Cmd, bool) {
	return m.completeHelmPath()
}

func (m *model) stashKindForm() {
	if m.dlg == nil || m.vs.kop == nil {
		return
	}
	op := *m.vs.kop
	var charts []string
	for _, f := range m.dlg.hfields {
		switch f.key {
		case "name":
			op.name = strings.TrimSpace(f.input.Value())
		case "planes":
			op.planes, _ = strconv.Atoi(strings.TrimSpace(f.input.Value()))
		case "workers":
			op.workers, _ = strconv.Atoi(strings.TrimSpace(f.input.Value()))
		case "config":
			op.config = strings.TrimSpace(f.input.Value())
		case "image":
			op.image = strings.TrimSpace(f.input.Value())
		case "cache":
			op.cache, op.cacheSet = f.on, true
		default:
			if strings.HasPrefix(f.key, "c:") && f.on {
				charts = append(charts, strings.TrimPrefix(f.key, "c:"))
			}
		}
	}
	op.charts = charts
	m.readKindNamespaces(&op)
	m.vs.kop = &op
}

func (m model) submitKindForm() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("create cluster"); blocked {
		return next, cmd
	}
	m.stashKindForm()
	op := m.vs.kop
	if op == nil {
		return m, nil
	}
	switch {
	case op.name == "":
		m.dlg.msg = "cluster name is empty"
		return m, nil
	case strings.Contains(op.name, " "):
		m.dlg.msg = "cluster name cannot contain spaces"
		return m, nil
	case m.kindClusterExists(op.name):
		m.dlg.msg = fmt.Sprintf("cluster %q already exists", op.name)
		return m, nil
	case op.planes < 1:
		m.dlg.msg = "need at least one control-plane"
		return m, nil
	case op.workers < 0:
		m.dlg.msg = "worker count cannot be negative"
		return m, nil
	}
	if op.autoConfig || generatedKindConfigPath(op.config, op.name) {
		if err := m.writeKindGeneratedConfig(op); err != nil && m.dlg != nil {
			m.dlg.msg = err.Error()
			return m, nil
		}
	}
	if op.config != "" {
		p := expandPath(op.config, "")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			m.dlg.msg = "config file not found: " + config.HomePath(p)
			return m, nil
		}
		op.config = p
		applyKindConfigToOp(op, p)
		m.vs.kop = op
	}
	if len(op.charts) > 0 {
		if _, next, cmd, ok := m.needHelmCLI("starter charts"); !ok {
			return next, cmd
		}
	}
	if kindSelectsSealedSecrets(op.charts) {
		if op.masterKeys != nil {
			if v, ok := op.masterKeys["sealed-secrets"]; ok {
				m.rememberSealedKey(v, m.configuredKindSealedKey())
			}
		}
		if _, msg := m.validateMasterKeyFile(m.kindSealedMasterKey(op)); msg != "" {
			m.dlg.msg = msg
			return m, nil
		}
	}
	home, _ := os.UserHomeDir()
	op.kubeconfig = kube.KindKubeconfig(op.name, home)
	m.vs.kop = op
	m.dlg = nil
	return m.askKindCreate(*op)
}

const kindDetailCol = 14

func kindDetail(label, value string) string {
	return fmt.Sprintf("%-*s %s", kindDetailCol, label, value)
}

func (m model) askKindCreate(op kindOp) (tea.Model, tea.Cmd) {
	topo := strconv.Itoa(op.planes) + " control-plane"
	if op.planes != 1 {
		topo += "s"
	}
	if op.workers > 0 {
		topo += ", " + strconv.Itoa(op.workers) + " worker"
		if op.workers != 1 {
			topo += "s"
		}
	}
	details := []string{
		kindDetail("cluster", op.name),
		kindDetail("context", kube.KindContext(op.name)),
		kindDetail("kubeconfig", config.HomePath(op.kubeconfig)),
	}
	if op.config != "" {
		details = append(details, kindDetail("config", config.HomePath(op.config)))
	}
	details = append(details, kindDetail("nodes", topo))
	if op.image != "" {
		details = append(details, kindDetail("image", op.image))
	}
	if len(op.charts) > 0 {
		details = append(details, kindDetail("charts", kindChartsWithNS(op.charts, op, m.cfg.Charts())))
	}
	if kindSelectsSealedSecrets(op.charts) {
		if path := m.kindSealedMasterKey(&op); path != "" {
			details = append(details, kindDetail("master key", config.HomePath(path)))
		}
	}
	if op.cache {
		cache := "yes (" + m.registryProxy().Container + ")"
		if !m.kindProxy.Running() {
			cache += " · starts first"
		}
		details = append(details, kindDetail("registry cache", cache))
	}
	return m.ask(askSpec{
		title: "✚ kind create", question: "create kind cluster " + op.name + "?", details: details, danger: true,
		kind: "kind", name: op.name, running: "creating cluster " + op.name + "…",
		onBack: func(m model) (tea.Model, tea.Cmd) {
			m.reopenKindForm()
			return m, nil
		},
		onYes: func(m model) (tea.Model, tea.Cmd) {
			if m.kindJobs == nil {
				m.kindJobs = map[string]*kindJob{}
			}
			j := m.ensureKindJob(op.name, "deploying", "creating cluster")
			j.op, j.err, j.log, j.last, j.started, j.create = op, nil, "", "", now(), true
			found := false
			for _, n := range m.vs.kindNames {
				if n == op.name {
					found = true
					break
				}
			}
			if !found {
				m.vs.kindNames = append(m.vs.kindNames, op.name)
			}
			m.stage = "kind"
			m.selectKind(op.name)
			m.status = "deploying " + op.name
			return m, m.kindCreateStream(op)
		},
	}), nil
}

func (m model) kindCreateStream(op kindOp) tea.Cmd {
	bin := m.cfg.Kind.Binary
	run := m.run
	pane := m.logID
	configPath, image := op.config, op.image
	if op.config == "" {
		text, err := kube.KindConfigYAML(op.planes, op.workers, op.image)
		if err != nil {
			return func() tea.Msg { return kindRunMsg{pane: pane, err: err, op: op} }
		}
		f, err := os.CreateTemp("", "k2-kind-*.yaml")
		if err != nil {
			return func() tea.Msg { return kindRunMsg{pane: pane, err: err, op: op} }
		}
		if _, err := f.WriteString(text); err != nil {
			f.Close()
			return func() tea.Msg { return kindRunMsg{pane: pane, err: err, op: op} }
		}
		f.Close()
		configPath = f.Name()
		image = ""
	} else if raw, err := os.ReadFile(configPath); err == nil {
		image = kube.KindCreateImage(string(raw), op.image)
	}
	if err := os.MkdirAll(filepath.Dir(op.kubeconfig), 0o700); err != nil {
		return func() tea.Msg { return kindRunMsg{pane: pane, err: err, op: op} }
	}
	args := kube.KindCreateArgs(op.name, op.kubeconfig, configPath, image)
	return m.kindStream(op.name, run, bin, args, "", func(text string, err error) tea.Msg {
		return kindRunMsg{pane: pane, text: text, err: err, op: op}
	})
}

func (m model) kindStream(name string, run shell.Runner, bin string, args []string, stdin string, done func(string, error) tea.Msg) tea.Cmd {
	return m.kindStreamCtx(name, run, bin, args, stdin, nil, done)
}

func (m model) kindStreamCtx(name string, run shell.Runner, bin string, args []string, stdin string, ctx context.Context, done func(string, error) tea.Msg) tea.Cmd {
	pane := m.logID
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			var buf strings.Builder
			req := shell.Request{Name: bin, Args: args, Stdin: stdin, Combined: true, Stream: true, Ctx: ctx, OnLine: func(line string) {
				if strings.TrimSpace(line) == "" {
					return
				}
				buf.WriteString(line)
				if !strings.HasSuffix(line, "\n") {
					buf.WriteByte('\n')
				}
				ch <- kindLineMsg{pane: pane, name: name, line: line, step: kindStep(line), ch: ch}
			}}
			res := run.Run(req)
			text := strings.TrimSpace(buf.String())
			if text == "" {
				text = strings.TrimSpace(string(res.Stdout) + "\n" + string(res.Stderr))
			}
			var err error
			if res.Err != nil {
				msg := text
				if msg == "" {
					msg = res.Err.Error()
				}
				err = fmt.Errorf("%s", msg)
			}
			ch <- done(text, err)
		}()
		return <-ch
	}
}

func kindCmdOut(run shell.Runner, bin string, args []string, stdin string, emit func(string)) (string, error) {
	var buf strings.Builder
	res := run.Run(shell.Request{Name: bin, Args: args, Stdin: stdin, Combined: true, Stream: true, OnLine: func(line string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		buf.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			buf.WriteByte('\n')
		}
		if emit != nil {
			emit(line)
		}
	}})
	text := strings.TrimSpace(buf.String())
	if text == "" {
		text = strings.TrimSpace(string(res.Stdout) + "\n" + string(res.Stderr))
	}
	if res.Err != nil {
		msg := text
		if msg == "" {
			msg = res.Err.Error()
		}
		return text, fmt.Errorf("%s", msg)
	}
	return text, nil
}

func (m *model) applyKindCreated(msg kindRunMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		j := m.ensureKindJob(msg.op.name, "", "")
		j.op = msg.op
		if msg.err == nil {
			j.create = false
		}
		if msg.err != nil {
			j.status, j.err = "failed", msg.err
			if msg.text != "" {
				j.last = lastNonEmpty(msg.text)
			}
			m.syncKindLog(msg.op.name)
			if m.stage != "kindlog" {
				m.stage = "kind"
				m.selectKind(msg.op.name)
			}
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "kind create failed")
			return
		}
		m.kubeconfig = msg.op.kubeconfig
		m.context = kube.KindContext(msg.op.name)
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
		m.clearHist()
		m.save()
		if msg.op.cache {
			j.status, j.step = "deploying", "wiring registry cache"
			m.status = "wiring registry cache on " + msg.op.name
			cmd = tea.Batch(m.notify(toneOK, "created "+msg.op.name), m.kindWireStep(msg.op))
			return
		}
		cmd = m.finishKindCreate(msg.op)
	})
	return cmd
}

func (m *model) finishKindCreate(op kindOp) tea.Cmd {
	j := m.ensureKindJob(op.name, "", "")
	if len(op.charts) == 0 && len(op.uninstall) == 0 {
		j.status, j.step, j.err = "running", "", nil
		m.showKind(op.name)
		if m.stage == "kindlog" {
			m.syncKindLog(op.name)
			m.status = "cluster " + op.name + " ready"
		} else {
			m.stage = "kind"
			m.status = "created " + op.name
		}
		return tea.Batch(m.notify(toneOK, "created "+op.name), m.kindRefreshCmd(), fetchResources(m.k, m.kubeconfig, m.context))
	}
	j.status, j.step = "deploying", "installing charts"
	return tea.Batch(m.notify(toneOK, "created "+op.name), m.startKindCharts(op, ""))
}

func lastNonEmpty(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return strings.TrimSpace(text)
}

type kindChartWork struct {
	parent  string
	steps   []config.Chart
	upgrade bool
}

func kindStepValueFiles(op kindOp, w kindChartWork, step config.Chart, custom string) (files, temps []string, err error) {
	if _, edited := op.deployed[w.parent]; w.upgrade && edited {
		if step.HelmRelease() != op.releases[w.parent].name {
			return kindChartValueFiles(step, "")
		}
		if custom != "" {
			return []string{custom}, nil, nil
		}
		return nil, nil, nil
	}
	return kindChartValueFiles(step, custom)
}

func kindChartValueFiles(step config.Chart, custom string) (files, temps []string, err error) {
	if len(step.Values) > 0 {
		path, err := config.WriteValuesFile(step.Values)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, path)
		temps = append(temps, path)
	}
	if custom != "" {
		files = append(files, custom)
	}
	return files, temps, nil
}

func (m model) startKindCharts(op kindOp, prefix string) tea.Cmd {
	charts := m.cfg.Charts()
	var work []kindChartWork
	for _, name := range op.charts {
		c, ok := config.LookupChart(charts, name)
		if !ok {
			continue
		}
		work = append(work, kindChartWork{parent: name, steps: op.chartSteps(c)})
	}
	for _, name := range op.upgrade {
		c, ok := config.LookupChart(charts, name)
		if !ok {
			continue
		}
		work = append(work, kindChartWork{parent: name, steps: op.chartSteps(c), upgrade: true})
	}
	var install []config.Chart
	for _, w := range work {
		install = append(install, w.steps...)
	}
	var uninstall []config.Chart
	for i := len(op.uninstall) - 1; i >= 0; i-- {
		c, ok := config.LookupChart(charts, op.uninstall[i])
		if !ok {
			continue
		}
		steps := op.chartSteps(c)
		for j := len(steps) - 1; j >= 0; j-- {
			uninstall = append(uninstall, steps[j])
		}
	}
	kc, ctx := op.kubeconfig, kube.KindContext(op.name)
	bin := m.cfg.Helm.Binary
	run := m.run
	docker := m.dockerCLI()
	pane := m.logID
	newMetal := false
	for _, name := range op.charts {
		if strings.EqualFold(name, "metallb") {
			newMetal = true
		}
	}
	for _, name := range op.uninstall {
		if strings.EqualFold(name, "metallb") {
			newMetal = false
		}
	}
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			var temps []string
			defer func() {
				seen := map[string]bool{}
				for _, p := range temps {
					if p != "" && !seen[p] {
						seen[p] = true
						_ = os.Remove(p)
					}
				}
				for _, p := range op.values {
					if p != "" && !seen[p] {
						seen[p] = true
						_ = os.Remove(p)
					}
				}
			}()
			var log strings.Builder
			if prefix != "" {
				log.WriteString(prefix)
				if !strings.HasSuffix(prefix, "\n") {
					log.WriteByte('\n')
				}
			}
			var notes []string
			emit := func(line, step string) {
				if strings.TrimSpace(line) == "" {
					return
				}
				log.WriteString(line)
				if !strings.HasSuffix(line, "\n") {
					log.WriteByte('\n')
				}
				ch <- kindLineMsg{pane: pane, name: op.name, line: line, step: step, ch: ch}
			}
			fail := func(err error) {
				ch <- kindChartsMsg{pane: pane, name: op.name, text: log.String(), err: err, notes: strings.Join(notes, "\n")}
			}
			for _, step := range uninstall {
				args := kube.HelmUninstallArgs(step.HelmRelease(), step.Namespace, kc, ctx)
				emit("$ helm "+strings.Join(args, " "), "uninstalling "+step.HelmRelease())
				_, err := kindCmdOut(run, bin, args, "", func(line string) { emit(line, "uninstalling "+step.HelmRelease()) })
				if err != nil {
					fail(err)
					return
				}
			}
			if err := ensureRepos(kube.Client{Run: run}, bin, install, func(line string) { emit(line, "adding helm repos") }); err != nil {
				fail(err)
				return
			}
			for _, w := range work {
				verb := "installing "
				if w.upgrade {
					verb = "upgrading "
				}
				custom := ""
				if op.values != nil {
					custom = op.values[w.parent]
				}
				if config.IsSealedSecretsName(w.parent) {
					path := ""
					if op.masterKeys != nil {
						path = op.masterKeys[w.parent]
					}
					if path == "" {
						if c, ok := config.LookupChart(charts, w.parent); ok {
							path = c.MasterKeyPath
						}
					}
					if path != "" {
						ns, createNS := "", false
						if len(w.steps) > 0 {
							ns = w.steps[0].Namespace
							createNS = w.steps[0].CreatesNamespace()
						}
						stepName := "applying sealed-secrets master key"
						if err := emitApplyMasterKey(run, path, ns, kc, ctx, createNS, func(line string) { emit(line, stepName) }); err != nil {
							fail(err)
							return
						}
					}
				}
				for _, step := range w.steps {
					files, extra, err := kindStepValueFiles(op, w, step, custom)
					if err != nil {
						fail(err)
						return
					}
					temps = append(temps, extra...)
					values := ""
					if len(files) > 0 {
						values = files[0]
					}
					args := kube.HelmApplyArgs(w.upgrade, step.HelmRelease(), step.HelmChart(), step.Version, step.Namespace, step.CreatesNamespace(), values, kc, ctx)
					if len(files) > 1 {
						for _, f := range files[1:] {
							args = append(args, "-f", f)
						}
					}
					emit("$ helm "+strings.Join(args, " "), verb+step.HelmRelease())
					_, err = kindCmdOut(run, bin, args, "", func(line string) { emit(line, verb+step.HelmRelease()) })
					if err != nil {
						fail(err)
						return
					}
					if !w.upgrade && newMetal && (strings.EqualFold(step.Name, "metallb") || strings.EqualFold(step.HelmRelease(), "metallb")) {
						n, wnote := setupMetalLB(run, docker, kc, ctx, step.Namespace, func(line string) { emit(line, "configuring metallb") })
						if n != "" {
							notes = append(notes, n)
						}
						if wnote != "" {
							notes = append(notes, wnote)
						}
					}
				}
			}
			ch <- kindChartsMsg{pane: pane, name: op.name, text: log.String(), notes: strings.Join(notes, "\n")}
		}()
		return <-ch
	}
}

func ensureRepos(k kube.Client, bin string, steps []config.Chart, onLine func(string)) error {
	seen := map[string]bool{}
	for _, step := range steps {
		name, url := step.HelmRepo(), step.Repo
		if name == "" || url == "" || seen[name] {
			continue
		}
		seen[name] = true
		onLine("$ helm " + strings.Join(config.HelmRepoAddArgs(name, url), " "))
		out, err := k.HelmOut(bin, config.HelmRepoAddArgs(name, url)...)
		onLine(out)
		if err != nil {
			return err
		}
		onLine("$ helm " + strings.Join(config.HelmRepoUpdateArgs(name), " "))
		out, err = k.HelmOut(bin, config.HelmRepoUpdateArgs(name)...)
		onLine(out)
		if err != nil {
			return err
		}
	}
	return nil
}

func setupMetalLB(run shell.Runner, docker, kc, ctx, ns string, onLine func(string)) (notes, warn string) {
	if ns == "" {
		ns = "metallb-system"
	}
	label := []string{"label", "namespace", ns,
		"pod-security.kubernetes.io/enforce=privileged",
		"pod-security.kubernetes.io/audit=privileged",
		"pod-security.kubernetes.io/warn=privileged",
		"--overwrite", "--kubeconfig", kc, "--context", ctx}
	onLine("$ kubectl " + strings.Join(label, " "))
	out, _ := shell.Combined(run, "kubectl", label...)
	onLine(string(out))
	wait := []string{"wait", "--for=condition=Available", "deployment", "--all", "-n", ns, "--timeout=120s"}
	wait = append(wait, "--kubeconfig", kc, "--context", ctx)
	onLine("$ kubectl " + strings.Join(wait, " "))
	out, _ = shell.Combined(run, "kubectl", wait...)
	onLine(string(out))
	if docker == "" {
		return "", "metallb: docker not in PATH, skipped IPAddressPool"
	}
	inspect := kube.DockerNetworkInspectArgs("kind")
	onLine("$ docker " + strings.Join(inspect, " "))
	out, err := shell.Combined(run, docker, inspect...)
	onLine(string(out))
	if err != nil {
		return "", "metallb: could not inspect the kind docker network, skipped IPAddressPool"
	}
	rng, err := kube.MetalLBRange(kube.ParseDockerSubnets(string(out)))
	if err != nil {
		return "", "metallb: " + err.Error() + ", skipped IPAddressPool"
	}
	manifest := kube.MetalLBManifest(ns, "k2-kind", rng)
	apply := []string{"apply", "-f", "-", "--kubeconfig", kc, "--context", ctx}
	var last error
	for i := 0; i < max(1, metalRetryN); i++ {
		if i > 0 && metalRetryWait > 0 {
			time.Sleep(metalRetryWait)
		}
		onLine("$ kubectl apply -f -")
		res := run.Run(shell.Request{Name: "kubectl", Args: apply, Stdin: manifest, Combined: true})
		onLine(string(res.Stdout) + string(res.Stderr))
		if res.Err == nil {
			return "metallb pool " + rng, ""
		}
		last = res.Err
	}
	if last != nil {
		return "", "metallb: installed, but applying IPAddressPool " + rng + " failed"
	}
	return "metallb pool " + rng, ""
}

func (m *model) applyKindCharts(msg kindChartsMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		name := msg.name
		if name == "" && m.vs.kop != nil {
			name = m.vs.kop.name
		}
		j := m.ensureKindJob(name, "", "")
		if msg.notes != "" {
			if j.log != "" && !strings.HasSuffix(j.log, "\n") {
				j.log += "\n"
			}
			j.log += msg.notes + "\n"
		}
		if msg.err != nil {
			j.status, j.err = "failed", msg.err
			if msg.text != "" {
				j.last = lastNonEmpty(msg.text)
			}
			m.syncKindLog(name)
			if m.stage != "kindlog" {
				m.stage = "kind"
				m.selectKind(name)
			}
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "chart install failed")
			return
		}
		j.status, j.step, j.err = "running", "", nil
		m.saveKindValuesCaches(j.op)
		m.syncKindLog(name)
		m.showKind(name)
		if m.stage == "kindlog" {
			m.status = "cluster " + name + " ready"
		} else {
			m.stage = "kind"
			m.status = "cluster " + name + " ready"
		}
		if j.op.kubeconfig != "" && m.kubeconfig == j.op.kubeconfig {
			cmd = tea.Batch(m.notify(toneOK, m.status), m.kindRefreshCmd(), fetchResources(m.k, m.kubeconfig, m.context))
			return
		}
		cmd = tea.Batch(m.notify(toneOK, m.status), m.kindRefreshCmd())
	})
	return cmd
}

func (m *model) applyKindInstalled(msg kindInstalledMsg) tea.Cmd {
	return m.applyKindCharts(kindChartsMsg{pane: msg.pane, name: msg.name, text: msg.text, notes: msg.notes, err: msg.err})
}

func (m model) askKindDelete(name string) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("delete cluster"); blocked {
		return next, cmd
	}
	bin, next, cmd, ok := m.needKindCLI("delete cluster")
	if !ok {
		return next, cmd
	}
	home, _ := os.UserHomeDir()
	managed := kube.KindKubeconfig(name, home)
	details := []string{kindDetail("cluster", name), kindDetail("context", kube.KindContext(name))}
	if kube.ManagedKindKubeconfig(managed, name, home) {
		details = append(details, kindDetail("will remove", config.HomePath(managed)))
	}
	run := m.run
	kc := m.kubeconfig
	pane := m.logID
	return m.ask(askSpec{
		title: "✗ kind delete", question: "delete kind cluster " + name + "?", details: details, danger: true,
		kind: "kind", name: name, running: "deleting " + name + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			if m.kindJobs == nil {
				m.kindJobs = map[string]*kindJob{}
			}
			j := m.ensureKindJob(name, "deleting", "deleting cluster")
			j.err, j.log, j.last, j.started = nil, "", "", now()
			m.stage = "kind"
			m.selectKind(name)
			m.status = "deleting " + name
			return m, m.kindStream(name, run, bin, kube.KindDeleteArgs(name), "", func(text string, err error) tea.Msg {
				if err == nil && kube.ManagedKindKubeconfig(managed, name, home) && filepath.Base(managed) != "config" {
					_ = os.Remove(managed)
				}
				return kindDeleteMsg{pane: pane, name: name, text: text, err: err, gone: kc == managed}
			})
		},
	}), nil
}

func (m *model) applyKindDeleted(msg kindDeleteMsg) tea.Cmd {
	var cmd tea.Cmd
	apply := func() {
		j := m.ensureKindJob(msg.name, "", "")
		if msg.err != nil {
			j.status, j.err = "failed", msg.err
			if msg.text != "" {
				j.last = lastNonEmpty(msg.text)
			}
			m.syncKindLog(msg.name)
			if m.stage != "kindlog" {
				m.stage = "kind"
				m.selectKind(msg.name)
			}
			m.status = m.errLine(msg.err)
			cmd = m.notify(toneErr, "kind delete failed")
			return
		}
		delete(m.kindJobs, msg.name)
		var names []string
		for _, n := range m.vs.kindNames {
			if n != msg.name {
				names = append(names, n)
			}
		}
		m.vs.kindNames = names
		wasActive := msg.gone || m.context == kube.KindContext(msg.name)
		if wasActive {
			var usable []string
			for _, n := range names {
				if _, ok := m.kindKubeconfigFor(n); ok {
					usable = append(usable, n)
				}
			}
			if len(usable) == 1 {
				path, _ := m.kindKubeconfigFor(usable[0])
				m.kubeconfig = path
				m.context = kube.KindContext(usable[0])
				m.namespace = m.k.NamespaceOf(path, m.context)
				m.clearHist()
				m.save()
				if m.stage != "kindlog" {
					m.stage = "kind"
				} else if m.vs.kindLog == msg.name {
					m.stage = "kind"
					m.vs.kindLog = ""
				}
				m.showKind(usable[0])
				m.status = "deleted " + msg.name + " · using " + m.context
				cmd = tea.Batch(m.notify(toneOK, m.status), m.kindRefreshCmd(), fetchResources(m.k, m.kubeconfig, m.context))
				return
			}
			m.kubeconfig, m.context, m.namespace = "", "", ""
			m.save()
		}
		if m.stage != "kindlog" {
			m.stage = "kind"
		} else if m.vs.kindLog == msg.name {
			m.stage = "kind"
			m.vs.kindLog = ""
		}
		m.showKind("")
		m.status = "deleted " + msg.name
		cmd = tea.Batch(m.notify(toneOK, m.status), m.kindRefreshCmd())
	}
	if msg.pane != "" {
		m.withPane(msg.pane, apply)
	} else {
		apply()
	}
	return cmd
}

func parseKindCmd(rest string) (kindOp, bool, string) {
	fields := splitHelmArgs(rest)
	if len(fields) == 0 {
		return kindOp{}, false, ""
	}
	if strings.ToLower(fields[0]) != "create" {
		return kindOp{}, false, ""
	}
	op := kindOp{name: "k2", planes: 1}
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
		case a == "--workers" || a == "--worker":
			n, err := strconv.Atoi(take())
			if err != nil || n < 0 {
				return kindOp{}, false, "workers must be a number >= 0"
			}
			op.workers = n
		case a == "--control-plane" || a == "--control-planes" || a == "--planes":
			n, err := strconv.Atoi(take())
			if err != nil || n < 1 {
				return kindOp{}, false, "control-planes must be a number >= 1"
			}
			op.planes = n
		case a == "--config":
			op.config = take()
		case strings.HasPrefix(a, "--config="):
			op.config = strings.TrimPrefix(a, "--config=")
		case a == "--image":
			op.image = take()
		case strings.HasPrefix(a, "--image="):
			op.image = strings.TrimPrefix(a, "--image=")
		case a == "--charts":
			op.charts = splitChartList(take())
		case strings.HasPrefix(a, "--charts="):
			op.charts = splitChartList(strings.TrimPrefix(a, "--charts="))
		case strings.HasPrefix(a, "-"):
			return kindOp{}, false, "unknown flag: " + a
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 0 {
		op.name = pos[0]
	}
	if len(pos) > 1 {
		return kindOp{}, false, "kind create [name] [--workers N] [--control-planes N] [--config FILE] [--image IMAGE] [--charts a,b]"
	}
	return op, true, ""
}

func splitChartList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func kindReleaseSet(secrets []kube.Object) map[string]bool {
	have := map[string]bool{}
	for _, r := range kube.HelmLatest(secrets) {
		if r.Name != "" {
			have[r.Name] = true
		}
	}
	return have
}

func catalogChartOn(have map[string]bool, c config.Chart) bool {
	for _, s := range c.InstallSteps() {
		if have[s.HelmRelease()] {
			return true
		}
	}
	return false
}

func (m model) openKindCharts(name string) (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("edit charts"); blocked {
		return next, cmd
	}
	t := m.kindTileFor(name)
	if t.status == "deploying" || t.status == "deleting" {
		m.status = name + " is still " + t.status
		return m, m.notify(toneWarn, m.status)
	}
	path, ok := m.kindKubeconfigFor(name)
	if !ok {
		m.status = "no kubeconfig for " + name
		return m, m.notify(toneWarn, m.status)
	}
	if _, next, cmd, ok := m.needHelmCLI("edit charts"); !ok {
		return next, cmd
	}
	secrets, err := m.k.HelmSecrets(path, kube.KindContext(name), "")
	have := map[string]bool{}
	if err == nil {
		have = kindReleaseSet(secrets)
	}
	var selected []string
	for _, c := range m.cfg.Charts() {
		if catalogChartOn(have, c) {
			selected = append(selected, c.Name)
		}
	}
	op := kindOp{name: name, kubeconfig: path, charts: selected, installed: append([]string{}, selected...), form: "kindcharts"}
	if err == nil {
		op.namespaces = kindInstalledNamespaces(secrets, m.cfg.Charts(), selected)
		op.releases = kindInstalledReleases(secrets, m.cfg.Charts(), selected)
	}
	d := m.newDialog("kindcharts", kindIconCharts+" charts · "+name, "kind", "", name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillKindChartsForm(d, &op)
	if len(d.hfields) == 0 {
		m.status = "no catalog charts configured"
		return m, m.notify(toneWarn, m.status)
	}
	if err != nil {
		d.msg = "could not list installed charts: " + err.Error()
	}
	m.vs.kop = &op
	m.dlg = d
	return m, nil
}

func (m model) submitKindCharts() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("edit charts"); blocked {
		return next, cmd
	}
	if m.vs.kop == nil || m.dlg == nil {
		return m, nil
	}
	op := *m.vs.kop
	m.readKindNamespaces(&op)
	had := map[string]bool{}
	already := op.installed
	if already == nil {
		already = op.charts
	}
	for _, n := range already {
		had[n] = true
	}
	var want []string
	wantSet := map[string]bool{}
	for _, f := range m.dlg.hfields {
		if strings.HasPrefix(f.key, "c:") && f.on {
			n := strings.TrimPrefix(f.key, "c:")
			want = append(want, n)
			wantSet[n] = true
		}
	}
	var install, uninstall, upgrade []string
	for _, n := range want {
		if !had[n] {
			install = append(install, n)
		} else if op.chartUpgrade(n) {
			upgrade = append(upgrade, n)
		}
	}
	for _, n := range already {
		if !wantSet[n] {
			uninstall = append(uninstall, n)
		}
	}
	if len(install) == 0 && len(uninstall) == 0 && len(upgrade) == 0 {
		m.dlg = nil
		m.status = "no chart changes"
		return m, nil
	}
	back := op
	back.charts = want
	op.charts, op.uninstall, op.upgrade = install, uninstall, upgrade
	m.vs.kop = &op
	if kindSelectsSealedSecrets(op.charts, op.upgrade) {
		if _, msg := m.validateMasterKeyFile(m.kindSealedMasterKey(&op)); msg != "" {
			m.dlg.msg = msg
			return m, nil
		}
	}
	var details []string
	if len(install) > 0 {
		details = append(details, kindDetail("install", kindChartsWithNS(install, op, m.cfg.Charts())))
	}
	if len(upgrade) > 0 {
		details = append(details, kindDetail("upgrade", kindChartsWithNS(upgrade, op, m.cfg.Charts())+" (values)"))
	}
	if len(uninstall) > 0 {
		details = append(details, kindDetail("uninstall", kindChartsWithNS(uninstall, op, m.cfg.Charts())))
	}
	if kindSelectsSealedSecrets(op.charts, op.upgrade) {
		if path := m.kindSealedMasterKey(&op); path != "" {
			details = append(details, kindDetail("master key", config.HomePath(path)))
		}
	}
	details = append(details, kindDetail("cluster", op.name), kindDetail("kubeconfig", config.HomePath(op.kubeconfig)))
	return m.ask(askSpec{
		title: "▦ charts", question: "apply chart changes on " + op.name + "?", details: details, danger: true,
		kind: "kind", name: op.name, running: "updating charts on " + op.name + "…",
		onBack: func(m model) (tea.Model, tea.Cmd) {
			m.vs.kop = &back
			m.reopenKindForm()
			return m, nil
		},
		onYes: func(m model) (tea.Model, tea.Cmd) {
			if m.kindJobs == nil {
				m.kindJobs = map[string]*kindJob{}
			}
			step := "installing charts"
			if len(install) == 0 && len(upgrade) > 0 {
				step = "upgrading charts"
			} else if len(install) == 0 {
				step = "uninstalling charts"
			}
			j := m.ensureKindJob(op.name, "deploying", step)
			j.op, j.err, j.log, j.last, j.started = op, nil, "", "", now()
			m.stage = "kind"
			m.selectKind(op.name)
			m.status = "updating charts on " + op.name
			return m, m.startKindCharts(op, "")
		},
	}), nil
}

func applyKindConfigToOp(op *kindOp, path string) {
	if op == nil || path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	info, err := kube.ParseKindConfig(string(raw))
	if err != nil {
		return
	}
	if info.Planes >= 1 {
		op.planes = info.Planes
	}
	if info.Workers >= 0 {
		op.workers = info.Workers
	}
	if info.Image != "" {
		op.image = info.Image
	}
}

func (m *model) applyKindConfigEdit() {
	if m.vs.kop == nil {
		return
	}
	m.vs.kop.edited = true
	path := strings.TrimSpace(m.vs.kop.config)
	if path == "" {
		return
	}
	applyKindConfigToOp(m.vs.kop, expandPath(path, ""))
}

func (m model) editKindConfig() (tea.Model, tea.Cmd) {
	m.stashKindForm()
	if m.vs.kop == nil {
		return m, nil
	}
	op := *m.vs.kop
	path := strings.TrimSpace(op.config)
	if path != "" {
		path = expandPath(path, "")
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			if m.dlg != nil {
				m.dlg.msg = "config file not found: " + config.HomePath(path)
			}
			return m, nil
		}
	} else {
		text, err := kube.KindConfigYAML(op.planes, op.workers, op.image)
		if err != nil {
			if m.dlg != nil {
				m.dlg.msg = err.Error()
			}
			return m, nil
		}
		f, err := os.CreateTemp("", "k2-kind-*.yaml")
		if err != nil {
			if m.dlg != nil {
				m.dlg.msg = err.Error()
			}
			return m, nil
		}
		if _, err := f.WriteString(text); err != nil {
			f.Close()
			if m.dlg != nil {
				m.dlg.msg = err.Error()
			}
			return m, nil
		}
		f.Close()
		path = f.Name()
		op.edited, op.config = true, path
		m.vs.kop = &op
	}
	m.status = "opening editor…"
	return m.runEditor("kindconfig", "vim "+filepath.Base(path), filepath.Base(path), "",
		func() *exec.Cmd { return vimCommand(path) },
		func(error) tea.Msg { return kindConfigDoneMsg{} })
}

func (m model) editKindChartValues() (tea.Model, tea.Cmd) {
	if m.dlg == nil || m.dlg.idx < 0 || m.dlg.idx >= len(m.dlg.hfields) {
		return m, nil
	}
	key := m.dlg.hfields[m.dlg.idx].key
	if !strings.HasPrefix(key, "c:") {
		return m, nil
	}
	name := strings.TrimPrefix(key, "c:")
	m.dlg.hfields[m.dlg.idx].on = true
	m.stashKindForm()
	if m.vs.kop == nil {
		return m, nil
	}
	op := *m.vs.kop
	if op.values == nil {
		op.values = map[string]string{}
	}
	path := op.values[name]
	if path == "" {
		seed := "# custom values for " + name + "\n"
		if c, ok := config.LookupChart(m.cfg.Charts(), name); ok {
			if text, err := config.ValuesFileYAML(c.Values); err == nil && text != "" {
				seed = text
			}
		}
		if op.isInstalled(name) {
			text, err := m.deployedKindValues(op, name)
			if err != nil {
				m.dlg.msg = "could not read deployed values: " + err.Error()
				return m, nil
			}
			if op.deployed == nil {
				op.deployed = map[string]string{}
			}
			op.deployed[name] = text
			seed = text
		}
		f, err := os.CreateTemp("", "k2-"+name+"-values-*.yaml")
		if err != nil {
			m.dlg.msg = err.Error()
			return m, nil
		}
		if _, err := f.WriteString(seed); err != nil {
			f.Close()
			m.dlg.msg = err.Error()
			return m, nil
		}
		f.Close()
		path = f.Name()
		op.values[name] = path
	}
	if op.form == "" {
		if m.dlg != nil && m.dlg.act == "kindcharts" {
			op.form = "kindcharts"
		} else {
			op.form = "kindcreate"
		}
	}
	op.valuesChart = name
	m.vs.kop = &op
	m.status = "opening editor…"
	return m.runEditor("kindvalues", "vim "+filepath.Base(path), filepath.Base(path), "",
		func() *exec.Cmd { return vimCommand(path) },
		func(error) tea.Msg { return kindValuesDoneMsg{} })
}

func (m model) deployedKindValues(op kindOp, name string) (string, error) {
	rel, ok := op.releases[name]
	if !ok {
		return "", fmt.Errorf("%s has no release on %s", name, op.name)
	}
	bin := m.helmCLI()
	if bin == "" {
		return "", fmt.Errorf("helm CLI not found")
	}
	args := []string{"get", "values", rel.name, "-o", "yaml"}
	args = append(args, kube.HelmArgs(op.kubeconfig, kube.KindContext(op.name), rel.namespace)...)
	raw, err := shell.Output(m.run, bin, args...)
	if err != nil {
		return "", errors.New(shell.FailText(raw, err))
	}
	out := string(raw)
	if kindValuesEmpty(out) {
		out = "{}\n"
	} else {
		out = formatHelmValues(out)
	}
	out = m.preferHelmValues(kube.KindContext(op.name), rel.namespace, rel.name, out)
	return out, nil
}

func kindUpgradeTag(on bool) string {
	if on {
		return "upgrade"
	}
	return ""
}

func kindParseValues(text string) (map[string]any, bool) {
	var v any
	if err := yaml.Unmarshal([]byte(text), &v); err != nil {
		return nil, false
	}
	switch t := v.(type) {
	case nil:
		return map[string]any{}, true
	case map[string]any:
		return t, true
	}
	return nil, false
}

func kindValuesEmpty(text string) bool {
	v, ok := kindParseValues(text)
	return ok && len(v) == 0
}

func kindValuesDiffer(a, b string) bool {
	va, okA := kindParseValues(a)
	vb, okB := kindParseValues(b)
	if !okA || !okB {
		return strings.TrimSpace(a) != strings.TrimSpace(b)
	}
	return !reflect.DeepEqual(va, vb)
}

func (op kindOp) chartUpgrade(name string) bool {
	path := ""
	if op.values != nil {
		path = op.values[name]
	}
	if path == "" {
		return false
	}
	base, ok := op.deployed[name]
	if !ok {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return kindValuesDiffer(base, string(raw))
}

func (m *model) reopenKindForm() {
	op := kindOp{name: "k2", planes: 1}
	if m.vs.kop != nil {
		op = *m.vs.kop
	}
	if op.form == "kindcharts" {
		title := kindIconCharts + " charts"
		if op.name != "" {
			title += " · " + op.name
		}
		d := m.newDialog("kindcharts", title, "kind", "", op.name)
		m.dlgSeq++
		d.seq = m.dlgSeq
		m.fillKindChartsForm(d, &op)
		m.vs.kop = &op
		m.dlg = d
		return
	}
	d := m.newDialog("kindcreate", "✚ kind create", "kind", "", op.name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillKindForm(d, &op)
	m.vs.kop = &op
	m.dlg = d
}

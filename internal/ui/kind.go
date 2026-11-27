package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type kindOp struct {
	name, config, image, kubeconfig string
	planes, workers                 int
	charts                          []string
}

type kindListMsg struct {
	pane  string
	token int
	names []string
	err   error
}

type kindRunMsg struct {
	pane  string
	token int
	text  string
	err   error
	op    kindOp
}

type kindChartsMsg struct {
	pane  string
	token int
	text  string
	err   error
	notes string
}

type kindDeleteMsg struct {
	name string
	text string
	err  error
	gone bool
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
	if m.stage != "kind" {
		m.rememberView()
	}
	_, next, cmd, ok := m.needKindCLI("kind")
	if !ok {
		nm := next.(model)
		nm.stage = "kind"
		nm.showKind("")
		return nm, cmd
	}
	spin := m.startLoad("loading kind clusters", true)
	return m, tea.Batch(spin, kindListCmd(m.kindCLI(), m.run, m.logID, m.load.token))
}

func kindListCmd(bin string, run shell.Runner, pane string, token int) tea.Cmd {
	return func() tea.Msg {
		out, err := shell.Combined(run, bin, kube.KindGetArgs()...)
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				text = err.Error()
			}
			return kindListMsg{pane: pane, token: token, err: fmt.Errorf("%s", text)}
		}
		return kindListMsg{pane: pane, token: token, names: kube.ParseKindClusters(string(out))}
	}
}

func (m *model) applyKindList(msg kindListMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		if m.load.token != msg.token && msg.token != 0 {
			return
		}
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			m.stage = "kind"
			m.showKind("")
			cmd = m.notify(toneErr, "could not list kind clusters")
			return
		}
		if m.stage != "kind" {
			m.vs.sortCol, m.vs.sortDesc = "", false
			m.content.ResetFilter()
			m.content.SetItems(nil)
		}
		m.stage = "kind"
		m.vs.kindNames = msg.names
		m.showKind("")
		if len(msg.names) == 0 {
			m.status = "no kind clusters — " + m.cfg.Keys.HelmInstall.Label() + " creates one"
			return
		}
		m.status = m.kindHint()
	})
	return cmd
}

func (m model) kindHint() string {
	k := m.cfg.Keys
	return k.Select.Label() + " use, " + k.HelmInstall.Label() + " create, " + k.Delete.Label() + " delete"
}

func (m *model) showKind(pick string) {
	home, _ := os.UserHomeDir()
	t := kube.Table{Headers: []string{"NAME", "KUBECONFIG", "ACTIVE"}, ColorCol: 2}
	for _, name := range m.vs.kindNames {
		path, ok := m.kindKubeconfigFor(name)
		shown := "—"
		if ok {
			shown = config.HomePath(path)
		} else {
			managed := kube.KindKubeconfig(name, home)
			if managed != "" {
				shown = config.HomePath(managed)
			}
		}
		active := "no"
		if ok && m.kubeconfig == path && (m.context == kube.KindContext(name) || strings.HasPrefix(m.context, "kind-"+name)) {
			active = "yes"
		}
		t.Rows = append(t.Rows, []string{name, shown, active})
		t.Refs = append(t.Refs, [2]string{"kind", name})
	}
	header, items, _ := packKube(t, m.tableWidth(), "kind")
	m.content.SetShowTitle(false)
	setRows(&m.content, header, items, m.vs, pick)
}

func (m model) selectedKind() (string, bool) {
	it, ok := m.selectedItem()
	if !ok || it.id == "" {
		return "", false
	}
	return it.id, true
}

func (m model) kindKubeconfigFor(name string) (string, bool) {
	home, _ := os.UserHomeDir()
	path := kube.KindKubeconfig(name, home)
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path, true
	}
	def := filepath.Join(home, ".kube", "config")
	if st, err := os.Stat(def); err != nil || st.IsDir() {
		return "", false
	}
	ctx := kube.KindContext(name)
	contexts, err := m.k.Contexts(def)
	if err != nil {
		return "", false
	}
	for _, c := range contexts {
		if c == ctx {
			return def, true
		}
	}
	return "", false
}

func (m model) kindKey(key string) (tea.Model, tea.Cmd, bool) {
	km := m.cfg.Keys
	switch {
	case km.HelmInstall.Has(key) || km.Apply.Has(key):
		next, cmd := m.openKindCreate(kindOp{name: "k2", planes: 1})
		return next, cmd, true
	case km.Delete.Has(key):
		name, ok := m.selectedKind()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.askKindDelete(name)
		return next, cmd, true
	case km.Select.Has(key):
		name, ok := m.selectedKind()
		if !ok {
			return m, nil, true
		}
		next, cmd := m.useKindCluster(name)
		return next, cmd, true
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
	title := "✚ kind create"
	d := m.newDialog("kindcreate", title, "kind", "", op.name)
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.fillKindForm(d, &op)
	m.vs.kop = &op
	m.dlg = d
	return m, nil
}

func (m *model) fillKindForm(d *dialog, op *kindOp) {
	fields := []helmField{
		{key: "name", label: "name", input: helmFieldInput(op.name, "k2")},
		{key: "planes", label: "control-planes", input: helmFieldInput(strconv.Itoa(max(1, op.planes)), "1")},
		{key: "workers", label: "workers", input: helmFieldInput(strconv.Itoa(max(0, op.workers)), "0")},
		{key: "config", label: "config", input: helmFieldInput(op.config, "path to kind config")},
		{key: "image", label: "image", input: helmFieldInput(op.image, "kindest/node:…")},
	}
	selected := map[string]bool{}
	for _, n := range op.charts {
		selected[n] = true
	}
	for _, c := range m.cfg.Charts() {
		fields = append(fields, helmField{
			key:    "c:" + c.Name,
			label:  c.Name,
			input:  helmFieldInput("", ""),
			toggle: true,
			on:     selected[c.Name],
		})
	}
	helmFocus(&fields[0])
	d.hfields = fields
	d.idx = 0
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
	case key == "tab" || key == "down" || km.ScrollDown.Has(key):
		if key == "tab" && d.hfields[d.idx].key == "config" {
			if next, cmd, done := m.completeKindPath(); done {
				return next, cmd
			}
		}
		return m.moveHelmField(1), nil
	case key == "shift+tab" || key == "up" || km.ScrollUp.Has(key):
		return m.moveHelmField(-1), nil
	case (key == " " || key == "c") && d.hfields[d.idx].toggle:
		d.hfields[d.idx].on = !d.hfields[d.idx].on
		return m, nil
	}
	if d.hfields[d.idx].toggle || d.hfields[d.idx].action || d.hfields[d.idx].locked {
		return m, nil
	}
	d.cands, d.candBase, d.candAt = nil, "", -1
	var cmd tea.Cmd
	d.hfields[d.idx].input, cmd = d.hfields[d.idx].input.Update(msg)
	d.msg = ""
	return m, cmd
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
		default:
			if strings.HasPrefix(f.key, "c:") && f.on {
				charts = append(charts, strings.TrimPrefix(f.key, "c:"))
			}
		}
	}
	op.charts = charts
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
	case op.planes < 1:
		m.dlg.msg = "need at least one control-plane"
		return m, nil
	case op.workers < 0:
		m.dlg.msg = "worker count cannot be negative"
		return m, nil
	}
	if op.config != "" {
		p := expandPath(op.config, "")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			m.dlg.msg = "config file not found: " + config.HomePath(p)
			return m, nil
		}
		op.config = p
		m.vs.kop = op
	}
	if len(op.charts) > 0 {
		if _, next, cmd, ok := m.needHelmCLI("starter charts"); !ok {
			return next, cmd
		}
	}
	home, _ := os.UserHomeDir()
	op.kubeconfig = kube.KindKubeconfig(op.name, home)
	m.vs.kop = op
	m.dlg = nil
	return m.askKindCreate(*op)
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
		"cluster " + op.name,
		"context " + kube.KindContext(op.name),
		"kubeconfig " + config.HomePath(op.kubeconfig),
	}
	if op.config != "" {
		details = append(details, "config "+config.HomePath(op.config))
	} else {
		details = append(details, topo)
	}
	if op.image != "" {
		details = append(details, "image "+op.image)
	}
	if len(op.charts) > 0 {
		details = append(details, "charts "+strings.Join(op.charts, ", "))
	}
	return m.ask(askSpec{
		title: "✚ kind create", question: "create kind cluster " + op.name + "?", details: details, danger: true,
		kind: "kind", name: op.name, running: "creating cluster " + op.name + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			spin := m.startLoad("creating cluster "+op.name, false)
			return m, tea.Batch(spin, m.kindCreateStream(op))
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
	}
	if err := os.MkdirAll(filepath.Dir(op.kubeconfig), 0o700); err != nil {
		return func() tea.Msg { return kindRunMsg{pane: pane, err: err, op: op} }
	}
	args := kube.KindCreateArgs(op.name, op.kubeconfig, configPath, image)
	return m.streamExec(run, bin, args, "", "creating "+op.name, func(text string, err error) tea.Msg {
		return kindRunMsg{pane: pane, text: text, err: err, op: op}
	})
}

func (m model) streamExec(run shell.Runner, bin string, args []string, stdin, what string, done func(string, error) tea.Msg) tea.Cmd {
	pane, token := m.logID, m.load.token
	if token == 0 {
		m.loadSeq++
		m.load = load{token: m.loadSeq, what: what, from: m.stage, start: now()}
		token = m.load.token
	} else {
		m.load.what = what
	}
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			var buf strings.Builder
			req := shell.Request{Name: bin, Args: args, Stdin: stdin, Combined: true, Stream: true, OnLine: func(line string) {
				if strings.TrimSpace(line) == "" {
					return
				}
				buf.WriteString(line)
				if !strings.HasSuffix(line, "\n") {
					buf.WriteByte('\n')
				}
				ch <- progressMsg{id: pane, token: token, line: line, what: what, ch: ch}
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

func (m *model) applyKindCreated(msg kindRunMsg) tea.Cmd {
	var cmd tea.Cmd
	m.withPane(msg.pane, func() {
		m.load = load{}
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			m.subject = "kind create " + msg.op.name
			m.logText = or(msg.text, msg.err.Error())
			m.logStick, m.logAt = true, 0
			m.stage = "view"
			m.vs.vback = "kind"
			cmd = tea.Batch(m.notify(toneErr, "kind create failed"), m.openErrCmd())
			return
		}
		m.kubeconfig = msg.op.kubeconfig
		m.context = kube.KindContext(msg.op.name)
		m.namespace = m.k.NamespaceOf(m.kubeconfig, m.context)
		m.clearHist()
		m.save()
		if len(msg.op.charts) == 0 {
			m.showKindResult(msg.op, msg.text, "", nil)
			cmd = tea.Batch(m.notify(toneOK, "created "+msg.op.name), fetchResources(m.k, m.kubeconfig, m.context))
			return
		}
		cmd = tea.Batch(m.notify(toneOK, "created "+msg.op.name), m.startKindCharts(msg.op, msg.text))
	})
	return cmd
}

func (m *model) showKindResult(op kindOp, text, notes string, err error) {
	head := "# kind cluster " + op.name + "\n# kubeconfig " + config.HomePath(op.kubeconfig) + "\n# context " + kube.KindContext(op.name) + "\n\n"
	if notes != "" {
		head += notes + "\n\n"
	}
	m.subject = "kind " + op.name
	m.logText = head + text
	if err != nil {
		m.logText += "\n\n" + err.Error()
	}
	m.logStick, m.logAt = true, 0
	m.stage = "view"
	m.vs.vback = "kind"
	m.status = "cluster " + op.name + " · " + m.cfg.Keys.Back.Label() + " back"
	if err != nil {
		m.status = m.errLine(err)
	}
}

func (m *model) startKindCharts(op kindOp, prefix string) tea.Cmd {
	spin := m.startLoad("installing charts", false)
	charts := m.cfg.Charts()
	var steps []config.Chart
	for _, name := range op.charts {
		c, ok := config.LookupChart(charts, name)
		if !ok {
			continue
		}
		steps = append(steps, c.InstallSteps()...)
	}
	kc, ctx := op.kubeconfig, kube.KindContext(op.name)
	bin := m.cfg.Helm.Binary
	run := m.run
	k := m.k
	docker := m.dockerCLI()
	pane, token := m.logID, m.load.token
	return tea.Batch(spin, func() tea.Msg {
		var log strings.Builder
		if prefix != "" {
			log.WriteString(prefix)
			log.WriteString("\n")
		}
		var notes []string
		onLine := func(line string) {
			log.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				log.WriteByte('\n')
			}
		}
		if err := ensureRepos(k, bin, steps, onLine); err != nil {
			return kindChartsMsg{pane: pane, token: token, text: log.String(), err: err, notes: strings.Join(notes, "\n")}
		}
		for _, step := range steps {
			values := ""
			if len(step.Values) > 0 {
				path, err := config.WriteValuesFile(step.Values)
				if err != nil {
					return kindChartsMsg{pane: pane, token: token, text: log.String(), err: err, notes: strings.Join(notes, "\n")}
				}
				values = path
			}
			args := kube.HelmApplyArgs(false, step.HelmRelease(), step.HelmChart(), step.Version, step.Namespace, step.CreatesNamespace(), values, kc, ctx)
			onLine("$ helm " + strings.Join(args, " "))
			out, err := k.HelmOut(bin, args...)
			onLine(out)
			if err != nil {
				return kindChartsMsg{pane: pane, token: token, text: log.String(), err: err, notes: strings.Join(notes, "\n")}
			}
			if strings.EqualFold(step.Name, "metallb") || strings.EqualFold(step.HelmRelease(), "metallb") {
				n, w := setupMetalLB(run, docker, kc, ctx, step.Namespace, onLine)
				if n != "" {
					notes = append(notes, n)
				}
				if w != "" {
					notes = append(notes, w)
				}
			}
		}
		return kindChartsMsg{pane: pane, token: token, text: log.String(), notes: strings.Join(notes, "\n")}
	})
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
	out, err := shell.Combined(run, "kubectl", label...)
	onLine(string(out))
	if err != nil && strings.TrimSpace(string(out)) != "" {
		onLine(strings.TrimSpace(string(out)))
	}
	wait := []string{"wait", "--for=condition=Available", "deployment", "--all", "-n", ns, "--timeout=120s"}
	wait = append(wait, "--kubeconfig", kc, "--context", ctx)
	onLine("$ kubectl " + strings.Join(wait, " "))
	out, err = shell.Combined(run, "kubectl", wait...)
	onLine(string(out))
	if docker == "" {
		return "", "metallb: docker not in PATH, skipped IPAddressPool"
	}
	inspect := kube.DockerNetworkInspectArgs("kind")
	onLine("$ docker " + strings.Join(inspect, " "))
	out, err = shell.Combined(run, docker, inspect...)
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
		if msg.token != 0 && m.load.token != msg.token {
			return
		}
		m.load = load{}
		op := kindOp{}
		if m.vs.kop != nil {
			op = *m.vs.kop
		}
		m.showKindResult(op, msg.text, msg.notes, msg.err)
		if msg.err != nil {
			cmd = tea.Batch(m.notify(toneErr, "chart install failed"), m.openErrCmd())
			return
		}
		cmd = tea.Batch(m.notify(toneOK, "cluster "+op.name+" ready"), fetchResources(m.k, m.kubeconfig, m.context))
	})
	return cmd
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
	details := []string{"cluster " + name, "context " + kube.KindContext(name)}
	if kube.ManagedKindKubeconfig(managed, name, home) {
		details = append(details, "will remove "+config.HomePath(managed))
	}
	run := m.run
	kc := m.kubeconfig
	return m.ask(askSpec{
		title: "✗ kind delete", question: "delete kind cluster " + name + "?", details: details, danger: true,
		kind: "kind", name: name, running: "deleting " + name + "…",
		onYes: func(m model) (tea.Model, tea.Cmd) {
			spin := m.startLoad("deleting "+name, false)
			return m, tea.Batch(spin, func() tea.Msg {
				out, err := shell.Combined(run, bin, kube.KindDeleteArgs(name)...)
				if err != nil {
					text := strings.TrimSpace(string(out))
					if text == "" {
						text = err.Error()
					}
					return kindDeleteMsg{name: name, text: text, err: fmt.Errorf("%s", text)}
				}
				if kube.ManagedKindKubeconfig(managed, name, home) && filepath.Base(managed) != "config" {
					_ = os.Remove(managed)
				}
				return kindDeleteMsg{name: name, text: string(out), gone: kc == managed}
			})
		},
	}), nil
}

func (m *model) applyKindDeleted(msg kindDeleteMsg) tea.Cmd {
	var cmd tea.Cmd
	m.load = load{}
	if msg.err != nil {
		m.status = m.errLine(msg.err)
		cmd = tea.Batch(m.notify(toneErr, "kind delete failed"), m.openErrCmd())
		return cmd
	}
	if msg.gone {
		m.kubeconfig, m.context, m.namespace = "", "", ""
		m.save()
	}
	next, c := m.openKind()
	nm := next.(model)
	*m = nm
	m.status = "deleted " + msg.name
	return tea.Batch(c, m.notify(toneOK, m.status))
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

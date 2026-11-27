package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func withKindCLIs(t *testing.T) {
	t.Helper()
	lookPath = func(name string) (string, error) {
		switch name {
		case "kind", "docker", "helm", "kubectl":
			return "/usr/bin/" + name, nil
		default:
			return "", os.ErrNotExist
		}
	}
	t.Cleanup(func() { lookPath = defaultLookPath })
}

func kindFake(t *testing.T, clusters []string) *shell.Fake {
	t.Helper()
	base := appCluster(t)
	parent := base.Handler
	list := append([]string{}, clusters...)
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch r.Name {
		case "kind":
			switch {
			case line == "get clusters" || strings.HasPrefix(line, "get clusters "):
				if len(list) == 0 {
					return shell.OK("")
				}
				return shell.OK(strings.Join(list, "\n") + "\n")
			case strings.HasPrefix(line, "create cluster"):
				name := "k2"
				for i, a := range r.Args {
					if a == "--name" && i+1 < len(r.Args) {
						name = r.Args[i+1]
					}
				}
				list = append(list, name)
				return shell.OK("Creating cluster " + name + " ...\nSet kubectl context to " + kube.KindContext(name) + "\n")
			case strings.HasPrefix(line, "delete cluster"):
				name := ""
				for i, a := range r.Args {
					if a == "--name" && i+1 < len(r.Args) {
						name = r.Args[i+1]
					}
				}
				var next []string
				for _, n := range list {
					if n != name {
						next = append(next, n)
					}
				}
				list = next
				return shell.OK("Deleting cluster \"" + name + "\" ...\n")
			default:
				return shell.OK("ok")
			}
		case "docker":
			if strings.Contains(line, "network inspect") {
				return shell.OK("172.18.0.0/16 ")
			}
			if len(r.Args) > 0 && r.Args[0] == "ps" {
				var b strings.Builder
				for _, name := range list {
					b.WriteString(name + "\tcontrol-plane\tkindest/node:v1.33.1\trunning\tUp 1 hour\n")
				}
				return shell.OK(b.String())
			}
			return shell.OK("")
		case "helm":
			switch {
			case strings.HasPrefix(line, "repo add") || strings.HasPrefix(line, "repo update"):
				return shell.OK("ok")
			case strings.HasPrefix(line, "search repo"):
				return shell.OK(searchOut)
			case strings.HasPrefix(line, "install ") || strings.HasPrefix(line, "upgrade ") || strings.HasPrefix(line, "template "):
				return shell.OK("NAME: " + line + "\nSTATUS: deployed\nNOTES:\ninstalled\n")
			default:
				return shell.OK("ok")
			}
		case "kubectl":
			if strings.Contains(line, "label") && strings.Contains(line, "namespace") {
				return shell.OK("namespace/metallb-system labeled")
			}
			if strings.Contains(line, "wait") {
				return shell.OK("deployment.apps/controller condition met")
			}
			if len(r.Args) > 0 && r.Args[0] == "apply" {
				return shell.OK("ipaddresspool.metallb.io/k2-kind created\nl2advertisement.metallb.io/k2-kind created")
			}
		}
		return parent(r)
	}}
}

func selectKindTile(t *testing.T, m model, name string) model {
	t.Helper()
	m.selectKind(name)
	got, ok := m.selectedKind()
	if !ok || got != name {
		t.Fatalf("no tile %q (sel=%d got=%q)", name, m.vs.kindSel, got)
	}
	return m
}

func kindJobLog(m model, name string) string {
	if j := m.kindJobs[name]; j != nil {
		return j.log
	}
	return ""
}

func kindCalls(f *shell.Fake, verb string) []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Name != "kind" {
			continue
		}
		line := strings.Join(c.Args, " ")
		if verb == "" || strings.HasPrefix(line, verb) {
			out = append(out, line)
		}
	}
	return out
}

func TestKindListView(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig("k2", home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = path, "kind-k2"
	m = open(t, m, "kind")
	if m.stage != "kind" {
		t.Fatalf("stage %s status %q", m.stage, m.status)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"k2", "dev", "~/.kube/k2", "ACTIVE", "running", "1 cp", "0 workers", "v1.33.1", kindIconRunning, kindIconNodes, kindIconKube, kindIconActive, kindIconSel} {
		if !strings.Contains(view, want) {
			t.Fatalf("tiles lack %q:\n%s", want, view)
		}
	}
	for _, want := range []string{kindIconNodes + "  1 cp", kindIconKube + "  ~/.kube/k2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("icon spacing lacks %q:\n%s", want, view)
		}
	}
	leg := ansi.Strip(m.legend())
	for _, want := range []string{"A create", "enter use / logs", "^d delete", "e charts", "r refresh"} {
		if !strings.Contains(leg, want) {
			t.Fatalf("legend lacks %q: %q", want, leg)
		}
	}
	if !strings.Contains(view, "kind") {
		t.Fatalf("title missing:\n%s", view)
	}
}

func TestKindCreateWizardAndStream(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"kind create", "name", "control-planes", "workers", "config", "edit config", "image", "istio", "metallb", "kafbat"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form lacks %q:\n%s", want, view)
		}
	}
	if helmFieldValue(m.dlg, "name") != "k2" {
		t.Fatalf("default name %q", helmFieldValue(m.dlg, "name"))
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "create kind cluster k2?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{"cluster k2", "context kind-k2", "kubeconfig ~/.kube/k2", "1 control-plane"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "enter")
	if len(kindCalls(f, "create cluster")) != 0 {
		t.Fatalf("enter created: %v", kindCalls(f, "create cluster"))
	}
	m = pressSettle(t, m, "y")
	got := kindCalls(f, "create cluster")
	if len(got) != 1 {
		t.Fatalf("create calls %v", got)
	}
	home := os.Getenv("HOME")
	wantKC := kube.KindKubeconfig("k2", home)
	if !strings.Contains(got[0], "--name k2") || !strings.Contains(got[0], "--kubeconfig "+wantKC) || !strings.Contains(got[0], "--config ") || !strings.Contains(got[0], "--wait 5m") {
		t.Fatalf("create args %q", got[0])
	}
	if m.kubeconfig != wantKC || m.context != "kind-k2" {
		t.Fatalf("switched to %s %s", m.kubeconfig, m.context)
	}
	if m.stage != "kind" {
		t.Fatalf("stage %s status %q", m.stage, m.status)
	}
	if j := m.kindJobs["k2"]; j == nil || (j.status != "running" && j.status != "deploying") {
		t.Fatalf("job %+v", j)
	}
	if !strings.Contains(kindJobLog(m, "k2"), "Creating cluster") && !strings.Contains(m.status, "created k2") {
		t.Fatalf("status %q log %s", m.status, kindJobLog(m, "k2"))
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "k2") || (!strings.Contains(view, kindIconRunning) && !strings.Contains(view, kindIconDeploy)) {
		t.Fatalf("tile missing after create:\n%s", view)
	}
}

func TestKindCreateCommandPrefillAndNamedKubeconfig(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	next, cmd := m.runCommand("kind create lab --control-planes 3 --workers 2 --image kindest/node:v1.31.0")
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v status %q", m.dlg, m.status)
	}
	if helmFieldValue(m.dlg, "name") != "lab" || helmFieldValue(m.dlg, "planes") != "3" || helmFieldValue(m.dlg, "workers") != "2" || helmFieldValue(m.dlg, "image") != "kindest/node:v1.31.0" {
		t.Fatalf("prefill name=%s planes=%s workers=%s image=%s", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "planes"), helmFieldValue(m.dlg, "workers"), helmFieldValue(m.dlg, "image"))
	}
	next, cmd = m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	home := os.Getenv("HOME")
	wantKC := kube.KindKubeconfig("lab", home)
	for _, want := range []string{"cluster lab", "3 control-planes, 2 workers", "kubeconfig " + config.HomePath(wantKC), "kindest/node:v1.31.0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "y")
	got := kindCalls(f, "create cluster")
	if len(got) != 1 || !strings.Contains(got[0], "--name lab") || !strings.Contains(got[0], "--kubeconfig "+wantKC) {
		t.Fatalf("create args %v", got)
	}
	if filepath.Base(wantKC) != "k2-lab" {
		t.Fatalf("kubeconfig name %s", wantKC)
	}
	if m.kubeconfig != wantKC || m.context != "kind-lab" {
		t.Fatalf("switched to %s %s", m.kubeconfig, m.context)
	}
}

func TestKindCreateInstallsCatalogChartsAndMetalLB(t *testing.T) {
	withKindCLIs(t)
	metalRetryWait, metalRetryN = 0, 1
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	setHelmField(m.dlg, "name", "k2")
	for i := range m.dlg.hfields {
		if m.dlg.hfields[i].key == "c:metallb" {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "charts metallb") {
		t.Fatalf("confirm lacks charts:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	helms := helmCalls(f)
	var added, installed bool
	for _, c := range helms {
		if strings.HasPrefix(c, "repo add metallb ") {
			added = true
		}
		if strings.HasPrefix(c, "install metallb metallb/metallb") && strings.Contains(c, "-n metallb-system") && strings.Contains(c, "--create-namespace") {
			installed = true
		}
	}
	if !added || !installed {
		t.Fatalf("helm calls %v", helms)
	}
	var docker, apply bool
	for _, c := range f.Calls() {
		line := strings.Join(c.Args, " ")
		if c.Name == "docker" && strings.Contains(line, "network inspect") {
			docker = true
		}
		if c.Name == "kubectl" && len(c.Args) > 0 && c.Args[0] == "apply" && strings.Contains(c.Stdin, "IPAddressPool") && strings.Contains(c.Stdin, "172.18.255.200") && strings.Contains(c.Stdin, "172.18.255.250") && strings.Contains(c.Stdin, "L2Advertisement") {
			apply = true
		}
	}
	if !docker || !apply {
		t.Fatalf("metallb setup docker=%v apply=%v calls=%v", docker, apply, callSummary(f))
	}
	log := m.logText + kindJobLog(m, "k2")
	if !strings.Contains(log, "172.18.255.200") && !strings.Contains(log, "metallb pool") {
		t.Fatalf("missing pool in output:\n%s", log)
	}
}

func callSummary(f *shell.Fake) []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, c.Name+" "+strings.Join(c.Args, " "))
	}
	return out
}

func TestKindEnterUsesClusterAndDeleteRemovesManagedKubeconfig(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	managed := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(managed, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(home, ".kube", "config")
	if err := os.WriteFile(def, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "enter")
	if m.kubeconfig != managed || m.context != "kind-lab" {
		t.Fatalf("switch %s %s status %q", m.kubeconfig, m.context, m.status)
	}
	if m.stage != "kind" {
		t.Fatalf("enter should stay on kind, stage %s status %q", m.stage, m.status)
	}
	m = pressSettle(t, m, "ctrl+d")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "delete kind cluster lab?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "will remove ~/.kube/k2-lab") {
		t.Fatalf("confirm lacks managed path:\n%s", view)
	}
	if strings.Contains(view, "~/.kube/config") && strings.Contains(view, "will remove ~/.kube/config") {
		t.Fatalf("must not offer to remove ~/.kube/config:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	got := kindCalls(f, "delete cluster")
	if len(got) != 1 || !strings.Contains(got[0], "--name lab") {
		t.Fatalf("delete args %v", got)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed kubeconfig still there: %v", err)
	}
	if _, err := os.Stat(def); err != nil {
		t.Fatalf("~/.kube/config was removed: %v", err)
	}
}

func TestKindReadOnlyBlocksCreateAndDelete(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	m := appModel(t, f)
	m = open(t, m, "kind")
	m.readOnly = true
	if got := press(t, m, "A"); got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("create: dlg %v status %q", got.dlg != nil, got.status)
	}
	m = selectKindTile(t, m, "k2")
	if got := press(t, m, "ctrl+d"); got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("delete: dlg %v status %q", got.dlg != nil, got.status)
	}
	next, cmd := m.runCommand("kind create other")
	got := settle(t, next.(model), cmd)
	if got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("cmdline: dlg %v status %q", got.dlg != nil, got.status)
	}
}

func TestKindNeedsCLI(t *testing.T) {
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = defaultLookPath })
	f := kindFake(t, nil)
	m := appModel(t, f)
	next, cmd := m.runCommand("kind")
	m = settle(t, next.(model), cmd)
	if !strings.Contains(m.status, "kind CLI") {
		t.Fatalf("status %q", m.status)
	}
	if len(kindCalls(f, "")) != 0 {
		t.Fatal("kind was invoked without a binary")
	}
}

func TestKindCreateNeedsDocker(t *testing.T) {
	lookPath = func(name string) (string, error) {
		if name == "kind" || name == "helm" {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPath = defaultLookPath })
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	got := pressSettle(t, m, "A")
	if got.dlg != nil || !strings.Contains(got.status, "docker") {
		t.Fatalf("dlg %v status %q", got.dlg != nil, got.status)
	}
}

func TestParseKindCmd(t *testing.T) {
	op, ok, err := parseKindCmd("create lab --control-planes 2 --workers 3 --image kindest/node:v1.31.0 --charts metallb,istio")
	if !ok || err != "" || op.name != "lab" || op.planes != 2 || op.workers != 3 || op.image != "kindest/node:v1.31.0" || strings.Join(op.charts, ",") != "metallb,istio" {
		t.Fatalf("%+v ok=%v err=%q", op, ok, err)
	}
	op, ok, err = parseKindCmd("create")
	if !ok || err != "" || op.name != "k2" || op.planes != 1 || op.workers != 0 {
		t.Fatalf("defaults %+v ok=%v err=%q", op, ok, err)
	}
	if _, ok, _ := parseKindCmd("list"); ok {
		t.Fatal("list is not create")
	}
	if _, ok, msg := parseKindCmd("create --workers no"); ok || msg == "" {
		t.Fatalf("bad workers ok=%v msg=%q", ok, msg)
	}
}

func TestKindCreateStreamRunsWithoutCommandContextError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "kind")
	script := "#!/bin/sh\necho Creating cluster \"k2\" ...\necho Set kubectl context to kind-k2\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testEnv(t)
	m := initialModel(Options{Runner: shell.Exec{}, Config: &cfg})
	t.Cleanup(m.streams.stopAll)
	m.width, m.height = 120, 40
	m.cfg.Kind.Binary = bin
	op := kindOp{name: "k2", planes: 1, kubeconfig: filepath.Join(dir, "kubeconfig")}
	m.load.token = 1
	cmd := m.kindCreateStream(op)
	if cmd == nil {
		t.Fatal("no command")
	}
	var run kindRunMsg
	got := false
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 50 {
			t.Fatal("too many steps")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		switch msg := msg.(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case progressMsg:
			if msg.ch != nil {
				queue = append(queue, listen(msg.ch))
			}
		case kindLineMsg:
			if msg.ch != nil {
				queue = append(queue, listen(msg.ch))
			}
		case kindRunMsg:
			run, got = msg, true
			queue = nil
		}
	}
	if !got {
		t.Fatal("no kindRunMsg")
	}
	if run.err != nil {
		t.Fatalf("kind create stream: %v", run.err)
	}
	if !strings.Contains(run.text, "Creating cluster") {
		t.Fatalf("output %q", run.text)
	}
}

func TestKindEditConfigOpensGeneratedYAML(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd { opened = path; return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })
	found := false
	for i, field := range m.dlg.hfields {
		if field.key == "editcfg" {
			m.dlg.idx = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no edit config field")
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if opened == "" {
		t.Fatal("editor was not opened")
	}
	raw, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kind.x-k8s.io/v1alpha4") {
		t.Fatalf("generated config:\n%s", raw)
	}
	next, cmd = m.Update(kindConfigDoneMsg{})
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v", m.dlg)
	}
	if helmFieldValue(m.dlg, "config") != opened {
		t.Fatalf("config %q want %q", helmFieldValue(m.dlg, "config"), opened)
	}
	if m.vs.kop == nil || !m.vs.kop.edited {
		t.Fatalf("edited flag %+v", m.vs.kop)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "(edited)") && !strings.Contains(view, filepath.Base(opened)) {
		t.Fatalf("missing edited mark:\n%s", view)
	}
}

func TestKindChartsEditorAndLiveLogs(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "e")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("charts form %+v status %q", m.dlg, m.status)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"charts", "istio", "metallb"} {
		if !strings.Contains(view, want) {
			t.Fatalf("charts form lacks %q:\n%s", want, view)
		}
	}
	for i := range m.dlg.hfields {
		if m.dlg.hfields[i].key == "c:metallb" {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindCharts()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "apply chart changes") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	if !strings.Contains(ansi.Strip(m.View()), "install metallb") {
		t.Fatalf("confirm lacks install:\n%s", ansi.Strip(m.View()))
	}
	m = pressSettle(t, m, "y")
	helms := helmCalls(f)
	var installed bool
	for _, c := range helms {
		if strings.HasPrefix(c, "install metallb metallb/metallb") {
			installed = true
		}
	}
	if !installed {
		t.Fatalf("helm calls %v", helms)
	}
	if j := m.kindJobs["lab"]; j == nil || j.status == "failed" {
		t.Fatalf("job %+v", j)
	}
	if m.kubeconfig == path {
		t.Fatal("chart edit should not switch the active kubeconfig")
	}

	m = open(t, m, "kind")
	m.kindJobs = map[string]*kindJob{"lab": {name: "lab", status: "deploying", step: "creating cluster", last: "Creating cluster lab", log: "Creating cluster lab ...\n", started: now()}}
	m.selectKind("lab")
	blocked := pressSettle(t, m, "ctrl+d")
	if blocked.dlg != nil && blocked.dlg.ask != "" {
		t.Fatalf("delete during deploy: %+v", blocked.dlg)
	}
	if !strings.Contains(blocked.status, "deploying") {
		t.Fatalf("delete block status %q", blocked.status)
	}
	m = pressSettle(t, m, "enter")
	if m.stage != "kindlog" {
		t.Fatalf("logs stage %s status %q", m.stage, m.status)
	}
	if !strings.Contains(m.logText, "Creating cluster lab") {
		t.Fatalf("log text %q", m.logText)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "kind" {
		t.Fatalf("back stage %s", m.stage)
	}
	got, _ := m.selectedKind()
	if got != "lab" {
		t.Fatalf("selection after logs %q", got)
	}
}

func TestKindTildeOpensKind(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	m := appModel(t, f)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Kind") {
		t.Fatalf("home nav lacks Kind:\n%s", view)
	}
	if !strings.Contains(view, ":kind") && !strings.Contains(view, "kind clusters") {
		t.Fatalf("home hint lacks kind:\n%s", view)
	}
	m = pressSettle(t, m, "~")
	if m.stage != "kind" {
		t.Fatalf("~ stage %s status %q", m.stage, m.status)
	}
	if m.cfg.Keys.Kind.Label() != "~" || m.cfg.Keys.Home.Label() != "" {
		t.Fatalf("defaults kind=%q home=%q", m.cfg.Keys.Kind.Label(), m.cfg.Keys.Home.Label())
	}
}

func TestKindTilesFitPane(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev", "lab", "other"})
	m := appModel(t, f)
	m = open(t, m, "kind")
	for _, w := range []int{36, 48, 60, 80, 100, 120} {
		body := m.kindBody(w)
		for i, line := range strings.Split(body, "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Fatalf("width %d line %d is %d:\n%s", w, i, got, ansi.Strip(line))
			}
		}
	}
}

func TestKindSelectedBorderDiffersFromActive(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig("k2", home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = path, "kind-k2"
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "dev")
	view := ansi.Strip(m.kindBody(80))
	if !strings.Contains(view, kindIconSel+"  dev") {
		t.Fatalf("missing selection marker:\n%s", view)
	}
	if !strings.Contains(view, "ACTIVE") || strings.Contains(view, kindIconSel+"  k2") {
		t.Fatalf("ACTIVE should stay on k2 without the cursor mark:\n%s", view)
	}
	if !strings.Contains(view, "┏") && !strings.Contains(view, "╔") {
		t.Fatalf("selected tile lacks a thick/double border:\n%s", view)
	}
	if !strings.Contains(view, "╭") {
		t.Fatalf("unselected ACTIVE tile should keep a rounded muted border:\n%s", view)
	}
}

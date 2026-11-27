package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func init() {
	fetchKindNodeImages = func() ([]string, error) { return nil, nil }
}

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
					b.WriteString(name + "\tcontrol-plane\tkindest/node:v1.33.1\trunning\tUp 1 hour\tid" + name + "aaaaaaaa\n")
				}
				return shell.OK(b.String())
			}
			if len(r.Args) > 0 && r.Args[0] == "stats" {
				var b strings.Builder
				for _, name := range list {
					b.WriteString("id" + name + "aaaaaaaa\t" + name + "-control-plane\t4.20%\t1.400GiB / 8.000GiB\t17.50%\n")
				}
				return shell.OK(b.String())
			}
			if len(r.Args) > 0 && r.Args[0] == "logs" {
				return shell.OK("I0000 starting kubelet\nI0001 apiserver is ready\n")
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
			if (strings.Contains(line, "get pods") || strings.HasPrefix(line, "get pods")) && strings.Contains(line, "--no-headers") {
				ctx := "k2"
				for i, a := range r.Args {
					if a == "--context" && i+1 < len(r.Args) {
						ctx = r.Args[i+1]
					}
				}
				return shell.OK(kindFakePodTable(strings.TrimPrefix(ctx, "kind-")))
			}
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

func kindFakePodTable(name string) string {
	ready, total := 14, 14
	if name == "dev" {
		ready, total = 12, 14
	}
	var b strings.Builder
	for i := 0; i < total; i++ {
		frac := "1/1"
		if i >= ready {
			frac = "0/1"
		}
		b.WriteString("ns  pod-" + name + "-" + strconv.Itoa(i) + "  " + frac + "  Running  0  1h\n")
	}
	return b.String()
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

func kindViewHasSpin(view string) bool {
	for _, f := range kindSpinFrames() {
		if strings.Contains(view, f) {
			return true
		}
	}
	return false
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
	for _, want := range []string{"k2", "dev", "~/.kube/k2", "running", "up 1h", "k8s", "v1.33.1", "control-plane", "workers", "pods", "CPU", "4.2%", "RAM", "1.4 GiB", kindIconRunning, kindIconKube, kindIconActive, kindIconIdle, kindIconSel} {
		if !strings.Contains(view, want) {
			t.Fatalf("tiles lack %q:\n%s", want, view)
		}
	}
	for _, want := range []string{"k8s            v1.33.1", "control-plane  1", "workers        0", "pods           14", kindIconKube + "  ~/.kube/k2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tile lines lack %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "k2 "+kindIconActive+" active") {
		t.Fatalf("active card should show ⬢ active next to the name:\n%s", view)
	}
	if strings.Contains(view, "ACTIVE") || strings.Contains(view, "1 cp") {
		t.Fatalf("old compact node line or ACTIVE label still present:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "dev") && strings.Contains(line, "active") && !strings.Contains(line, "k2") {
			t.Fatalf("idle card should not show active:\n%s", line)
		}
	}
	if strings.Contains(view, "●") || strings.Contains(view, "○") {
		t.Fatalf("still using round status dots:\n%s", view)
	}
	leg := ansi.Strip(m.legend())
	for _, want := range []string{"A create", "enter logs", "space use", "^d delete", "e charts", "r refresh"} {
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
	for _, want := range []string{"kind create", "name", "control-planes", "workers", "config", "image", "istio", "metallb", "kafbat"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form lacks %q:\n%s", want, view)
		}
	}
	if helmFieldValue(m.dlg, "name") != "k2" {
		t.Fatalf("default name %q", helmFieldValue(m.dlg, "name"))
	}
	if helmFieldValue(m.dlg, "config") == "" {
		t.Fatal("config should be prefilled with the generated path")
	}
	for _, f := range m.dlg.hfields {
		if f.key == "editcfg" {
			t.Fatal("edit config row should be gone")
		}
	}
	if strings.Contains(ansi.Strip(m.View()), "edit config") {
		t.Fatalf("edit config still shown:\n%s", ansi.Strip(m.View()))
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "create kind cluster k2?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{kindDetail("cluster", "k2"), kindDetail("context", "kind-k2"), kindDetail("kubeconfig", "~/.kube/k2"), kindDetail("config", config.HomePath(kindConfigPath("k2"))), kindDetail("nodes", "1 control-plane")} {
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
	if !strings.Contains(view, "k2") || (!strings.Contains(view, kindIconRunning) && !kindViewHasSpin(view)) {
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
	for _, want := range []string{kindDetail("cluster", "lab"), kindDetail("config", config.HomePath(kindConfigPath("lab"))), kindDetail("kubeconfig", config.HomePath(wantKC)), kindDetail("nodes", "3 control-planes, 2 workers"), "kindest/node:v1.31.0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "y")
	got := kindCalls(f, "create cluster")
	if len(got) != 1 || !strings.Contains(got[0], "--name lab") || !strings.Contains(got[0], "--kubeconfig "+wantKC) {
		t.Fatalf("create args %v", got)
	}
	if strings.Contains(got[0], "--image") {
		t.Fatalf("create should not pass --image when the config already sets it: %q", got[0])
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
	if !strings.Contains(view, kindDetail("charts", "metallb")) {
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
	m = pressSettle(t, m, " ")
	if m.kubeconfig != managed || m.context != "kind-lab" {
		t.Fatalf("switch %s %s status %q", m.kubeconfig, m.context, m.status)
	}
	if m.stage != "kind" {
		t.Fatalf("space should stay on kind, stage %s status %q", m.stage, m.status)
	}
	m = pressSettle(t, m, "ctrl+d")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "delete kind cluster lab?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, kindDetail("will remove", "~/.kube/k2-lab")) {
		t.Fatalf("confirm lacks managed path:\n%s", view)
	}
	if strings.Contains(view, "~/.kube/config") && strings.Contains(view, kindDetail("will remove", "~/.kube/config")) {
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
	if m.kubeconfig != "" || m.context != "" {
		t.Fatalf("kubeconfig should clear when no cluster remains: %s %s", m.kubeconfig, m.context)
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
	cfg := helmFieldValue(m.dlg, "config")
	if cfg == "" {
		t.Fatal("config should be prefilled")
	}
	m = focusKindField(t, m, "config")
	m = pressSettle(t, m, "ctrl+e")
	if opened == "" {
		t.Fatal("editor was not opened")
	}
	if opened != cfg {
		t.Fatalf("opened %q want prefilled %q", opened, cfg)
	}
	raw, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kind.x-k8s.io/v1alpha4") {
		t.Fatalf("generated config:\n%s", raw)
	}
	next, cmd := m.Update(kindConfigDoneMsg{})
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
	if !strings.Contains(ansi.Strip(m.View()), kindDetail("install", "metallb")) {
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
	if !strings.Contains(view, kindIconActive) || strings.Contains(view, kindIconSel+"  k2") {
		t.Fatalf("active hexagon should stay on k2 without the cursor mark:\n%s", view)
	}
	if !strings.Contains(view, kindIconIdle) {
		t.Fatalf("unselected tile should show the idle hexagon:\n%s", view)
	}
	if strings.ContainsAny(view, "┏┓┗┛╔╗╚╝") {
		t.Fatalf("tiles should use rounded corners like the rest of k2:\n%s", view)
	}
	if !strings.Contains(view, "╭") {
		t.Fatalf("tiles should use a rounded border:\n%s", view)
	}
}

func TestKindMiniBar(t *testing.T) {
	if got := kindMiniBar(0, 5); got != "□□□□□" {
		t.Fatalf("empty %q", got)
	}
	if got := kindMiniBar(100, 5); got != "■■■■■" {
		t.Fatalf("full %q", got)
	}
	if got := kindMiniBar(40, 5); got != "■■□□□" {
		t.Fatalf("40%% %q", got)
	}
	if got := kindMiniBar(1, 5); got != "■□□□□" {
		t.Fatalf("tiny %q", got)
	}
}

func TestKindUsageAndChartsOnTile(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	m := appModel(t, f)
	m.vs.kindNames = []string{"k2"}
	m.vs.kindNodes = map[string]kube.KindNodes{"k2": {Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 2h"}}
	m.kindUsage = map[string]kube.KindUsage{"k2": {CPU: 4.2, MemUsed: 1503238554, MemPct: 40, OK: true}}
	m.kindCatalog = map[string][]string{"k2": {"metallb", "argocd"}}
	m.stage = "kind"
	view := ansi.Strip(m.kindBody(80))
	for _, want := range []string{kindIconRunning, "running · up 2h", "k8s            v1.33.1", "control-plane  1", "workers        0", "pods           —", "CPU  4.2%", "■□□□□", "RAM  1.4 GiB (40%)", "■■□□□", kindIconCharts + "  metallb, argocd"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tile lacks %q:\n%s", want, view)
		}
	}
	m.kindPods = map[string]kube.KindPods{"k2": {Running: 12, Total: 14, OK: true}}
	view = ansi.Strip(m.kindBody(80))
	if !strings.Contains(view, "pods           12/14") {
		t.Fatalf("ready/total pods:\n%s", view)
	}
	m.kindCatalog["k2"] = nil
	view = ansi.Strip(m.kindBody(80))
	if !strings.Contains(view, kindIconCharts+"  none") {
		t.Fatalf("empty catalog:\n%s", view)
	}
	m.vs.kindNodes["k2"] = kube.KindNodes{Planes: 1, Running: 0, Total: 1, Version: "v1.33.1"}
	m.kindUsage = nil
	view = ansi.Strip(m.kindBody(80))
	if !strings.Contains(view, kindIconStopped) || !strings.Contains(view, "stopped") || !strings.Contains(view, "CPU  —") || !strings.Contains(view, "RAM  —") || !strings.Contains(view, "pods           —") {
		t.Fatalf("stopped tile:\n%s", view)
	}
}

func focusKindField(t *testing.T, m model, key string) model {
	t.Helper()
	if m.dlg == nil {
		t.Fatal("no form")
	}
	for i, f := range m.dlg.hfields {
		if f.key == key {
			m.dlg.idx = i
			return m.moveHelmField(0)
		}
	}
	t.Fatalf("no field %s", key)
	return m
}

func TestKindChartsWrapOnCard(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	m := appModel(t, f)
	m.vs.kindNames = []string{"k2", "dev"}
	nodes := kube.KindNodes{Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 2h"}
	m.vs.kindNodes = map[string]kube.KindNodes{"k2": nodes, "dev": nodes}
	u := kube.KindUsage{CPU: 4.2, MemUsed: 1503238554, MemPct: 40, OK: true}
	m.kindUsage = map[string]kube.KindUsage{"k2": u, "dev": u}
	charts := []string{"istio", "sealed-secrets", "metallb", "argocd", "cert-manager", "metrics-server", "kube-prometheus-stack", "kafbat"}
	m.kindCatalog = map[string][]string{"k2": charts, "dev": {"metallb"}}
	m.stage = "kind"
	tiles := m.kindTiles()
	if len(tiles) != 2 {
		t.Fatalf("tiles %d", len(tiles))
	}
	tileW := kindCardWidth(80)
	inner := max(8, tileW-2)
	c1 := m.kindTileContent(tiles[0], false, inner)
	c2 := m.kindTileContent(tiles[1], false, inner)
	if len(c1) <= len(c2) {
		t.Fatalf("expected wrapped charts to add lines: k2=%d dev=%d\n%q\n%q", len(c1), len(c2), c1, c2)
	}
	view := ansi.Strip(m.kindBody(80))
	for _, name := range charts {
		if !strings.Contains(view, name) {
			t.Fatalf("wrapped card missing %q:\n%s", name, view)
		}
	}
	if strings.Contains(view, "…") {
		t.Fatalf("catalog list should wrap, not ellipsize:\n%s", view)
	}
	if strings.Count(view, "╭") != 2 {
		t.Fatalf("expected 2 stacked cards:\n%s", view)
	}
	if strings.Index(view, "kube-prometheus-stack") > strings.Index(view, "│dev") && strings.Contains(view, "│dev") {
		t.Fatalf("k2 card should appear above dev:\n%s", view)
	}
}

func TestKindDeleteSwitchesToRemainingCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	labKC := kube.KindKubeconfig("lab", home)
	devKC := kube.KindKubeconfig("dev", home)
	for _, p := range []string{labKC, devKC} {
		if err := os.WriteFile(p, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, " ")
	if m.kubeconfig != labKC || m.context != "kind-lab" {
		t.Fatalf("switch %s %s", m.kubeconfig, m.context)
	}
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "ctrl+d", "y")
	if m.kubeconfig != devKC || m.context != "kind-dev" {
		t.Fatalf("auto-switch %s %s status %q", m.kubeconfig, m.context, m.status)
	}
	if name, ok := m.selectedKind(); !ok || name != "dev" {
		t.Fatalf("selected %q %v", name, ok)
	}
	if _, err := os.Stat(labKC); !os.IsNotExist(err) {
		t.Fatalf("lab kubeconfig still there: %v", err)
	}
	if _, err := os.Stat(devKC); err != nil {
		t.Fatalf("dev kubeconfig gone: %v", err)
	}
}

func TestKindChartValuesInCreateAndInstall(t *testing.T) {
	withKindCLIs(t)
	metalRetryWait, metalRetryN = 0, 1
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	m = focusKindField(t, m, "c:metallb")
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd { opened = path; return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })
	m = pressSettle(t, m, "v")
	if opened == "" {
		t.Fatal("editor was not opened")
	}
	raw, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "custom values for metallb") {
		t.Fatalf("seed:\n%s", raw)
	}
	if err := os.WriteFile(opened, append(raw, []byte("replicaCount: 2\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(kindValuesDoneMsg{})
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "metallb (values)") {
		t.Fatalf("missing values badge:\n%s", view)
	}
	on := false
	for _, field := range m.dlg.hfields {
		if field.key == "c:metallb" {
			on = field.on
		}
	}
	if !on {
		t.Fatal("metallb should be checked")
	}
	if m.vs.kop == nil || m.vs.kop.values["metallb"] != opened {
		t.Fatalf("values path %+v", m.vs.kop)
	}
	next, cmd = m.submitKindForm()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "y")
	var got string
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "install metallb ") {
			got = c
		}
	}
	if got == "" || !strings.Contains(got, "-f "+opened) {
		t.Fatalf("install should pass custom values, got %v", helmCalls(f))
	}
	if _, err := os.Stat(opened); !os.IsNotExist(err) {
		t.Fatalf("values file should be cleaned up: %v", err)
	}
}

func TestKindChartValuesUpgradeOnExisting(t *testing.T) {
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
	vf, err := os.CreateTemp("", "k2-metallb-values-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vf.WriteString("replicaCount: 3\n"); err != nil {
		t.Fatal(err)
	}
	vf.Close()
	m = open(t, m, "kind")
	op := kindOp{
		name: "lab", kubeconfig: path, charts: []string{"metallb"}, form: "kindcharts",
		values: map[string]string{"metallb": vf.Name()},
	}
	d := m.newDialog("kindcharts", "charts · lab", "kind", "", "lab")
	m.fillKindChartsForm(d, &op)
	m.vs.kop = &op
	m.dlg = d
	next, cmd := m.submitKindCharts()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, kindDetail("upgrade", "metallb (values)")) {
		t.Fatalf("confirm:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var got string
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, "upgrade metallb ") {
			got = c
		}
	}
	if got == "" || !strings.Contains(got, "-f "+vf.Name()) {
		t.Fatalf("upgrade should pass values, got %v", helmCalls(f))
	}
	if _, err := os.Stat(vf.Name()); !os.IsNotExist(err) {
		t.Fatalf("values file should be cleaned up: %v", err)
	}
}

func TestKindChartValuesInChartsForm(t *testing.T) {
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
		t.Fatalf("form %+v", m.dlg)
	}
	m = focusKindField(t, m, "c:metallb")
	var opened string
	old := vimCommand
	vimCommand = func(p string) *exec.Cmd { opened = p; return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })
	m = pressSettle(t, m, "v")
	if opened == "" {
		t.Fatal("editor was not opened")
	}
	next, cmd := m.Update(kindValuesDoneMsg{})
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("reopen %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "metallb (values)") || !strings.Contains(view, "v values") {
		t.Fatalf("charts form:\n%s", view)
	}
}

func TestKindNodeImagePicker(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	images := []string{"kindest/node:v1.34.0", "kindest/node:v1.33.1", "kindest/node:v1.32.2", "kindest/node:v1.31.4", "kindest/node:v1.30.10"}
	m.kindNodeImages = images
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	if helmFieldValue(m.dlg, "image") != images[0] {
		t.Fatalf("default image %q", helmFieldValue(m.dlg, "image"))
	}
	m = focusKindField(t, m, "image")
	view := ansi.Strip(m.View())
	for _, img := range images {
		if !strings.Contains(view, img) {
			t.Fatalf("prefilled image should still list all tags, missing %q:\n%s", img, view)
		}
	}
	if !strings.Contains(view, "cycle images") {
		t.Fatalf("footer:\n%s", view)
	}
	if strings.Contains(view, "space toggle") || strings.Contains(view, " values") && strings.Contains(view, "v values") {
		t.Fatalf("footer should not show chart hints on image:\n%s", view)
	}
	m = pressSettle(t, m, "tab")
	if helmFieldValue(m.dlg, "image") != images[1] {
		t.Fatalf("tab next %q", helmFieldValue(m.dlg, "image"))
	}
	m = pressSettle(t, m, "shift+tab")
	if helmFieldValue(m.dlg, "image") != images[0] {
		t.Fatalf("shift+tab prev %q", helmFieldValue(m.dlg, "image"))
	}
	for i := 0; i < len(images)-1; i++ {
		m = pressSettle(t, m, "tab")
	}
	if helmFieldValue(m.dlg, "image") != images[len(images)-1] {
		t.Fatalf("last %q", helmFieldValue(m.dlg, "image"))
	}
	m = pressSettle(t, m, "tab")
	if helmFieldValue(m.dlg, "image") != images[0] {
		t.Fatalf("wrap %q", helmFieldValue(m.dlg, "image"))
	}
	start := m.dlg.idx
	m = pressSettle(t, m, "down")
	if m.dlg.idx == start {
		t.Fatal("down should leave the image field")
	}
	m = pressSettle(t, m, "up")
	if m.dlg.hfields[m.dlg.idx].key != "image" {
		t.Fatalf("up should return to image, at %s", m.dlg.hfields[m.dlg.idx].key)
	}
	setHelmField(m.dlg, "image", "v1.32")
	m.syncKindImageSuggest()
	if !containsStr(m.dlg.cands, "kindest/node:v1.32.2") || len(m.dlg.cands) != 1 {
		t.Fatalf("typed filter %v", m.dlg.cands)
	}
}

func TestKindNodeImagesFillEmptyField(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	if helmFieldValue(m.dlg, "image") != "" {
		t.Fatalf("offline default %q", helmFieldValue(m.dlg, "image"))
	}
	next, cmd := m.Update(kindNodeImagesMsg{images: []string{"kindest/node:v1.34.0", "kindest/node:v1.33.1"}})
	m = settle(t, next.(model), cmd)
	if helmFieldValue(m.dlg, "image") != "kindest/node:v1.34.0" {
		t.Fatalf("filled %q", helmFieldValue(m.dlg, "image"))
	}
}

func TestKindChartValueFilesOrder(t *testing.T) {
	custom, err := os.CreateTemp("", "k2-custom-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	custom.Close()
	t.Cleanup(func() { os.Remove(custom.Name()) })
	files, temps, err := kindChartValueFiles(config.Chart{Values: map[string]any{"foo": "bar"}}, custom.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[1] != custom.Name() || len(temps) != 1 || temps[0] == custom.Name() {
		t.Fatalf("files %v temps %v", files, temps)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "foo:") {
		t.Fatalf("catalog values:\n%s", raw)
	}
	os.Remove(files[0])
}

func widthSuffix(s string, n int) string {
	s = ansi.Strip(s)
	w := ansi.StringWidth(s)
	if w <= n {
		return s
	}
	skip := w - n
	var b strings.Builder
	seen := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if seen < skip {
			seen += rw
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestKindActiveBadgeCorner(t *testing.T) {
	withKindCLIs(t)
	longName := "this-is-a-very-long-kind-cluster-name"
	f := kindFake(t, []string{longName, "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig(longName, home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = path, kube.KindContext(longName)
	m.vs.kindNames = []string{longName, "dev"}
	nodes := kube.KindNodes{Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 2h"}
	m.vs.kindNodes = map[string]kube.KindNodes{longName: nodes, "dev": nodes}
	m.stage = "kind"
	tiles := m.kindTiles()
	if len(tiles) != 2 || !tiles[0].active || tiles[1].active {
		t.Fatalf("tiles %+v", tiles)
	}
	tileW := kindCardWidth(40)
	inner := max(8, tileW-2)
	active := m.kindTileContent(tiles[0], true, inner)
	idle := m.kindTileContent(tiles[1], false, inner)
	if len(active) < 2 || len(idle) < 2 {
		t.Fatalf("content %d %d", len(active), len(idle))
	}
	a0, a1 := padPlain(ansi.Strip(active[0]), inner), padPlain(ansi.Strip(active[1]), inner)
	i0, i1 := padPlain(ansi.Strip(idle[0]), inner), padPlain(ansi.Strip(idle[1]), inner)
	for _, line := range []string{a0, a1, i0, i1} {
		if ansi.StringWidth(line) != inner {
			t.Fatalf("line width %d want %d: %q", ansi.StringWidth(line), inner, line)
		}
	}
	badgeW := ansi.StringWidth(kindIconActive) + kindTileSafe
	if widthSuffix(a0, badgeW) != kindIconActive+" " {
		t.Fatalf("filled hexagon should sit one space from the right border:\n%q", a0)
	}
	if widthSuffix(i0, badgeW) != kindIconIdle+" " {
		t.Fatalf("idle hexagon should sit one space from the right border:\n%q", i0)
	}
	if strings.Contains(a1, kindIconActive) || strings.Contains(a1, "active") || strings.Contains(i1, kindIconIdle) || strings.Contains(i1, "active") {
		t.Fatalf("status line should not carry the badge:\n%q\n%q", a1, i1)
	}
	if strings.Contains(a0, longName) {
		t.Fatalf("long name should truncate to leave room for the badge:\n%q", a0)
	}
	if !strings.Contains(a0, "…") {
		t.Fatalf("long name should show an ellipsis:\n%q", a0)
	}
	if strings.Contains(i0, kindIconActive) || strings.Contains(i0, "active") {
		t.Fatalf("idle tile should not show the filled hexagon or active word:\n%q", i0)
	}
	wideInner := max(8, kindCardWidth(80)-2)
	wideActive := padPlain(ansi.Strip(m.kindTileContent(tiles[0], true, wideInner)[0]), wideInner)
	wideIdle := padPlain(ansi.Strip(m.kindTileContent(tiles[1], false, wideInner)[0]), wideInner)
	if !strings.Contains(wideActive, longName+" "+kindIconActive+" active") {
		t.Fatalf("wide active card should show the name and ⬢ active on the left:\n%q", wideActive)
	}
	if widthSuffix(wideActive, badgeW) != kindIconActive+" " {
		t.Fatalf("wide card should still keep the filled hexagon one space from the right border:\n%q", wideActive)
	}
	if strings.Contains(wideIdle, "active") || strings.Contains(wideIdle, kindIconActive) {
		t.Fatalf("wide idle card should only show the name on the left:\n%q", wideIdle)
	}
	if widthSuffix(wideIdle, badgeW) != kindIconIdle+" " {
		t.Fatalf("wide idle card should keep the empty hexagon one space from the right border:\n%q", wideIdle)
	}
	view := ansi.Strip(m.kindBody(80))
	if !strings.Contains(view, longName+" "+kindIconActive+" active") {
		t.Fatalf("body should show ⬢ active next to the active name:\n%s", view)
	}
	if strings.Contains(view, "ACTIVE") {
		t.Fatalf("ACTIVE label still present:\n%s", view)
	}
	for i, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > 80 {
			t.Fatalf("body line %d is %d wide:\n%s", i, got, line)
		}
	}
}

func TestKindPodsCmdAndCleanup(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"k2", "dev"} {
		path := kube.KindKubeconfig(name, home)
		if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m.kubeconfig, m.context = kube.KindKubeconfig("k2", home), kube.KindContext("k2")
	m = open(t, m, "kind")
	if m.kindPods["k2"].Label() != "14" || m.kindPods["dev"].Label() != "12/14" {
		t.Fatalf("pods %+v", m.kindPods)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "pods           14") || !strings.Contains(view, "pods           12/14") {
		t.Fatalf("tile pods:\n%s", view)
	}
	var saw bool
	for _, c := range f.Calls() {
		if c.Name == "kubectl" && strings.Contains(strings.Join(c.Args, " "), "get pods") && strings.Contains(strings.Join(c.Args, " "), "--no-headers") {
			saw = true
			line := strings.Join(c.Args, " ")
			if !strings.Contains(line, "--kubeconfig") || !strings.Contains(line, "--context") {
				t.Fatalf("pod count args %q", line)
			}
		}
	}
	if !saw {
		t.Fatal("expected a background pod-count fetch")
	}
	m.vs.kindNodes["k2"] = kube.KindNodes{Planes: 1, Running: 0, Total: 1, Version: "v1.33.1"}
	m.applyKindPods(kindPodsMsg{pods: map[string]kube.KindPods{"dev": {Running: 12, Total: 14, OK: true}}})
	if _, ok := m.kindPods["k2"]; ok {
		t.Fatalf("stopped cluster still has pods %+v", m.kindPods)
	}
	if m.kindPods["dev"].Label() != "12/14" {
		t.Fatalf("dev pods %+v", m.kindPods["dev"])
	}
}

func TestKindFormFooterAndChartNames(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	view := ansi.Strip(m.View())
	assertKindFormFooter(t, view)
	if !strings.Contains(view, "kube-prometheus-stack") {
		t.Fatalf("chart name truncated:\n%s", view)
	}
	m = pressSettle(t, m, "esc")
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := kube.KindKubeconfig("k2", home)
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = path, kube.KindContext("k2")
	m = open(t, m, "kind")
	m = pressSettle(t, m, "e")
	view = ansi.Strip(m.View())
	assertKindFormFooter(t, view)
	if !strings.Contains(view, "kube-prometheus-stack") {
		t.Fatalf("charts form truncated:\n%s", view)
	}
}

func assertKindFormFooter(t *testing.T, view string) {
	t.Helper()
	if strings.Contains(view, "canc\nel") || strings.Contains(view, "esc canc\n") {
		t.Fatalf("footer split mid-word:\n%s", view)
	}
	if !strings.Contains(view, "cancel") {
		t.Fatalf("footer missing cancel:\n%s", view)
	}
}

func TestKindCardsStackFullWidth(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev", "lab"})
	m := appModel(t, f)
	m.vs.kindNames = []string{"k2", "dev", "lab"}
	nodes := kube.KindNodes{Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 2h"}
	m.vs.kindNodes = map[string]kube.KindNodes{"k2": nodes, "dev": nodes, "lab": nodes}
	m.stage = "kind"
	view := ansi.Strip(m.kindBody(80))
	k2At, devAt, labAt := strings.Index(view, "k2"), strings.Index(view, "dev"), strings.Index(view, "lab")
	if k2At < 0 || devAt <= k2At || labAt <= devAt {
		t.Fatalf("cards should stack k2, then dev, then lab:\n%s", view)
	}
	for i, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, kindIconIdle) && !strings.Contains(line, kindIconActive) {
			continue
		}
		if strings.Contains(line, "k2") && strings.Contains(line, "dev") {
			t.Fatalf("line %d still places two clusters side by side: %q", i, line)
		}
	}
	wantW := kindCardWidth(80)
	var top int
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "╭") {
			top++
			if got := ansi.StringWidth(line); got != wantW {
				t.Fatalf("card top width %d want %d: %q", got, wantW, line)
			}
		}
	}
	if top != 3 {
		t.Fatalf("expected 3 stacked cards, got %d:\n%s", top, view)
	}
}

func TestKindCardsSplitAndNarrow(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	m := appModel(t, f)
	m.vs.kindNames = []string{"k2"}
	m.vs.kindNodes = map[string]kube.KindNodes{"k2": {Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 2h"}}
	m.kindUsage = map[string]kube.KindUsage{"k2": {CPU: 4.2, MemUsed: 1503238554, MemPct: 40, OK: true}}
	m.kindCatalog = map[string][]string{"k2": {"metallb"}}
	m.stage = "kind"
	t.Helper()
	wideInner := max(8, kindCardWidth(80)-2)
	if wideInner < kindSplitMin {
		t.Fatalf("wide inner %d", wideInner)
	}
	wide := m.kindTileContent(m.kindTiles()[0], true, wideInner)
	var split bool
	for _, line := range wide {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "k8s") && strings.Contains(plain, "CPU") {
			split = true
			break
		}
	}
	if !split {
		t.Fatalf("wide card should put k8s and CPU on one line:\n%q", wide)
	}
	narrow := m.kindTileContent(m.kindTiles()[0], true, 40)
	for _, line := range narrow {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "k8s") && strings.Contains(plain, "CPU") {
			t.Fatalf("narrow card should stack details, got %q", plain)
		}
	}
}

func TestKindCardsScrollKeepsSelectionVisible(t *testing.T) {
	withKindCLIs(t)
	names := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	f := kindFake(t, names)
	m := appModel(t, f)
	m.vs.kindNames = names
	nodes := kube.KindNodes{Planes: 1, Running: 1, Total: 1, Version: "v1.33.1", Uptime: "up 1h"}
	m.vs.kindNodes = map[string]kube.KindNodes{}
	for _, n := range names {
		m.vs.kindNodes[n] = nodes
	}
	m.stage = "kind"
	m.boxH = 12
	m.vs.kindSel = 0
	m.ensureKindVisible()
	first := ansi.Strip(m.kindBody(80))
	if !strings.Contains(first, "alpha") {
		t.Fatalf("first card should be visible:\n%s", first)
	}
	m.selectKind("echo")
	later := ansi.Strip(m.kindBody(80))
	if !strings.Contains(later, "echo") {
		t.Fatalf("selected card should be visible:\n%s", later)
	}
	if strings.Contains(later, "alpha") {
		t.Fatalf("scrolled view should have moved past the first card:\n%s", later)
	}
}

func TestKindSpaceUsesCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	labKC := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(labKC, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, " ")
	if m.kubeconfig != labKC || m.context != "kind-lab" {
		t.Fatalf("space switch %s %s status %q", m.kubeconfig, m.context, m.status)
	}
	if m.stage != "kind" {
		t.Fatalf("space should stay on kind, stage %s", m.stage)
	}
}

func TestKindDetailAlignsConfirmRows(t *testing.T) {
	if got, want := kindDetail("install", "kafbat"), "install      kafbat"; got != want {
		t.Fatalf("install %q want %q", got, want)
	}
	if got, want := kindDetail("cluster", "kindest"), "cluster      kindest"; got != want {
		t.Fatalf("cluster %q want %q", got, want)
	}
	if got, want := kindDetail("kubeconfig", "~/.kube/config"), "kubeconfig   ~/.kube/config"; got != want {
		t.Fatalf("kubeconfig %q want %q", got, want)
	}
	if kindDetail("will remove", "~/.kube/k2-lab") != "will remove  ~/.kube/k2-lab" {
		t.Fatalf("will remove %q", kindDetail("will remove", "~/.kube/k2-lab"))
	}
}

func TestKindDetailWrapsConfirmValue(t *testing.T) {
	charts := "sealed-secrets, metallb, cert-manager, metrics-server, kafbat"
	line := kindDetail("charts", charts)
	const width = 52
	got := wrapKindDetail(line, width)
	if len(got) < 2 {
		t.Fatalf("expected wrap, got %q", got)
	}
	prefix, first, ok := kindDetailParts(got[0])
	if !ok {
		t.Fatalf("first line lost the label column: %q", got[0])
	}
	if !strings.HasPrefix(strings.TrimSpace(prefix), "charts") || !strings.Contains(first, "sealed-secrets") {
		t.Fatalf("first line %q", got[0])
	}
	pad := strings.Repeat(" ", len(prefix))
	for i, row := range got {
		if lipgloss.Width(row) > width {
			t.Fatalf("line %d wider than %d: %q", i, width, row)
		}
		if i == 0 {
			continue
		}
		if !strings.HasPrefix(row, pad) {
			t.Fatalf("continuation should align under the value column:\n%s", strings.Join(got, "\n"))
		}
		rest := strings.TrimSpace(row)
		if rest == "" || strings.HasPrefix(rest, "metrics-server") && strings.HasPrefix(row, "metrics-server") {
			t.Fatalf("continuation started at column 0: %q", row)
		}
		if strings.HasPrefix(row, "metrics-server") || strings.HasPrefix(row, "kafbat") {
			t.Fatalf("wrapped charts look like a label: %q", row)
		}
	}
	joined := strings.Join(got, " ")
	for _, name := range []string{"sealed-secrets", "metallb", "cert-manager", "metrics-server", "kafbat"} {
		if !strings.Contains(joined, name) {
			t.Fatalf("missing %q in %q", name, got)
		}
	}
}

func TestKindCreateConfirmWrapsCharts(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = open(t, m, "kind")
	m = pressSettle(t, m, "A")
	setHelmField(m.dlg, "name", "k2")
	want := []string{"sealed-secrets", "metallb", "cert-manager", "metrics-server", "kafbat"}
	on := map[string]bool{}
	for _, n := range want {
		on["c:"+n] = true
	}
	for i := range m.dlg.hfields {
		if on[m.dlg.hfields[i].key] {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.ask == "" {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "charts") || !strings.Contains(view, "sealed-secrets") {
		t.Fatalf("confirm lacks charts:\n%s", view)
	}
	var hit string
	for _, row := range strings.Split(view, "\n") {
		trim := strings.TrimSpace(row)
		if strings.Contains(trim, "metrics-server") || (strings.Contains(trim, "kafbat") && !strings.Contains(trim, "charts")) {
			hit = row
			idx := kindDialogValueCol(row, "metrics-server")
			if idx < 0 {
				idx = kindDialogValueCol(row, "kafbat")
			}
			chartsIdx := -1
			for _, r := range strings.Split(view, "\n") {
				if strings.Contains(r, "sealed-secrets") && strings.Contains(r, "charts") {
					chartsIdx = kindDialogValueCol(r, "sealed-secrets")
					break
				}
			}
			if idx == 0 || strings.HasPrefix(trim, "metrics-server") || strings.HasPrefix(trim, "kafbat") {
				t.Fatalf("wrapped charts look like a label:\n%s", view)
			}
			if chartsIdx > 0 && idx > 0 && idx != chartsIdx {
				t.Fatalf("continuation at col %d, want %d under sealed-secrets:\n%s", idx, chartsIdx, view)
			}
			break
		}
	}
	if hit == "" {
		t.Fatalf("expected charts to wrap in the popup:\n%s", view)
	}
}

func TestKindStatusIconUsesMiniDot(t *testing.T) {
	m := model{cfg: testEnv(t)}
	m.cfg.UI.Animations = true
	m.beat = 0
	if got, want := m.kindStatusIcon("deploying"), kindSpinGlyph(0); got != want {
		t.Fatalf("deploying %q want %q", got, want)
	}
	m.beat = 3
	if got, want := m.kindStatusIcon("deploying"), kindSpinGlyph(3); got != want {
		t.Fatalf("deploying beat 3 %q want %q", got, want)
	}
	if got, want := m.kindStatusIcon("deleting"), kindSpinGlyph(5); got != want {
		t.Fatalf("deleting %q want %q", got, want)
	}
	m.cfg.UI.Animations = false
	if got := m.kindStatusIcon("deploying"); got != kindSpinGlyph(0) {
		t.Fatalf("static deploying %q", got)
	}
	for _, ch := range []string{"◧", "◨", "◩", "◪"} {
		if m.kindStatusIcon("deploying") == ch || m.kindStatusIcon("deleting") == ch {
			t.Fatalf("still using square spinner %s", ch)
		}
	}
	if m.kindStatusIcon("running") != kindIconRunning || m.kindStatusIcon("stopped") != kindIconStopped || m.kindStatusIcon("failed") != kindIconFailed {
		t.Fatal("steady status icons changed")
	}
}

func TestKindFormTypesJAndK(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "kind"), "A")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v", m.dlg)
	}
	for i, f := range m.dlg.hfields {
		if f.key == "name" {
			m.dlg.idx = i
			break
		}
	}
	m = m.moveHelmField(0)
	setHelmField(m.dlg, "name", "")
	start := m.dlg.idx
	for _, r := range "k2-jk" {
		m = pressSettle(t, m, string(r))
		if m.dlg == nil || m.dlg.idx != start || m.dlg.hfields[m.dlg.idx].key != "name" {
			t.Fatalf("typing %q into name moved the field", string(r))
		}
	}
	if helmFieldValue(m.dlg, "name") != "k2-jk" {
		t.Fatalf("name=%q", helmFieldValue(m.dlg, "name"))
	}
}

func TestKindCreateHidesDialogOverEditor(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "kind"), "A")
	if m.dlg == nil {
		t.Fatal("no form")
	}
	if !m.dialogActive() || !strings.Contains(ansi.Strip(m.View()), "control-planes") {
		t.Fatalf("form should be visible before the editor:\n%s", ansi.Strip(m.View()))
	}
	m.ed = &editor{kind: "kindconfig"}
	if m.dialogActive() {
		t.Fatal("dialog should be suspended while vim is open")
	}
	if m.withDialog("SCREEN") != "SCREEN" {
		t.Fatal("create popup should not overlay the screen while vim is open")
	}
	m.ed = nil
	if !m.dialogActive() || !strings.Contains(ansi.Strip(m.View()), "control-planes") {
		t.Fatalf("form should return after vim:\n%s", ansi.Strip(m.View()))
	}
}

func TestKindCreateFooterFollowsFocus(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m.kindNodeImages = []string{"kindest/node:v1.34.0", "kindest/node:v1.33.1"}
	m = pressSettle(t, open(t, m, "kind"), "A")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "cycle images") || strings.Contains(view, "space toggle") || strings.Contains(view, "ctrl+e") {
		t.Fatalf("name field footer should be generic:\n%s", view)
	}
	m = focusKindField(t, m, "config")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "ctrl+e") || !strings.Contains(view, "edit") {
		t.Fatalf("config footer:\n%s", view)
	}
	if strings.Contains(view, "space toggle") || strings.Contains(view, "cycle images") {
		t.Fatalf("config footer leaked other hints:\n%s", view)
	}
	m = focusKindField(t, m, "c:istio")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "space") || !strings.Contains(view, "toggle") || !strings.Contains(view, "values") {
		t.Fatalf("chart footer:\n%s", view)
	}
	if strings.Contains(view, "cycle images") || strings.Contains(view, "ctrl+e") {
		t.Fatalf("chart footer leaked other hints:\n%s", view)
	}
}

func dockerCalls(f *shell.Fake, verb string) []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Name != "docker" {
			continue
		}
		line := strings.Join(c.Args, " ")
		if verb == "" || strings.HasPrefix(line, verb) {
			out = append(out, line)
		}
	}
	return out
}

func TestKindEnterOpensControlPlaneLogs(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	labKC := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(labKC, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "enter")
	if m.stage != "kindlog" {
		t.Fatalf("enter stage %s status %q", m.stage, m.status)
	}
	if m.kubeconfig != "/kc" {
		t.Fatalf("enter should not switch the cluster, kubeconfig %s", m.kubeconfig)
	}
	if !strings.Contains(m.logText, "apiserver is ready") {
		t.Fatalf("control-plane log %q", m.logText)
	}
	got := dockerCalls(f, "logs")
	if len(got) == 0 || !strings.Contains(got[0], "logs --tail 200 -f lab-control-plane") {
		t.Fatalf("docker logs calls %v", got)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "kind" {
		t.Fatalf("esc stage %s", m.stage)
	}
	if name, ok := m.selectedKind(); !ok || name != "lab" {
		t.Fatalf("selection after logs %q %v", name, ok)
	}
	if m.kindLogCancel != nil {
		t.Fatal("live log should be cancelled on esc")
	}
}

func TestKindCacheRendersBeforeRefresh(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab", "dev"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lab", "dev"} {
		path := kube.KindKubeconfig(name, home)
		if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m = open(t, m, "kind")
	if !strings.Contains(ansi.Strip(m.View()), "lab") {
		t.Fatalf("first open should settle with tiles:\n%s", ansi.Strip(m.View()))
	}
	m.stage = "nav"
	m.vs.kindNames = nil
	m.vs.kindNodes = nil
	before := len(f.Calls())
	next, cmd := m.openKind()
	m = next.(model)
	if m.stage != "kind" {
		t.Fatalf("stage %s", m.stage)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "lab") || !strings.Contains(view, "dev") {
		t.Fatalf("cached tiles missing before refresh:\n%s", view)
	}
	if len(f.Calls()) != before {
		t.Fatalf("openKind ran docker/kubectl on the event loop: +%d calls", len(f.Calls())-before)
	}
	if cmd == nil {
		t.Fatal("openKind should still refresh in the background")
	}
}

func TestKindExtrasThrottleAndForceRefresh(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	m = open(t, m, "kind")
	if m.kindExtrasAt.IsZero() {
		t.Fatal("first open should fetch extras")
	}
	m.kindUsageBusy, m.kindCatalogBusy, m.kindPodsBusy = false, false, false
	t0 := m.kindExtrasAt
	oldNow := now
	t.Cleanup(func() { now = oldNow })
	now = func() time.Time { return t0.Add(5 * time.Second) }
	if m.kindExtrasCmds() != nil {
		t.Fatal("extras should stay quiet inside the throttle window")
	}
	m.stage = "nav"
	now = func() time.Time { return t0.Add(2 * kindExtrasEvery) }
	if m.kindVisible() {
		t.Fatal("nav should hide kind")
	}
	if m.kindExtrasCmds() != nil {
		t.Fatal("extras should not run while kind is hidden")
	}
	m.stage = "kind"
	now = func() time.Time { return t0.Add(5 * time.Second) }
	m.kindExtrasForce = true
	if m.kindExtrasCmds() == nil {
		t.Fatal("force refresh should start extras at once")
	}
	m.kindUsageBusy, m.kindCatalogBusy, m.kindPodsBusy = false, false, false
	m.kindExtrasAt = t0
	now = func() time.Time { return t0.Add(kindExtrasEvery) }
	if m.kindExtrasCmds() == nil {
		t.Fatal("extras should run again after the throttle")
	}
	m.kindUsageBusy, m.kindCatalogBusy, m.kindPodsBusy = false, false, false
	m = pressSettle(t, m, "r")
	if m.kindExtrasAt.Equal(t0) {
		t.Fatal("r should force extras")
	}
}

func TestKindCreateGeneratedConfigSyncsWithWizard(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "kind"), "A")
	path := helmFieldValue(m.dlg, "config")
	if path != kindConfigPath("k2") {
		t.Fatalf("config path %q", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := kube.ParseKindConfig(string(raw))
	if err != nil || info.Planes != 1 || info.Workers != 0 {
		t.Fatalf("initial %+v %v\n%s", info, err, raw)
	}
	setHelmField(m.dlg, "planes", "2")
	setHelmField(m.dlg, "workers", "3")
	setHelmField(m.dlg, "image", "kindest/node:v1.34.0")
	m.refreshKindGeneratedConfig()
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err = kube.ParseKindConfig(string(raw))
	if err != nil || info.Planes != 2 || info.Workers != 3 || info.Image != "kindest/node:v1.34.0" {
		t.Fatalf("updated %+v %v\n%s", info, err, raw)
	}
	setHelmField(m.dlg, "name", "lab")
	m.refreshKindGeneratedConfig()
	next := helmFieldValue(m.dlg, "config")
	if next != kindConfigPath("lab") {
		t.Fatalf("renamed path %q", next)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("old generated file still present: %v", err)
	}
	raw, err = os.ReadFile(next)
	if err != nil {
		t.Fatal(err)
	}
	info, err = kube.ParseKindConfig(string(raw))
	if err != nil || info.Planes != 2 || info.Workers != 3 || info.Image != "kindest/node:v1.34.0" {
		t.Fatalf("renamed %+v %v\n%s", info, err, raw)
	}
}

func TestKindCreateConfigEditReflectsInWizard(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m.kindNodeImages = []string{"kindest/node:v1.37.0", "kindest/node:v1.36.0", "kindest/node:v1.35.0"}
	m = pressSettle(t, open(t, m, "kind"), "A")
	setHelmField(m.dlg, "workers", "2")
	setHelmField(m.dlg, "image", "kindest/node:v1.37.0")
	m.refreshKindGeneratedConfig()
	path := helmFieldValue(m.dlg, "config")
	text, err := kube.KindConfigYAML(1, 0, "kindest/node:v1.35.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(kindConfigDoneMsg{})
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("form %+v", m.dlg)
	}
	if helmFieldValue(m.dlg, "image") != "kindest/node:v1.35.0" {
		t.Fatalf("image %q", helmFieldValue(m.dlg, "image"))
	}
	if helmFieldValue(m.dlg, "workers") != "0" {
		t.Fatalf("workers %q", helmFieldValue(m.dlg, "workers"))
	}
	if helmFieldValue(m.dlg, "planes") != "1" {
		t.Fatalf("planes %q", helmFieldValue(m.dlg, "planes"))
	}
	setHelmField(m.dlg, "workers", "1")
	m.refreshKindGeneratedConfig()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := kube.ParseKindConfig(string(raw))
	if err != nil || info.Planes != 1 || info.Workers != 1 || info.Image != "kindest/node:v1.35.0" {
		t.Fatalf("resync %+v %v\n%s", info, err, raw)
	}
}

func TestKindCreateConfirmBackAndCancel(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "kind"), "A")
	setHelmField(m.dlg, "name", "lab")
	setHelmField(m.dlg, "planes", "2")
	setHelmField(m.dlg, "workers", "3")
	setHelmField(m.dlg, "image", "kindest/node:v1.35.0")
	m.refreshKindGeneratedConfig()
	for i := range m.dlg.hfields {
		if m.dlg.hfields[i].key == "c:metallb" {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.ask == "" || !strings.Contains(m.dlg.ask, "create kind cluster lab?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "esc/q back") || !strings.Contains(view, "default no") {
		t.Fatalf("footer:\n%s", view)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "kindcreate" || m.dlg.ask != "" {
		t.Fatalf("esc should return to the wizard: %+v", m.dlg)
	}
	if helmFieldValue(m.dlg, "name") != "lab" || helmFieldValue(m.dlg, "planes") != "2" || helmFieldValue(m.dlg, "workers") != "3" || helmFieldValue(m.dlg, "image") != "kindest/node:v1.35.0" {
		t.Fatalf("fields name=%s planes=%s workers=%s image=%s", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "planes"), helmFieldValue(m.dlg, "workers"), helmFieldValue(m.dlg, "image"))
	}
	on := false
	for _, f := range m.dlg.hfields {
		if f.key == "c:metallb" {
			on = f.on
		}
	}
	if !on {
		t.Fatal("metallb should stay selected")
	}
	next, cmd = m.submitKindForm()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "q")
	if m.dlg == nil || m.dlg.act != "kindcreate" || helmFieldValue(m.dlg, "name") != "lab" {
		t.Fatalf("q should return to the wizard: %+v", m.dlg)
	}
	next, cmd = m.submitKindForm()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "n")
	if m.dlg != nil {
		t.Fatalf("n should cancel: %+v", m.dlg)
	}
	if m.status != "cancelled" {
		t.Fatalf("status %q", m.status)
	}
	if len(kindCalls(f, "create cluster")) != 0 {
		t.Fatal("cancelled create ran")
	}
}

func kindDialogValueCol(row, value string) int {
	i := strings.Index(row, value)
	if i <= 0 {
		return i
	}
	left := strings.LastIndex(row[:i], "│")
	if left < 0 {
		return i
	}
	return i - left
}

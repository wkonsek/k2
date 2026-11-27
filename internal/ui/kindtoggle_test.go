package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

type stateFake struct {
	mu      sync.Mutex
	running map[string]bool
	*shell.Fake
}

func (s *stateFake) set(name string, up bool) {
	s.mu.Lock()
	s.running[name] = up
	s.mu.Unlock()
}

func kindStateFake(t *testing.T, clusters map[string]bool) *stateFake {
	t.Helper()
	var names []string
	for n := range clusters {
		names = append(names, n)
	}
	sort.Strings(names)
	parent := kindFake(t, names).Handler
	s := &stateFake{running: map[string]bool{}}
	for n, up := range clusters {
		s.running[n] = up
	}
	s.Fake = &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Name != "docker" || len(r.Args) == 0 {
			return parent(r)
		}
		line := strings.Join(r.Args, " ")
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.Args[0] == "ps" && strings.Contains(line, " -q "):
			for _, n := range names {
				if strings.HasSuffix(line, "io.x-k8s.kind.cluster="+n) {
					return shell.OK("id" + n + "aaaaaaaa\n")
				}
			}
			return shell.OK("")
		case r.Args[0] == "ps":
			var b strings.Builder
			for _, n := range names {
				if s.running[n] {
					b.WriteString(n + "\tcontrol-plane\tkindest/node:v1.33.1\trunning\tUp 1 hour\tid" + n + "aaaaaaaa\n")
				} else {
					b.WriteString(n + "\tcontrol-plane\tkindest/node:v1.33.1\texited\tExited (0) 2 minutes ago\tid" + n + "aaaaaaaa\n")
				}
			}
			return shell.OK(b.String())
		case r.Args[0] == "stop" || r.Args[0] == "start":
			for _, id := range r.Args[1:] {
				for _, n := range names {
					if id == "id"+n+"aaaaaaaa" {
						s.running[n] = r.Args[0] == "start"
					}
				}
			}
			return shell.OK(strings.Join(r.Args[1:], "\n"))
		}
		return parent(r)
	}}
	return s
}

func writeKindKC(t *testing.T, names ...string) {
	t.Helper()
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(kube.KindKubeconfig(name, home), []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKindToggleStopsRunningCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindStateFake(t, map[string]bool{"lab": true})
	writeKindKC(t, "lab")
	m := open(t, appModel(t, f.Fake), "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "s")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "stop kind cluster lab?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	if len(dockerCalls(f.Fake, "stop")) != 0 {
		t.Fatal("stopped before confirm")
	}
	m = pressSettle(t, m, "y")
	got := dockerCalls(f.Fake, "stop")
	if len(got) != 1 || got[0] != "stop idlabaaaaaaaa" {
		t.Fatalf("stop calls %v", got)
	}
	if st := m.kindTileFor("lab"); st.status != "stopped" || !st.nodes.StartedAt.IsZero() {
		t.Fatalf("after stop %+v", st)
	}
	if !strings.Contains(m.status, "s start") {
		t.Fatalf("status %q", m.status)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "stopped") || strings.Contains(view, "up 1h") {
		t.Fatalf("card:\n%s", view)
	}
}

func TestKindToggleStartsStoppedCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindStateFake(t, map[string]bool{"lab": false})
	writeKindKC(t, "lab")
	m := open(t, appModel(t, f.Fake), "kind")
	m = selectKindTile(t, m, "lab")
	if leg := ansi.Strip(m.legend()); !strings.Contains(leg, "s start") {
		t.Fatalf("legend %q", leg)
	}
	next, cmd := m.Update(key(t, "s"))
	m = next.(model)
	if m.dlg != nil {
		t.Fatalf("start should not ask: %+v", m.dlg)
	}
	if st := m.kindTileFor("lab").status; st != "starting" {
		t.Fatalf("in flight status %s", st)
	}
	if !strings.Contains(ansi.Strip(m.View()), "starting") {
		t.Fatalf("card:\n%s", ansi.Strip(m.View()))
	}
	m = settle(t, m, cmd)
	got := dockerCalls(f.Fake, "start")
	if len(got) != 1 || got[0] != "start idlabaaaaaaaa" {
		t.Fatalf("start calls %v", got)
	}
	if st := m.kindTileFor("lab").status; st != "running" {
		t.Fatalf("after start %s", st)
	}
	if len(m.kindToggle) != 0 {
		t.Fatalf("toggle state left %v", m.kindToggle)
	}
}

func TestKindToggleReadOnly(t *testing.T) {
	withKindCLIs(t)
	f := kindStateFake(t, map[string]bool{"lab": true, "dev": false})
	writeKindKC(t, "lab", "dev")
	m := open(t, appModel(t, f.Fake), "kind")
	m.readOnly = true
	for _, name := range []string{"lab", "dev"} {
		m = selectKindTile(t, m, name)
		next := pressSettle(t, m, "s")
		if next.dlg != nil || !strings.Contains(next.status, "read-only") {
			t.Fatalf("%s: dlg %+v status %q", name, next.dlg, next.status)
		}
	}
	if len(dockerCalls(f.Fake, "stop")) != 0 || len(dockerCalls(f.Fake, "start")) != 0 {
		t.Fatal("read-only must not touch containers")
	}
}

func TestKindToggleProxy(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "running")
	m := open(t, proxyModel(t, f), "kind")
	m.selectKindProxy()
	if !m.kindProxySelected() {
		t.Fatal("proxy not selected")
	}
	m = pressSettle(t, m, "s")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "stop registry proxy k2-registry-proxy?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	m = pressSettle(t, m, "y")
	if got := proxyCalls(f.Fake, "stop k2-registry-proxy"); len(got) != 1 {
		t.Fatalf("stop calls %v", got)
	}
	if m.kindProxy.Running() || m.kindProxyOp != "" || !m.kindProxySelected() {
		t.Fatalf("after stop %+v op %q", m.kindProxy, m.kindProxyOp)
	}
	if !strings.Contains(m.kindHint(), "s start") || !strings.Contains(m.kindHint(), "enter logs") {
		t.Fatalf("hint %q", m.kindHint())
	}
	m = pressSettle(t, m, "s")
	if m.dlg != nil {
		t.Fatalf("start should not ask: %+v", m.dlg)
	}
	if got := proxyCalls(f.Fake, "start k2-registry-proxy"); len(got) != 1 {
		t.Fatalf("start calls %v", got)
	}
	if !m.kindProxy.Running() {
		t.Fatalf("after start %+v", m.kindProxy)
	}
	m.readOnly = true
	m = pressSettle(t, m, "s")
	if m.dlg != nil || !strings.Contains(m.status, "read-only") {
		t.Fatalf("read-only proxy: %+v %q", m.dlg, m.status)
	}
}

func TestKindRefreshSeesExternallyStoppedCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindStateFake(t, map[string]bool{"lab": true})
	writeKindKC(t, "lab")
	m := open(t, appModel(t, f.Fake), "kind")
	if st := m.kindTileFor("lab"); st.status != "running" || st.nodes.StartedAt.IsZero() {
		t.Fatalf("start %+v", st)
	}
	f.set("lab", false)
	m.kindExtrasForce = true
	m = settle(t, m, m.kindExtrasCmds())
	st := m.kindTileFor("lab")
	if st.status != "stopped" || !st.nodes.StartedAt.IsZero() || st.nodes.Running != 0 {
		t.Fatalf("tick did not see the stop: %+v", st)
	}
	if _, ok := m.kindUsage["lab"]; ok {
		t.Fatal("usage should be dropped for a stopped cluster")
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "up 1h") || !strings.Contains(view, "stopped") {
		t.Fatalf("card:\n%s", view)
	}
	f.set("lab", true)
	m = pressSettle(t, m, "r")
	if st := m.kindTileFor("lab"); st.status != "running" || st.nodes.StartedAt.IsZero() {
		t.Fatalf("r did not see the start: %+v", st)
	}
}

func TestKindCreateRefusesDuplicateName(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	writeKindKC(t, "k2")
	m := open(t, appModel(t, f), "kind")
	m = pressSettle(t, m, "A")
	if m.vs.kop == nil || m.vs.kop.name != "k2-2" {
		t.Fatalf("default name should skip the existing k2: %+v", m.vs.kop)
	}
	m.vs.kop.name = "k2"
	m.fillKindForm(m.dlg, m.vs.kop)
	next, _ := m.submitKindForm()
	m = next.(model)
	if m.dlg == nil || m.dlg.msg != `cluster "k2" already exists` {
		t.Fatalf("dlg %+v", m.dlg)
	}
	if m.dlg.ask != "" || len(kindCalls(f, "create cluster")) != 0 {
		t.Fatal("duplicate must not reach the confirm or kind create")
	}
}

func TestKindFailedCreateNeverHidesLiveCluster(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	writeKindKC(t, "k2")
	m := open(t, appModel(t, f), "kind")
	m.kindJobs = map[string]*kindJob{"k2": {name: "k2", status: "failed", create: true, log: "node(s) already exist for a cluster with the name \"k2\"\n", last: "already exist", started: now()}}
	if st := m.kindTileFor("k2").status; st != "running" {
		t.Fatalf("live card covered by failed job: %s", st)
	}
	if len(m.kindTiles()) != 1 {
		t.Fatalf("tiles %d", len(m.kindTiles()))
	}
	m.kindJobs["gone"] = &kindJob{name: "gone", status: "failed", create: true, log: "boom\n", started: now()}
	m.kindJobs["lab"] = &kindJob{name: "lab", status: "deploying", log: "Creating\n", started: now()}
	if st := m.kindTileFor("gone").status; st != "failed" {
		t.Fatalf("failed create without a cluster should still show: %s", st)
	}
	m = pressSettle(t, m, "r")
	if _, ok := m.kindJobs["k2"]; ok {
		t.Fatal("r should clear the stale failed create job")
	}
	if _, ok := m.kindJobs["gone"]; ok {
		t.Fatal("r should clear failed jobs")
	}
	if j := m.kindJobs["lab"]; j == nil || j.status != "deploying" {
		t.Fatal("r must keep in-flight jobs")
	}
}

func TestKindProxyBorderMatchesClusters(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, []string{"lab"}, "running")
	m := open(t, proxyModel(t, f), "kind")
	m = selectKindTile(t, m, "lab")
	proxy := m.renderKindProxy(false, 90)
	cluster := m.renderKindTile(m.kindTileFor("lab"), false, 90, 0, nil)
	want := sgr(th.Muted)
	if colorBefore(proxy, "╭") != want || colorBefore(cluster, "╭") != want {
		t.Fatalf("unselected borders differ: proxy %q cluster %q want %q", colorBefore(proxy, "╭"), colorBefore(cluster, "╭"), want)
	}
	if colorBefore(proxy, kindIconProxy) != sgr(th.Warn) {
		t.Fatalf("badge color %q", colorBefore(proxy, kindIconProxy))
	}
	if colorBefore(m.renderKindProxy(true, 90), "╭") != sgr(th.Accent) {
		t.Fatal("selected proxy border should be the accent")
	}
}

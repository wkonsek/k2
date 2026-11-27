package ui

import (
	"strings"
	"testing"
	"time"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

type proxyFake struct {
	state string
	*shell.Fake
}

func kindProxyFake(t *testing.T, clusters []string, state string) *proxyFake {
	t.Helper()
	kube.RegistryProxyCAWait = time.Millisecond
	base := kindFake(t, clusters)
	parent := base.Handler
	p := &proxyFake{state: state}
	p.Fake = &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if r.Name == "kind" && strings.HasPrefix(line, "get nodes --name ") {
			return shell.OK(strings.TrimPrefix(line, "get nodes --name ") + "-control-plane\n")
		}
		if r.Name != "docker" || len(r.Args) == 0 {
			return parent(r)
		}
		switch {
		case r.Args[0] == "inspect" && strings.Contains(line, "k2-registry-proxy"):
			if p.state == "" {
				return shell.Fail(1, "Error: No such object: k2-registry-proxy")
			}
			return shell.OK(`[{"State":{"Status":"` + p.state + `","StartedAt":"2026-11-27T08:00:00Z"},"Config":{"Env":["REGISTRIES=registry.k8s.io gcr.io quay.io ghcr.io"]},"NetworkSettings":{"Networks":{"kind":{}}}}]`)
		case r.Args[0] == "network" && len(r.Args) == 3:
			return shell.OK("[]")
		case r.Args[0] == "run":
			p.state = "running"
			return shell.OK("deadbeef")
		case r.Args[0] == "start":
			p.state = "running"
			return shell.OK("k2-registry-proxy")
		case r.Args[0] == "stop" && strings.Contains(line, "k2-registry-proxy"):
			p.state = "exited"
			return shell.OK("k2-registry-proxy")
		case r.Args[0] == "rm":
			p.state = ""
			return shell.OK("k2-registry-proxy")
		case r.Args[0] == "exec" && len(r.Args) > 1 && r.Args[1] == "k2-registry-proxy":
			return shell.OK("-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")
		case r.Args[0] == "logs" && strings.Contains(line, "k2-registry-proxy"):
			return shell.OK("proxy ready\nGET /v2/ cached\n")
		}
		return parent(r)
	}}
	return p
}

func proxyModel(t *testing.T, f *proxyFake) model {
	t.Helper()
	m := appModel(t, f.Fake)
	m.cfg.Kind.RegistryProxy.CacheDir = t.TempDir()
	return m
}

func proxyCalls(f *shell.Fake, sub string) []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Name == "docker" && strings.Contains(strings.Join(c.Args, " "), sub) {
			out = append(out, strings.Join(c.Args, " "))
		}
	}
	return out
}

func TestKindProxyShortcutWorksWithNoClusters(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "")
	m := open(t, proxyModel(t, f), "kind")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "no kind clusters") || !strings.Contains(view, "p for a registry proxy") {
		t.Fatalf("empty view:\n%s", view)
	}
	m = pressSettle(t, m, "p")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "set up registry proxy k2-registry-proxy?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{"rpardini/docker-registry-proxy:0.8.0", "registry.k8s.io", "ghcr.io"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "y")
	run := proxyCalls(f.Fake, "run -d --name k2-registry-proxy")
	if len(run) != 1 || !strings.Contains(run[0], "--network kind") || !strings.Contains(run[0], "REGISTRIES=registry.k8s.io gcr.io quay.io ghcr.io") {
		t.Fatalf("run %v", run)
	}
	if !m.kindProxy.Running() || !m.kindProxySelected() || m.kindProxyOp != "" {
		t.Fatalf("proxy %+v sel %d op %q", m.kindProxy, m.vs.kindSel, m.kindProxyOp)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{"registry proxy", "k2-registry-proxy", "running", "not a cluster", "docker.io"} {
		if !strings.Contains(view, want) {
			t.Fatalf("card lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "p")
	if m.dlg != nil || !strings.Contains(m.status, "already running") {
		t.Fatalf("second p: dlg %+v status %q", m.dlg, m.status)
	}
}

func TestKindProxyStartsStoppedContainer(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, []string{"lab"}, "exited")
	m := open(t, proxyModel(t, f), "kind")
	if !strings.Contains(ansi.Strip(m.View()), "exited") {
		t.Fatalf("stopped card missing:\n%s", ansi.Strip(m.View()))
	}
	m = pressSettle(t, m, "p")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "start registry proxy") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	m = pressSettle(t, m, "y")
	if len(proxyCalls(f.Fake, "start k2-registry-proxy")) != 1 || len(proxyCalls(f.Fake, "run -d")) != 0 {
		t.Fatalf("calls %v", f.Calls())
	}
}

func TestKindProxyReadOnlyBlocks(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "running")
	m := open(t, proxyModel(t, f), "kind")
	m.readOnly = true
	m = pressSettle(t, m, "p")
	if m.dlg != nil || !strings.Contains(m.status, "read-only") {
		t.Fatalf("p in read-only: dlg %+v status %q", m.dlg, m.status)
	}
	m.selectKindProxy()
	m = pressSettle(t, m, "ctrl+d")
	if m.dlg != nil || len(proxyCalls(f.Fake, "rm -f")) != 0 {
		t.Fatalf("ctrl+d in read-only: dlg %+v", m.dlg)
	}
	m.readOnly = false
	m.kindProxy = kube.ProxyStatus{}
	m = pressSettle(t, m, "p")
	if m.dlg == nil {
		t.Fatal("p should ask once read-only is off")
	}
}

func TestKindProxyCardSelectionLogsAndBack(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, []string{"lab"}, "running")
	m := open(t, proxyModel(t, f), "kind")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "lab") || !strings.Contains(view, "registry proxy") || !strings.Contains(view, "up ") {
		t.Fatalf("view:\n%s", view)
	}
	if m.kindCardCount() != 2 || m.kindProxySelected() {
		t.Fatalf("count %d sel %d", m.kindCardCount(), m.vs.kindSel)
	}
	m = pressSettle(t, m, "j")
	if !m.kindProxySelected() {
		t.Fatalf("j should reach the proxy card, sel %d", m.vs.kindSel)
	}
	if _, ok := m.selectedKind(); ok {
		t.Fatal("proxy card is not a cluster")
	}
	m = pressSettle(t, m, "j")
	if !m.kindProxySelected() {
		t.Fatal("proxy is the last card")
	}
	m = pressSettle(t, m, " ")
	if !strings.Contains(m.status, "not a cluster") || m.context != "kind-dev" {
		t.Fatalf("space on proxy: status %q ctx %s", m.status, m.context)
	}
	m = pressSettle(t, m, "e")
	if m.dlg != nil || !strings.Contains(m.status, "not a cluster") {
		t.Fatalf("e on proxy: dlg %+v", m.dlg)
	}
	m = pressSettle(t, m, "enter")
	if m.stage != "kindlog" || m.vs.kindLog != kindProxyLog || m.subject != "registry proxy" {
		t.Fatalf("logs stage %s log %q subject %q", m.stage, m.vs.kindLog, m.subject)
	}
	if got := proxyCalls(f.Fake, "logs --tail 200 -f k2-registry-proxy"); len(got) != 1 {
		t.Fatalf("logs %v", got)
	}
	if !strings.Contains(m.logText, "proxy ready") {
		t.Fatalf("log text %q", m.logText)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "kind" || !m.kindProxySelected() {
		t.Fatalf("back: stage %s sel %d", m.stage, m.vs.kindSel)
	}
}

func TestKindProxyRemoveKeepsCacheAndClusterDeleteKeepsProxy(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, []string{"lab"}, "running")
	m := open(t, proxyModel(t, f), "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "ctrl+d")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "delete kind cluster lab?") {
		t.Fatalf("cluster delete %+v", m.dlg)
	}
	m = pressSettle(t, m, "y")
	if len(proxyCalls(f.Fake, "rm -f")) != 0 || !m.kindProxy.Running() {
		t.Fatal("deleting a cluster must not touch the proxy")
	}
	m.selectKindProxy()
	m = pressSettle(t, m, "ctrl+d")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "remove registry proxy k2-registry-proxy?") {
		t.Fatalf("proxy delete %+v", m.dlg)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(strings.Join(m.dlg.details, " "), "stay on disk") || !strings.Contains(view, "stay on") {
		t.Fatalf("confirm should say the cache stays:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	if len(proxyCalls(f.Fake, "rm -f k2-registry-proxy")) != 1 {
		t.Fatalf("rm %v", f.Calls())
	}
	if m.kindProxyShown() || !strings.Contains(m.status, "cache kept") {
		t.Fatalf("after remove: %+v status %q", m.kindProxy, m.status)
	}
}

func TestKindCreateCacheToggleDefaults(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "")
	m := open(t, proxyModel(t, f), "kind")
	m = pressSettle(t, m, "A")
	if m.dlg == nil || helmFieldOn(m.dlg, "cache") {
		t.Fatalf("cache should default to no without a running proxy: %+v", m.dlg)
	}
	if !strings.Contains(ansi.Strip(m.View()), "registry cache") {
		t.Fatalf("form lacks registry cache:\n%s", ansi.Strip(m.View()))
	}
	m.dlg = nil
	f.state = "running"
	m = pressSettle(t, m, "r")
	m = pressSettle(t, m, "A")
	if m.dlg == nil || !helmFieldOn(m.dlg, "cache") {
		t.Fatalf("cache should default to yes with a running proxy")
	}
	m = focusKindField(t, m, "cache")
	m = pressSettle(t, m, " ")
	if helmFieldOn(m.dlg, "cache") {
		t.Fatal("space should toggle cache off")
	}
	if !strings.Contains(ansi.Strip(m.View()), "space toggle") {
		t.Fatalf("footer:\n%s", ansi.Strip(m.View()))
	}
}

func TestKindCreateWithCacheStartsProxyAndWiresNodes(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "")
	m := open(t, proxyModel(t, f), "kind")
	m = pressSettle(t, m, "A")
	m = focusKindField(t, m, "cache")
	m = pressSettle(t, m, " ")
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, kindDetail("registry cache", "yes (k2-registry-proxy) · starts first")) {
		t.Fatalf("confirm lacks registry cache:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	if len(kindCalls(f.Fake, "create cluster")) != 1 {
		t.Fatalf("create %v", kindCalls(f.Fake, "create cluster"))
	}
	if len(proxyCalls(f.Fake, "run -d --name k2-registry-proxy")) != 1 {
		t.Fatalf("proxy not started: %v", f.Calls())
	}
	ca := proxyCalls(f.Fake, "exec -i k2-control-plane sh -c cat > /usr/local/share/ca-certificates/docker_registry_proxy.crt")
	drop := proxyCalls(f.Fake, "exec -i k2-control-plane sh -c mkdir -p /etc/systemd/system/containerd.service.d")
	if len(ca) != 1 || len(drop) != 1 {
		t.Fatalf("node wiring ca=%v drop=%v", ca, drop)
	}
	var stdin string
	for _, c := range f.Calls() {
		if c.Name == "docker" && strings.Contains(strings.Join(c.Args, " "), "http-proxy.conf") {
			stdin = c.Stdin
		}
	}
	if !strings.Contains(stdin, "HTTP_PROXY=http://k2-registry-proxy:3128/") || !strings.Contains(stdin, "172.18.0.0/16") || !strings.Contains(stdin, "k2-control-plane") {
		t.Fatalf("drop-in %q", stdin)
	}
	if j := m.kindJobs["k2"]; j != nil && j.status != "running" {
		t.Fatalf("job %+v", j)
	}
	if !strings.Contains(kindJobLog(m, "k2"), "registry cache: k2-registry-proxy") {
		t.Fatalf("log %q", kindJobLog(m, "k2"))
	}
}

func TestKindCreateWithoutCacheSkipsProxy(t *testing.T) {
	withKindCLIs(t)
	f := kindProxyFake(t, nil, "")
	m := open(t, proxyModel(t, f), "kind")
	m = pressSettle(t, m, "A")
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if strings.Contains(ansi.Strip(m.View()), "registry cache") {
		t.Fatalf("confirm should not mention the cache:\n%s", ansi.Strip(m.View()))
	}
	m = pressSettle(t, m, "y")
	if len(proxyCalls(f.Fake, "run -d")) != 0 || len(proxyCalls(f.Fake, "http-proxy.conf")) != 0 {
		t.Fatalf("calls %v", f.Calls())
	}
}

func TestKindProxyCardRendersDistinctFromClusters(t *testing.T) {
	m := newTestModel(t, testEnv(t), &shell.Fake{})
	m.kindProxy = kube.ProxyStatus{State: "running", StartedAt: now().Add(-3 * time.Hour), Registries: []string{"quay.io"}, CacheOK: true, CacheBytes: 3 << 20}
	out := ansi.Strip(m.renderKindProxy(true, 90))
	for _, want := range []string{"▸  k2-registry-proxy", "⇄ registry proxy", "up 3h", "3.0", "docker.io, quay.io", "http://k2-registry-proxy:3128/", "╭", "╯"} {
		if !strings.Contains(out, want) {
			t.Fatalf("card lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "control-plane  ") || strings.Contains(out, "⬡") {
		t.Fatalf("proxy card looks like a cluster:\n%s", out)
	}
	if strings.ContainsAny(out, "╔╗╚╝═║") {
		t.Fatalf("proxy card should use the single-line card border:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "…") {
			t.Fatalf("truncated line %q", line)
		}
	}
	m.kindProxy = kube.ProxyStatus{State: "exited"}
	if out := ansi.Strip(m.renderKindProxy(false, 90)); !strings.Contains(out, "□  exited") || strings.Contains(out, "▸") {
		t.Fatalf("stopped card:\n%s", out)
	}
}

func helmFieldOn(d *dialog, key string) bool {
	for _, f := range d.hfields {
		if f.key == key {
			return f.on
		}
	}
	return false
}

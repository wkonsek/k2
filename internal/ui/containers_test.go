package ui

import (
	"strings"
	"testing"

	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const containerPodJSON = `{"metadata":{"name":"web-1","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
"spec":{"nodeName":"n",
 "initContainers":[
  {"name":"migrate","image":"reg.example/shop:2.3.0"},
  {"name":"shipper","image":"fluent/fluent-bit:3.1.9","restartPolicy":"Always","ports":[{"name":"metrics","containerPort":2020}],"livenessProbe":{"tcpSocket":{"port":2020}}}],
 "containers":[
  {"name":"app","image":"reg.example/shop:2.3.0","ports":[{"name":"http","containerPort":8080},{"containerPort":9090}],
   "readinessProbe":{"tcpSocket":{"port":8080}},"resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"500m","memory":"256Mi"}}},
  {"name":"side","image":"envoyproxy/envoy:v1.31.2","ports":[{"name":"admin","containerPort":9901}],"resources":{"limits":{"cpu":"200m","memory":"128Mi"}}}]},
"status":{"phase":"Running",
 "initContainerStatuses":[
  {"name":"migrate","ready":true,"restartCount":0,"state":{"terminated":{"reason":"Completed","startedAt":"2026-10-22T10:00:10Z","finishedAt":"2026-10-22T10:00:13Z"}}},
  {"name":"shipper","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-10-22T10:00:14Z"}}}],
 "containerStatuses":[
  {"name":"app","ready":false,"restartCount":5,"state":{"waiting":{"reason":"CrashLoopBackOff"}}},
  {"name":"side","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-10-22T10:00:20Z"}}}]}}`

func containerCluster() *shell.Fake {
	inner := actionCluster()
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(line, "get pods web-1 -o json"):
			return shell.OK(containerPodJSON)
		case strings.HasPrefix(line, "get pods cache-0 -o json"):
			return shell.OK(`{"metadata":{"name":"cache-0","namespace":"shop","creationTimestamp":"2026-10-21T10:00:00Z"},"spec":{"containers":[{"name":"redis","image":"redis:7"}]},"status":{"phase":"Pending"}}`)
		case strings.HasPrefix(line, "top pod web-1 --containers"):
			return shell.OK("web-1 app 250m 128Mi\nweb-1 side 190m 64Mi\nweb-1 shipper 4m 18Mi\n")
		case strings.HasPrefix(line, "describe pod web-1"):
			return shell.OK("Name: web-1")
		}
		return inner.Handler(r)
	}}
}

func containersModel(t *testing.T, f *shell.Fake) model {
	t.Helper()
	m := selectRow(t, actionModel(t, f), "web-1")
	m.width, m.height = 220, 40
	return pressSettle(t, m, "enter")
}

func TestEnterOnPodOpensContainers(t *testing.T) {
	f := containerCluster()
	m := containersModel(t, f)
	if !m.inContainers() || m.dlg != nil || m.stage != "workpods" {
		t.Fatalf("stage %s dlg %v", m.stage, m.dlg)
	}
	if got := len(m.content.Items()); got != 4 {
		t.Fatalf("%d rows", got)
	}
	if l := m.paneLabel(m.focus); l != "Containers(shop/web-1)[4]" {
		t.Fatalf("label %q", l)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Containers(shop/web-1)[4]", "NAME", "PF", "IMAGE", "READY", "STATE", "INIT", "RESTARTS", "PROBES(L:R)", "CPU", "MEM", "CPU/R:L", "PORTS", "AGE",
		"migrate (init)", "shipper (sidecar)", "app", "side", "Completed", "CrashLoopBackOff", "http:8080,9090", "100:500", "250m"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if n := len(calls(f, "top")); n == 0 {
		t.Fatal("no container metrics fetched")
	}
	for _, c := range calls(f, "top") {
		if strings.HasPrefix(c, "top pods") {
			t.Fatalf("pod metrics fetched in containers view: %s", c)
		}
	}
}

func TestContainersEscKeepsPodSelection(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, actionModel(t, f), "cache-0")
	idx := m.content.Index()
	m = pressSettle(t, m, "enter")
	if !m.inContainers() {
		t.Fatal("cache-0 has a container too, expected containers view")
	}
	m = pressSettle(t, m, "esc")
	if m.inContainers() || m.stage != "workpods" || m.content.Index() != idx {
		t.Fatalf("back: containers %v index %d want %d", m.inContainers(), m.content.Index(), idx)
	}
	if it, _ := m.selectedItem(); it.id != "cache-0" {
		t.Fatalf("selected %q", it.id)
	}
	m = pressSettle(t, selectRow(t, m, "web-1"), "enter", "left")
	if it, _ := m.selectedItem(); m.inContainers() || it.id != "web-1" {
		t.Fatalf("left: %v %q", m.inContainers(), it.id)
	}
}

func TestContainersKeepPodSortAndMarks(t *testing.T) {
	f := containerCluster()
	m := actionModel(t, f)
	m = pressSettle(t, selectRow(t, m, "web-1"), "space")
	marked := len(m.vs.marks)
	sortCol := m.vs.sortCol
	m = pressSettle(t, m, "enter", "esc")
	if len(m.vs.marks) != marked || m.vs.sortCol != sortCol {
		t.Fatalf("marks %d want %d sort %q want %q", len(m.vs.marks), marked, m.vs.sortCol, sortCol)
	}
}

func TestContainersLogsScopedToContainer(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, containersModel(t, f), "side")
	m = pressSettle(t, m, "enter")
	if m.stage != "logs" || m.vs.logCont != "side" || m.dlg != nil {
		t.Fatalf("stage %s cont %q", m.stage, m.vs.logCont)
	}
	logs := calls(f, "logs")
	if len(logs) == 0 || !strings.Contains(logs[len(logs)-1], "web-1 -n shop") || !strings.Contains(logs[len(logs)-1], "-c side") {
		t.Fatalf("logs %v", logs)
	}
	m = pressSettle(t, m, "esc")
	if !m.inContainers() {
		t.Fatalf("esc from logs landed in %s", m.stage)
	}

	m = selectRow(t, m, "shipper")
	m = pressSettle(t, m, "p")
	logs = calls(f, "logs")
	last := logs[len(logs)-1]
	if !m.vs.logPrev || m.vs.logCont != "shipper" || !strings.Contains(last, "--previous") || !strings.Contains(last, "-c shipper") {
		t.Fatalf("previous: %v %q %s", m.vs.logPrev, m.vs.logCont, last)
	}
	m = pressSettle(t, m, "esc")
	m = pressSettle(t, selectRow(t, m, "migrate"), "l")
	if m.stage != "logs" || m.vs.logCont != "migrate" {
		t.Fatalf("l: %s %q", m.stage, m.vs.logCont)
	}
}

func TestContainersShellAndForwardAndDescribe(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, containersModel(t, f), "side")
	next, cmd, handled := m.containerKey("s")
	if !handled || cmd == nil || next.(model).dlg != nil {
		t.Fatalf("shell: handled %v cmd %v", handled, cmd != nil)
	}

	n := pressSettle(t, m, "F")
	if !n.pfOn || !strings.Contains(ansi.Strip(n.View()), "port-forward pod/web-1") {
		t.Fatalf("forward popup: %v %q", n.pfOn, n.status)
	}

	d := pressSettle(t, m, "d")
	desc := calls(f, "describe")
	if d.inContainers() || len(desc) == 0 || !strings.Contains(desc[len(desc)-1], "pod web-1") || !strings.Contains(desc[len(desc)-1], "-n shop") {
		t.Fatalf("describe: stage %s calls %v", d.stage, desc)
	}
}

func TestPortSuggestionsPutContainerPortsFirst(t *testing.T) {
	sug := []kube.PortSuggestion{{Remote: "80"}, {Remote: "9901"}, {Remote: "8080"}}
	got := containerPortsFirst(sug, "9901,8080")
	if got[0].Remote != "9901" || got[1].Remote != "8080" || got[2].Remote != "80" {
		t.Fatalf("%v", got)
	}
}

func TestContainersReadOnlyBlocksShellAndForward(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, containersModel(t, f), "side")
	m.readOnly = true
	for _, k := range []string{"s", "F"} {
		got := pressSettle(t, m, k)
		if got.pfOn || got.dlg != nil || !strings.Contains(got.status, "read-only") {
			t.Fatalf("%s: pf %v status %q", k, got.pfOn, got.status)
		}
	}
	got := pressSettle(t, m, "enter")
	if got.stage != "logs" {
		t.Fatalf("logs are read only, stage %s", got.stage)
	}
}

func TestContainersSwallowPodActions(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, containersModel(t, f), "side")
	for _, k := range []string{"ctrl+d", "ctrl+k", "e", "x", "ctrl+r", "R", "l"} {
		got := pressSettle(t, m, k)
		if k == "l" {
			if got.stage != "logs" {
				t.Fatalf("l: %s", got.stage)
			}
			continue
		}
		if got.dlg != nil || !got.inContainers() {
			t.Fatalf("%s leaked: stage %s dlg %v", k, got.stage, got.dlg)
		}
	}
	if calls(f, "delete") != nil || calls(f, "scale") != nil {
		t.Fatal("mutating call issued")
	}
}

func TestContainersPFIndicator(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, actionModel(t, f), "web-1")
	m.forwards = []forward.Forward{{Namespace: "shop", Pod: "web-1", Kind: "pod", Remote: "8080", Local: "18080", Context: "kind-dev"}}
	m = pressSettle(t, m, "enter")
	var app, side string
	for _, raw := range m.content.Items() {
		it := raw.(item)
		if it.id == "app" {
			app = it.cells[1]
		}
		if it.id == "side" {
			side = it.cells[1]
		}
	}
	if !strings.Contains(app, "●") || strings.Contains(side, "●") {
		t.Fatalf("pf cells app %q side %q", app, side)
	}
}

func TestContainersFitNarrowTerminals(t *testing.T) {
	f := containerCluster()
	for _, w := range []int{200, 140, 100, 70} {
		m := selectRow(t, actionModel(t, f), "web-1")
		m.width, m.height = w, 30
		m = pressSettle(t, m, "enter")
		for i, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if got := len([]rune(line)); got > w {
				t.Fatalf("width %d line %d is %d wide: %q", w, i, got, line)
			}
		}
		if !strings.Contains(ansi.Strip(m.View()), "mig") {
			t.Fatalf("width %d lost the name column", w)
		}
	}
}

func TestContainersWithoutMetricsShowNA(t *testing.T) {
	f := containerCluster()
	m := selectRow(t, actionModel(t, f), "web-1")
	m.cfg.UI.PodMetrics = false
	m.width, m.height = 220, 40
	m = pressSettle(t, m, "enter")
	if !strings.Contains(ansi.Strip(m.View()), "n/a") || len(calls(f, "top")) != 0 {
		t.Fatalf("top calls %v", calls(f, "top"))
	}
}

func TestContainersRefreshReloads(t *testing.T) {
	f := containerCluster()
	m := containersModel(t, f)
	msg := loadRows(m.k, "x", "workpods", "pods", "/kc", "kind-dev", "shop", m.vs.podSel, 200, true)().(rowsMsg)
	if msg.err != nil || len(msg.items) != 4 || !strings.Contains(msg.header, "PROBES(L:R)") {
		t.Fatalf("rows %v %d %q", msg.err, len(msg.items), msg.header)
	}
	m.applyRows(rowsMsg{id: m.logID, header: msg.header, items: msg.items})
	if len(m.content.Items()) != 4 || !m.inContainers() {
		t.Fatalf("applied %d", len(m.content.Items()))
	}
}

func TestPodsListLogsKeysStillWork(t *testing.T) {
	f := containerCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "db-0"), "l")
	if m.stage != "logs" || m.inContainers() {
		t.Fatalf("l on a pod: %s", m.stage)
	}
	m = pressSettle(t, selectRow(t, actionModel(t, f), "db-0"), "p")
	if m.stage != "logs" || !m.vs.logPrev {
		t.Fatalf("p on a pod: %s prev %v", m.stage, m.vs.logPrev)
	}
	m = pressSettle(t, selectRow(t, actionModel(t, f), "web-1"), "l")
	if m.dlg == nil || strings.Join(m.dlg.options, ",") != "app,side,migrate" {
		t.Fatalf("picker %+v", m.dlg)
	}
	m = pressSettle(t, selectRow(t, actionModel(t, f), "web-1"), "right")
	if !m.inContainers() {
		t.Fatalf("right opened %s", m.stage)
	}
}

func TestCommandPodsLeavesContainers(t *testing.T) {
	f := containerCluster()
	m := containersModel(t, f)
	if !m.inContainers() {
		t.Fatal("expected containers")
	}
	for _, q := range []string{"pods", "po", "pod"} {
		if !m.inContainers() {
			t.Fatalf("setup %s: not in containers", q)
		}
		before := len(m.hist)
		m = open(t, m, q)
		if m.inContainers() || m.stage != "workpods" || m.kind != "pods" {
			t.Fatalf(":%s stayed in containers: stage %s kind %s status %q", q, m.stage, m.kind, m.status)
		}
		if m.namespace != "shop" {
			t.Fatalf(":%s ns %q", q, m.namespace)
		}
		if it, ok := m.selectedItem(); !ok || it.id != "web-1" {
			t.Fatalf(":%s selected %+v", q, m.content.SelectedItem())
		}
		if len(m.hist) != before+1 {
			t.Fatalf(":%s hist %d want %d", q, len(m.hist), before+1)
		}
		m = pressSettle(t, m, "q")
		if !m.inContainers() || m.paneLabel(m.focus) != "Containers(shop/web-1)[4]" {
			t.Fatalf("q after :%s: containers %v label %q", q, m.inContainers(), m.paneLabel(m.focus))
		}
	}
	m = open(t, m, "pods")
	n := len(m.hist)
	m = open(t, m, "po")
	if m.inContainers() || len(m.hist) != n {
		t.Fatalf("repeat :po hist %d want %d containers %v", len(m.hist), n, m.inContainers())
	}
}

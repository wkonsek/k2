package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const rke2Nodes = `{"items":[
 {"metadata":{"name":"master1","creationTimestamp":"2026-10-22T10:00:00Z","labels":{"node-role.kubernetes.io/control-plane":"true","node-role.kubernetes.io/etcd":"true"}},
  "status":{"allocatable":{"cpu":"4","memory":"8Gi"},"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.2+rke2r1"}}},
 {"metadata":{"name":"worker1","creationTimestamp":"2026-10-22T10:00:00Z","labels":{"kubernetes.io/hostname":"worker1"}},
  "status":{"allocatable":{"cpu":"4","memory":"8Gi"},"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.2+rke2r1"}}}
]}`

const shopMaps = `{"items":[{"metadata":{"name":"web-config","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"}}]}`

func nodeCluster(top string, tops *[]string) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		joined := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(joined, "top "):
			*tops = append(*tops, joined)
			if top == "" {
				return shell.Fail(1, "error: Metrics API not available")
			}
			return shell.OK(top)
		case strings.HasPrefix(joined, "get nodes"):
			return shell.OK(rke2Nodes)
		case strings.HasPrefix(joined, "get configmaps"):
			return shell.OK(shopMaps)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func openKind(t *testing.T, ns, kind string, metrics bool, top string, tops *[]string) model {
	t.Helper()
	cfg := testEnv(t)
	cfg.UI.PodMetrics = metrics
	m := newTestModel(t, cfg, nodeCluster(top, tops))
	m.kubeconfig, m.context, m.namespace = "/kc", "rke2", ns
	next, cmd := m.openResource(kind)
	m = settle(t, next.(model), cmd)
	if m.stage != "res" || len(m.content.Items()) == 0 {
		t.Fatalf("%s: stage %q items %d status %q", kind, m.stage, len(m.content.Items()), m.status)
	}
	return m
}

func TestNodesViewColumns(t *testing.T) {
	for _, ns := range []string{"", "shop"} {
		m := openKind(t, ns, "nodes", false, "", new([]string))
		it := m.content.Items()[0].(item)
		if got := strings.Join(it.heads, " "); got != "NAME STATUS ROLES AGE VERSION" {
			t.Fatalf("ns %q heads %q", ns, got)
		}
		view := ansi.Strip(m.View())
		if strings.Contains(view, "NAMESPACE") {
			t.Fatalf("ns %q: NAMESPACE column in\n%s", ns, view)
		}
		for _, want := range []string{"ROLES", "VERSION", "control-plane,etcd", "<none>", "v1.34.2+rke2r1"} {
			if !strings.Contains(view, want) {
				t.Fatalf("ns %q: no %q in\n%s", ns, want, view)
			}
		}
		if roles, _ := cellOf(t, m, "worker1", "ROLES"); roles != "<none>" {
			t.Fatalf("worker roles %q", roles)
		}
		if st, _ := cellOf(t, m, "master1", "STATUS"); st != "Ready" {
			t.Fatalf("master status %q", st)
		}
	}
}

func TestAllNamespacesKeepsNamespaceColumn(t *testing.T) {
	m := openKind(t, "", "configmaps", true, "", new([]string))
	it := m.content.Items()[0].(item)
	if strings.Join(it.heads, " ") != "NAMESPACE NAME AGE" || it.cells[0] != "shop" || it.id != "shop/web-config" {
		t.Fatalf("heads %v cells %v id %q", it.heads, it.cells, it.id)
	}
}

func TestNodeMetricsColumns(t *testing.T) {
	var tops []string
	m := openKind(t, "", "nodes", true, "master1   250m   6%   2048Mi   25%\nworker1   <unknown>   <unknown>   <unknown>   <unknown>\n", &tops)
	if got := strings.Join(m.content.Items()[0].(item).heads, " "); got != "NAME STATUS ROLES CPU MEM AGE VERSION" {
		t.Fatalf("heads %q", got)
	}
	m = tickOnce(t, m)
	if len(tops) != 1 || tops[0] != "top nodes --no-headers --request-timeout=5s --kubeconfig /kc --context rke2" {
		t.Fatalf("top calls %v", tops)
	}
	cpu, it := cellOf(t, m, "master1", "CPU")
	if mem, _ := cellOf(t, m, "master1", "MEM"); cpu != "250m" || mem != "2048Mi" {
		t.Fatalf("master1 %q %q", cpu, mem)
	}
	if me := it.meters[indexOf(it.heads, "MEM")]; !me.ok || me.pct != 25 {
		t.Fatalf("mem meter %+v", me)
	}
	if cpu, _ := cellOf(t, m, "worker1", "CPU"); cpu != "-" {
		t.Fatalf("worker1 cpu %q", cpu)
	}
	if strings.Contains(ansi.Strip(m.View()), "NAMESPACE") {
		t.Fatal("NAMESPACE column with metrics")
	}
}

func TestNodeMetricsSkippedWithoutMetricsServer(t *testing.T) {
	var tops []string
	m := tickOnce(t, openKind(t, "", "nodes", true, "", &tops))
	if len(tops) != 1 {
		t.Fatalf("top calls %v", tops)
	}
	if got := strings.Join(m.content.Items()[0].(item).heads, " "); got != "NAME STATUS ROLES AGE VERSION" {
		t.Fatalf("heads %q", got)
	}
	if strings.Contains(m.status, "metrics") {
		t.Fatalf("status %q", m.status)
	}
	if !strings.Contains(ansi.Strip(m.View()), "VERSION") {
		t.Fatal("table lost its columns")
	}
}

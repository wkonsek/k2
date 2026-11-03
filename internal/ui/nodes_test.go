package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const rke2Nodes = `{"items":[
 {"metadata":{"name":"master1","creationTimestamp":"2026-10-22T10:00:00Z","labels":{"node-role.kubernetes.io/control-plane":"true","node-role.kubernetes.io/etcd":"true"}},
  "status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.2+rke2r1"}}},
 {"metadata":{"name":"worker1","creationTimestamp":"2026-10-22T10:00:00Z","labels":{"kubernetes.io/hostname":"worker1"}},
  "status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.34.2+rke2r1"}}}
]}`

const shopMaps = `{"items":[{"metadata":{"name":"web-config","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"}}]}`

func nodeCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		joined := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(joined, "get nodes"):
			return shell.OK(rke2Nodes)
		case strings.HasPrefix(joined, "get configmaps"):
			return shell.OK(shopMaps)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func openKind(t *testing.T, ns, kind string) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), nodeCluster())
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
		m := openKind(t, ns, "nodes")
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
	m := openKind(t, "", "configmaps")
	it := m.content.Items()[0].(item)
	if strings.Join(it.heads, " ") != "NAMESPACE NAME AGE" || it.cells[0] != "shop" || it.id != "shop/web-config" {
		t.Fatalf("heads %v cells %v id %q", it.heads, it.cells, it.id)
	}
}

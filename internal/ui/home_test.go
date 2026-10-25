package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestCtrlDOnHomeClosesPaneTabOrQuits(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, "ctrl+a", "v")
	if len(m.panes) != 2 || m.stage != "nav" {
		t.Fatalf("split: panes %d stage %q", len(m.panes), m.stage)
	}
	m = press(t, m, "ctrl+d")
	if len(m.panes) != 1 || len(m.tabs) != 1 || m.asking() != "" || m.quitting {
		t.Fatalf("pane: panes %d tabs %d confirm %q quit %v", len(m.panes), len(m.tabs), m.asking(), m.quitting)
	}

	m = press(t, m, "ctrl+a", "t")
	if len(m.tabs) != 2 || m.stage != "nav" {
		t.Fatalf("tabs %d stage %q", len(m.tabs), m.stage)
	}
	m = press(t, m, "ctrl+d")
	if len(m.tabs) != 1 || len(m.panes) != 1 || m.asking() != "" || m.quitting || m.stage != "nav" {
		t.Fatalf("tab: tabs %d panes %d confirm %q quit %v stage %q", len(m.tabs), len(m.panes), m.asking(), m.quitting, m.stage)
	}

	next, cmd := m.Update(key(t, "ctrl+d"))
	got := next.(model)
	if !got.quitting || !isQuit(cmd) || got.asking() != "" || len(got.tabs) != 1 {
		t.Fatalf("quit: quitting %v quitcmd %v confirm %q tabs %d", got.quitting, cmd != nil && isQuit(cmd), got.asking(), len(got.tabs))
	}
}

func TestCtrlDOnPodsStillDeletes(t *testing.T) {
	m := newTestModel(t, testEnv(t), nsCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	m = settle(t, next.(model), cmd)

	m = press(t, m, "ctrl+a", "v", "ctrl+left")
	if m.stage != "workpods" || len(m.panes) != 2 {
		t.Fatalf("split pods: stage %q panes %d", m.stage, len(m.panes))
	}
	m = press(t, m, "ctrl+d")
	if m.asking() == "" || len(m.panes) != 2 || m.quitting {
		t.Fatalf("split delete: confirm %q panes %d quit %v", m.asking(), len(m.panes), m.quitting)
	}

	m = press(t, m, "n", "ctrl+a", "t")
	next, cmd = m.openPods()
	m = settle(t, next.(model), cmd)
	m = press(t, m, "ctrl+d")
	if m.asking() == "" || len(m.tabs) != 2 || len(m.panes) != 3 || m.quitting || m.stage != "workpods" {
		t.Fatalf("tab delete: confirm %q tabs %d panes %d quit %v stage %q", m.asking(), len(m.tabs), len(m.panes), m.quitting, m.stage)
	}
}

func nsCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "get projects"), strings.Contains(line, "get ns"):
			return shell.OK("shop\npay\n")
		case strings.Contains(line, "get pods"):
			return shell.OK(podsJSON)
		case strings.Contains(line, "get deployments"):
			return shell.OK(`{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"}}]}`)
		default:
			return shell.OK("")
		}
	}}
}

func TestNamespacePickerReturnsToTheList(t *testing.T) {
	m := newTestModel(t, testEnv(t), nsCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"

	m = pressSettle(t, m, "n")
	if m.stage != "projects" || m.vs.listFrom != "" {
		t.Fatalf("from home: stage %q from %q", m.stage, m.vs.listFrom)
	}
	m = pressSettle(t, m, "down", "down", "enter")
	if m.stage != "nav" || m.namespace != "pay" {
		t.Fatalf("home return: stage %q ns %q", m.stage, m.namespace)
	}

	next, cmd := m.openPods()
	m = settle(t, next.(model), cmd)
	if m.stage != "workpods" {
		t.Fatalf("pods %q", m.stage)
	}
	m = pressSettle(t, m, "n")
	if m.stage != "projects" || m.vs.listFrom != "workpods" {
		t.Fatalf("picker %q from %q", m.stage, m.vs.listFrom)
	}
	m = pressSettle(t, m, "down", "enter")
	if m.stage != "workpods" || m.namespace != "shop" || m.kind != "pods" {
		t.Fatalf("back to pods: stage %q ns %q kind %q", m.stage, m.namespace, m.kind)
	}

	m = pressSettle(t, m, "n", "esc")
	if m.stage != "workpods" || m.namespace != "shop" {
		t.Fatalf("esc: stage %q ns %q", m.stage, m.namespace)
	}

	m.stage, m.kind = "res", "deployments"
	m = pressSettle(t, m, "n", "down", "enter")
	if m.stage != "res" || m.kind != "deployments" || m.namespace != "shop" {
		t.Fatalf("back to deployments: stage %q kind %q ns %q", m.stage, m.kind, m.namespace)
	}
}

func TestOpenShiftContextSetsNamespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `apiVersion: v1
kind: Config
current-context: kind-dev
contexts:
- name: kind-dev
  context: {cluster: kind, namespace: default, user: kind}
- name: shop/api-cluster:6443/kube:admin
  context: {cluster: api, namespace: payments, user: admin}
- name: proj/api-cluster:6443/dev
  context: {cluster: api, user: dev}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(strings.Join(r.Args, " "), "get-contexts") {
			return shell.OK("kind-dev\nshop/api-cluster:6443/kube:admin\nproj/api-cluster:6443/dev\n")
		}
		return shell.Fail(1, "config view should not run")
	}}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = path, "kind-dev", "default"
	m = press(t, m, "c", "down", "enter")
	if m.context != "shop/api-cluster:6443/kube:admin" || m.namespace != "payments" {
		t.Fatalf("field: ctx %q ns %q", m.context, m.namespace)
	}
	m = press(t, m, "c", "down", "down", "enter")
	if m.context != "proj/api-cluster:6443/dev" || m.namespace != "proj" {
		t.Fatalf("oc name: ctx %q ns %q", m.context, m.namespace)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c.Line(), "config view") {
			t.Fatalf("fell back to kubectl: %s", c.Line())
		}
	}
}

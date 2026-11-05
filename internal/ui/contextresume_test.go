package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"
)

func contextCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "get-contexts"):
			return shell.OK("kind-dev\nstaging\n")
		case strings.Contains(line, "get pods"):
			return shell.OK(podsJSON)
		case strings.Contains(line, "get secrets"), strings.Contains(line, "get secret"):
			return shell.OK(`{"items":[{"metadata":{"name":"s","namespace":"shop","creationTimestamp":"2024-01-01T00:00:00Z"},"type":"Opaque","data":{}}]}`)
		case strings.Contains(line, "api-resources"):
			return shell.OK("secrets                         secret                         true   Secret\npods                            po                            true   Pod\n")
		default:
			return shell.Fail(1, "unexpected "+line)
		}
	}}
}

func TestContextReturnsToPreviousView(t *testing.T) {
	m := newTestModel(t, testEnv(t), contextCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	m = settle(t, next.(model), cmd)
	if m.stage != "workpods" {
		t.Fatalf("stage %s", m.stage)
	}
	next, _ = m.openNamed("ctx")
	m = next.(model)
	if m.stage != "contexts" || m.vs.listFrom != "workpods" {
		t.Fatalf("ctx picker: stage=%s from=%q", m.stage, m.vs.listFrom)
	}
	m.content.Select(0)
	next, cmd = m.handleContentEnter()
	m = settle(t, next.(model), cmd)
	if m.stage != "workpods" {
		t.Fatalf("resume stage %s", m.stage)
	}
}

func TestContextEscReturnsToPreviousView(t *testing.T) {
	m := newTestModel(t, testEnv(t), contextCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	next, _ = m.openNamed("ctx")
	m = next.(model)
	if m.vs.listFrom != "res" {
		t.Fatalf("from %q", m.vs.listFrom)
	}
	cmd = m.back()
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if m.stage != "res" {
		t.Fatalf("esc stage %s", m.stage)
	}
}

func TestUnknownResourceShowsToast(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.resources = []string{"pods", "services"}
	next, cmd := m.runCommand("franek")
	m = next.(model)
	if !strings.Contains(m.status, "unknown resource: franek") {
		t.Fatalf("status %q", m.status)
	}
	if cmd == nil {
		t.Fatal("expected notify cmd")
	}
}

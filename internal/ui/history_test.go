package ui

import (
	"fmt"
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
)

func viewHistCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "get-contexts"):
			return shell.OK("kind-dev\nstaging\n")
		case strings.HasPrefix(line, "get pods web-1 -o json"):
			return shell.OK(containerPodJSON)
		case strings.Contains(line, "get pods"):
			return shell.OK(actionPods)
		case strings.Contains(line, "get secrets"), strings.Contains(line, "get secret"):
			return shell.OK(`{"items":[
				{"metadata":{"name":"db-cred","namespace":"shop","creationTimestamp":"2026-10-20T10:00:00Z"},"type":"Opaque","data":{}},
				{"metadata":{"name":"web-tls","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},"type":"kubernetes.io/tls","data":{}}
			]}`)
		case strings.Contains(line, "get ns"), strings.Contains(line, "get projects"):
			return shell.OK("shop\npay\n")
		case strings.Contains(line, "api-resources"):
			return shell.OK("secrets                         secret                         true   Secret\npods                            po                            true   Pod\n")
		default:
			return shell.OK("")
		}
	}}
}

func histModel(t *testing.T) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), viewHistCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	return settle(t, next.(model), cmd)
}

func TestHistoryBackRestoresThePreviousView(t *testing.T) {
	m := histModel(t)
	m = selectRow(t, m, "db-0")
	m = open(t, m, "secrets")
	if m.stage != "res" || m.kind != "secrets" {
		t.Fatalf("secrets: %s %s", m.stage, m.kind)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "workpods" || m.kind != "pods" {
		t.Fatalf("q: stage %s kind %s", m.stage, m.kind)
	}
	if it, ok := m.selectedItem(); !ok || it.id != "db-0" {
		t.Fatalf("selection %+v", m.content.SelectedItem())
	}
	if m.namespace != "shop" {
		t.Fatalf("ns %q", m.namespace)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "nav" || m.quitting {
		t.Fatalf("second q: stage %s quit %v", m.stage, m.quitting)
	}
}

func TestHistoryRestoresNamespaceFilterAndSelection(t *testing.T) {
	m := histModel(t)
	m = selectRow(t, m, "db-0")
	m = pressSettle(t, m, "/")
	m = typeText(t, m, "db")
	m = pressSettle(t, m, "enter")
	if m.content.FilterState() != list.FilterApplied || m.content.FilterValue() != "db" {
		t.Fatalf("filter %v %q", m.content.FilterState(), m.content.FilterValue())
	}
	m = open(t, m, "secrets")
	m.namespace = "pay"
	m = pressSettle(t, m, "q")
	if m.stage != "workpods" || m.namespace != "shop" {
		t.Fatalf("restored %s ns %q", m.stage, m.namespace)
	}
	if m.content.FilterState() != list.FilterApplied || m.content.FilterValue() != "db" {
		t.Fatalf("filter after back: %v %q", m.content.FilterState(), m.content.FilterValue())
	}
	if it, ok := m.selectedItem(); !ok || it.id != "db-0" {
		t.Fatalf("sel %+v", m.content.SelectedItem())
	}
}

func TestHomeClearsHistory(t *testing.T) {
	m := histModel(t)
	m = open(t, m, "secrets")
	if len(m.hist) == 0 {
		t.Fatal("expected a history entry")
	}
	next, cmd := m.goHome()
	m = settle(t, next.(model), cmd)
	if m.stage != "nav" || m.quitting || len(m.hist) != 0 {
		t.Fatalf("home: stage %s quit %v hist %d", m.stage, m.quitting, len(m.hist))
	}
	next, cmd = m.goHome()
	m = settle(t, next.(model), cmd)
	if m.stage != "nav" || m.quitting {
		t.Fatalf("goHome on home: stage %s quit %v", m.stage, m.quitting)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "nav" || m.quitting {
		t.Fatalf("q on home: stage %s quit %v", m.stage, m.quitting)
	}
}

func TestContextSwitchClearsHistory(t *testing.T) {
	m := histModel(t)
	m = open(t, m, "secrets")
	if len(m.hist) == 0 {
		t.Fatal("expected a history entry")
	}
	next, _ := m.openNamed("ctx")
	m = next.(model)
	m.content.Select(1)
	next, cmd := m.handleContentEnter()
	m = settle(t, next.(model), cmd)
	if m.context != "staging" {
		t.Fatalf("context %q", m.context)
	}
	if len(m.hist) != 0 {
		t.Fatalf("history survived the context switch: %d", len(m.hist))
	}
	m = pressSettle(t, m, "q")
	if m.stage != "nav" {
		t.Fatalf("q after a context switch went to %s", m.stage)
	}
}

func TestHistoryIsCappedAndSkipsDuplicates(t *testing.T) {
	m := histModel(t)
	m.stage, m.kind = "res", "k0"
	for i := 0; i < 60; i++ {
		m.kind = fmt.Sprintf("k%d", i)
		m.rememberView()
	}
	if len(m.hist) != histCap {
		t.Fatalf("cap %d, want %d", len(m.hist), histCap)
	}
	if m.hist[0].kind != "k10" || m.hist[histCap-1].kind != "k59" {
		t.Fatalf("window %s..%s", m.hist[0].kind, m.hist[histCap-1].kind)
	}
	n := len(m.hist)
	m.rememberView()
	if len(m.hist) != n {
		t.Fatalf("duplicate pushed: %d", len(m.hist))
	}
}

func TestHistoryDoesNotPushTheSameListAgain(t *testing.T) {
	m := histModel(t)
	if len(m.hist) != 0 {
		t.Fatalf("open from home pushed %d", len(m.hist))
	}
	m = open(t, m, "pods")
	if len(m.hist) != 0 {
		t.Fatalf(":pods again pushed %d", len(m.hist))
	}
	m = open(t, m, "secrets")
	if len(m.hist) != 1 || m.hist[0].stage != "workpods" {
		t.Fatalf("hist %+v", m.hist)
	}
	m = open(t, m, "secrets")
	if len(m.hist) != 1 {
		t.Fatalf("reopened secrets pushed %d", len(m.hist))
	}
}

func TestContainersBackStaysConsistentThenPops(t *testing.T) {
	m := histModel(t)
	m = open(t, m, "secrets")
	m = open(t, m, "pods")
	m = selectRow(t, m, "web-1")
	m = pressSettle(t, m, "enter")
	if !m.inContainers() {
		t.Fatalf("not in containers: %s %q", m.stage, m.vs.podSel)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "workpods" || m.inContainers() {
		t.Fatalf("q from containers: stage %s sel %q", m.stage, m.vs.podSel)
	}
	if it, ok := m.selectedItem(); !ok || it.id != "web-1" {
		t.Fatalf("pod sel %+v", m.content.SelectedItem())
	}
	m = pressSettle(t, m, "q")
	if m.stage != "res" || m.kind != "secrets" {
		t.Fatalf("q from pods: %s %s", m.stage, m.kind)
	}
}

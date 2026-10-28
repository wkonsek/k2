package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

const podsJSON = `{"items":[
 {"metadata":{"name":"web-1","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"node-a"},
  "status":{"phase":"Running","podIP":"10.0.0.7","containerStatuses":[
    {"ready":true,"restartCount":2,"state":{}},
    {"ready":false,"restartCount":1,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
 {"metadata":{"name":"api-0","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"node-b"},
  "status":{"phase":"Running","podIP":"10.0.0.8","containerStatuses":[{"ready":true,"restartCount":0,"state":{}}]}}
]}`

func podCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 1 && r.Args[0] == "get" && r.Args[1] == "pods" {
			return shell.OK(podsJSON)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
}

func TestElapsed(t *testing.T) {
	cases := map[time.Duration]string{0: "0.0s", 800 * time.Millisecond: "0.8s", 9940 * time.Millisecond: "9.9s",
		12 * time.Second: "12s", 65 * time.Second: "1m05s", -time.Second: "0.0s"}
	for d, want := range cases {
		if got := elapsed(d); got != want {
			t.Errorf("elapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestProgressBar(t *testing.T) {
	if got := ansi.Strip(progressBar(3, 8, 16)); got != "━━━━━━──────────" {
		t.Fatalf("bar %q", got)
	}
	if got := ansi.Strip(progressBar(9, 8, 4)); got != "━━━━" {
		t.Fatalf("overfull bar %q", got)
	}
	if progressBar(1, 0, 10) != "" {
		t.Fatal("unknown total must draw nothing")
	}
}

func TestOpenPodsShowsSpinnerThenTable(t *testing.T) {
	cfg := testEnv(t)
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	fixClock(t, t0)
	m := newTestModel(t, cfg, podCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	m = next.(model)
	if m.stage != "nav" || !m.load.active() {
		t.Fatalf("stage %q load %+v", m.stage, m.load)
	}
	now = func() time.Time { return t0.Add(800 * time.Millisecond) }
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "╭ 1 kc:kind-dev · home · ") || !strings.Contains(view, "loading pods… 0.8s") {
		t.Fatalf("no spinner in the header:\n%s", view)
	}
	if !strings.Contains(view, "/kc") || !strings.Contains(view, "shop") {
		t.Fatal("pane went blank while loading")
	}
	m = settle(t, m, cmd)
	if m.load.active() || m.stage != "workpods" || len(m.content.Items()) != 2 {
		t.Fatalf("stage %q items %d", m.stage, len(m.content.Items()))
	}
	if m.status != "enter logs, d describe, ctrl+f forward, s shell, e edit, ctrl+d delete" {
		t.Fatalf("status %q", m.status)
	}
	if strings.Contains(ansi.Strip(m.View()), "loading pods") {
		t.Fatal("spinner still shown")
	}
}

func TestStaleListResultsAreDropped(t *testing.T) {
	cfg := testEnv(t)
	base := newTestModel(t, cfg, podCluster())
	base.kubeconfig, base.context, base.namespace = "/kc", "kind-dev", "shop"

	next, cmd := base.openPods()
	m := press(t, next.(model), "esc")
	if m.load.active() || m.status != "loading pods cancelled" {
		t.Fatalf("load %+v status %q", m.load, m.status)
	}
	if m = settle(t, m, cmd); m.stage != "nav" {
		t.Fatalf("cancelled load still opened %q", m.stage)
	}

	next, cmd = base.openPods()
	m = press(t, next.(model), "f")
	if m = settle(t, m, cmd); m.stage != "fwds" {
		t.Fatalf("late rows replaced the forwards view: %q", m.stage)
	}

	next, first := base.openPods()
	next, _ = next.(model).openResource("secrets")
	m = settle(t, next.(model), first)
	if m.stage != "nav" || !m.load.active() || m.load.what != "loading secrets" {
		t.Fatalf("old result applied: stage %q load %+v", m.stage, m.load)
	}
}

func TestListLoadsIntoItsOwnPane(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, podCluster())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	m = press(t, next.(model), "ctrl+a", "h")
	if !strings.Contains(ansi.Strip(m.View()), "loading pods") {
		t.Fatal("unfocused pane lost its spinner")
	}
	m = settle(t, m, cmd)
	if m.focus != 1 || m.stage != "nav" {
		t.Fatalf("focused pane changed: focus %d stage %q", m.focus, m.stage)
	}
	if p := m.panes[0]; p.stage != "workpods" || p.load.active() || len(p.content.Items()) != 2 {
		t.Fatalf("pane 1: stage %q items %d", p.stage, len(p.content.Items()))
	}
}

func TestRunOutputGoesToItsPane(t *testing.T) {
	cfg := testEnv(t)
	f := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.OK("NAME\nweb\n") }}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = press(t, m, "U", "g", "e", "t", " ", "p", "o")
	next, cmd := m.Update(key(t, "enter"))
	m = next.(model)
	if body := ansi.Strip(m.rightBody(80)); !strings.Contains(body, "running kubectl…") {
		t.Fatalf("blank body while running: %q", body)
	}
	m = press(t, m, "ctrl+a", "h")
	m = settle(t, m, cmd)
	if m.focus != 1 || m.stage != "nav" {
		t.Fatalf("output landed in the focused pane: %q", m.stage)
	}
	if p := m.panes[0]; p.stage != "kout" || !strings.Contains(p.logText, "web") {
		t.Fatalf("pane 1: stage %q text %q", p.stage, p.logText)
	}
}

func TestNoAnimations(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.Animations = false
	m := newTestModel(t, cfg, podCluster())
	m.kubeconfig, m.context = "/kc", "kind-dev"
	next, _ := m.openPods()
	m = next.(model)
	if m.spinning || !strings.Contains(ansi.Strip(m.View()), "• loading pods…") {
		t.Fatal("animation not off")
	}
}

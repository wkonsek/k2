package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func lineWith(view string, parts ...string) string {
	for _, l := range strings.Split(view, "\n") {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(l, p)
		}
		if ok {
			return l
		}
	}
	return ""
}

func TestSplitRightSideBySide(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	wide := m.tableWidth()
	m = press(t, m, "ctrl+a", "v")
	if len(m.panes) != 2 || m.tabPos(m.logID) != 1 || m.context != "kind-dev" || m.namespace != "shop" || m.kubeconfig != "/kc" {
		t.Fatalf("panes %d pos %d target %s/%s", len(m.panes), m.tabPos(m.logID), m.context, m.namespace)
	}
	view := ansi.Strip(m.View())
	if lineWith(view, "╭ 1 kc:kind-dev · home", "╭ 2 kc:kind-dev · home") == "" {
		t.Fatalf("panes not side by side:\n%s", view)
	}
	if got := m.tableWidth(); got >= wide || got < wide/2-4 {
		t.Fatalf("table width %d (was %d)", got, wide)
	}

	m = press(t, m, "ctrl+a", "h")
	view = ansi.Strip(m.View())
	if lineWith(view, "╭ 1 kc:kind-dev · home", "╭ 2 kc:kind-dev · home") == "" || lineWith(view, "│", "╭ 3 ") == "" {
		t.Fatalf("layout:\n%s", view)
	}
	if m.status != "pane 3/3" {
		t.Fatalf("status %q", m.status)
	}
}

func TestSplitDownStacksAtTheBottom(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, "ctrl+a", "h", "ctrl+a", "h", "ctrl+a", "o")
	m = press(t, m, "ctrl+a", "h")
	if got := m.tabPos(m.logID); got != 3 {
		t.Fatalf("new pane at position %d", got)
	}
	view := ansi.Strip(m.View())
	for i, want := range []string{"╭ 1 ", "╭ 2 ", "╭ 3 ", "╭ 4 "} {
		if strings.Count(view, want) != 1 {
			t.Fatalf("pane %d missing:\n%s", i+1, view)
		}
	}
}

func TestSplitLimits(t *testing.T) {
	cfg := testEnv(t)
	cfg.MaxPanes = 3
	m := newTestModel(t, cfg, nil)
	m = press(t, m, "ctrl+a", "v", "ctrl+a", "h", "ctrl+a", "h")
	if len(m.panes) != 3 || m.status != "3 panes max" || m.toast.text != "3 panes max" {
		t.Fatalf("panes %d status %q toast %q", len(m.panes), m.status, m.toast.text)
	}
	small := newTestModel(t, testEnv(t), nil)
	small.width, small.height = 80, 24
	small = press(t, small, "ctrl+a", "v", "ctrl+a", "v")
	if len(small.panes) != 2 || small.status != "no room for another pane here" {
		t.Fatalf("panes %d status %q", len(small.panes), small.status)
	}
}

func TestClosePaneCollapses(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	first := m.logID
	m = press(t, m, "ctrl+a", "v", "ctrl+a", "h")
	m = press(t, m, "ctrl+up")
	m = press(t, m, "ctrl+a", "x", "y")
	if len(m.panes) != 2 || m.tabs[0].root.Count() != 2 || m.status != "pane 2/2" {
		t.Fatalf("panes %d status %q", len(m.panes), m.status)
	}
	if m.tabs[0].root.Dir != 1 || len(m.tabs[0].root.Kids) != 2 {
		t.Fatalf("tree not collapsed: %+v", m.tabs[0].root)
	}
	m = press(t, m, "ctrl+a", "x", "y")
	if len(m.panes) != 1 || m.logID != first {
		t.Fatalf("panes %d", len(m.panes))
	}
	m = press(t, m, "ctrl+a", "x")
	if m.status != "last pane" || m.asking() != "" {
		t.Fatalf("status %q confirm %q", m.status, m.asking())
	}
}

func TestSpatialFocus(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	a := m.logID
	m = press(t, m, "ctrl+a", "h")
	d := m.logID
	m = press(t, m, "ctrl+up", "ctrl+a", "v")
	b := m.logID
	m = press(t, m, "ctrl+a", "h")
	c := m.logID
	steps := []struct{ key, want string }{
		{"ctrl+left", a}, {"ctrl+right", b}, {"ctrl+down", c}, {"ctrl+down", d},
		{"ctrl+up", a}, {"ctrl+down", d}, {"ctrl+down", d}, {"ctrl+a o", a}, {"ctrl+a o", b},
	}
	for i, s := range steps {
		m = press(t, m, strings.Fields(s.key)...)
		if m.logID != s.want {
			t.Fatalf("step %d %s: focus %s, want %s", i, s.key, m.logID, s.want)
		}
	}
}

func TestCtrlVInKubectlLineDoesNotSplit(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = press(t, m, "U", "ctrl+v")
	if len(m.panes) != 1 || m.stage != "kcmd" {
		t.Fatalf("panes %d stage %q", len(m.panes), m.stage)
	}
}

func TestResizeRefits(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, "ctrl+a", "v")
	before := m.tableWidth()
	nm, _ := m.Update(windowSize(200, 50))
	m = nm.(model)
	if m.tableWidth() <= before {
		t.Fatalf("table width %d after growing (was %d)", m.tableWidth(), before)
	}
	view := ansi.Strip(m.View())
	if lines := strings.Split(view, "\n"); len(lines) != 50 || ansi.StringWidth(lines[0]) > 200 {
		t.Fatalf("view %d lines, first %d wide", len(lines), ansi.StringWidth(lines[0]))
	}
}

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

func TestSplitReloadsTables(t *testing.T) {
	f := podCluster()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	n, c := m.openPods()
	m = settle(t, n.(model), c)
	calls := len(f.Calls())
	m = pressSettle(t, m, "ctrl+a", "v")
	if len(f.Calls()) != calls+1 {
		t.Fatalf("kubectl calls %d → %d", calls, len(f.Calls()))
	}
	m = press(t, m, "ctrl+left")
	if w := m.tableWidth(); w > 50 {
		t.Fatalf("pods pane width %d", w)
	}
}

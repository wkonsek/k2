package ui

import (
	"testing"
)

func TestTeaSplitRight(t *testing.T) {
	tm := start(t, newTestModel(t, testEnv(t), nil))
	send(t, tm, "ctrl+a", "v")
	waitFor(t, tm, "2 home")
	m := finish(t, tm)
	if len(m.panes) != 2 || m.tabs[0].root.Dir != 1 {
		t.Fatalf("panes %d root %+v", len(m.panes), m.tabs[0].root)
	}
}

func TestTeaTabs(t *testing.T) {
	tm := start(t, newTestModel(t, testEnv(t), nil))
	send(t, tm, "ctrl+a", "t", "ctrl+a", "t")
	waitFor(t, tm, "3 home")
	send(t, tm, "ctrl+a", "2", "ctrl+a", "]", "ctrl+a", "]", "ctrl+a", "[")
	m := finish(t, tm)
	if len(m.tabs) != 3 || m.tab != 2 {
		t.Fatalf("tabs %d active %d", len(m.tabs), m.tab)
	}
}

func TestTeaCloseTabAsks(t *testing.T) {
	tm := start(t, newTestModel(t, testEnv(t), nil))
	send(t, tm, "ctrl+a", "t", "ctrl+a", "w")
	waitFor(t, tm, "close tab 2?")
	send(t, tm, "n", "ctrl+a", "w", "y")
	waitFor(t, tm, "tab 1/1")
	m := finish(t, tm)
	if len(m.tabs) != 1 || len(m.panes) != 1 {
		t.Fatalf("tabs %d panes %d", len(m.tabs), len(m.panes))
	}
}

func TestTeaBracketInKubectlLine(t *testing.T) {
	tm := start(t, newTestModel(t, testEnv(t), nil))
	send(t, tm, "ctrl+a", "t", "U", "[", "]")
	waitFor(t, tm, "kubectl []")
	m := finish(t, tm)
	if m.tab != 1 || m.sh.Value() != "[]" {
		t.Fatalf("tab %d line %q", m.tab, m.sh.Value())
	}
}

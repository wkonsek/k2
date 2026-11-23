package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"
)

func markRows(t *testing.T, m model, ids ...string) model {
	t.Helper()
	for _, id := range ids {
		m = selectRow(t, m, id)
		m = press(t, m, " ")
	}
	if len(m.vs.marks) != len(ids) {
		t.Fatalf("marks %d want %d status %q", len(m.vs.marks), len(ids), m.status)
	}
	return m
}

func TestBackClearsMarksThenGoesBack(t *testing.T) {
	for _, key := range []string{"esc", "q", "H"} {
		t.Run(key, func(t *testing.T) {
			m := histModel(t)
			m = open(t, m, "secrets")
			if m.stage != "res" || len(m.hist) == 0 {
				t.Fatalf("setup stage %s hist %d", m.stage, len(m.hist))
			}
			hist := len(m.hist)
			m = markRows(t, m, "db-cred", "web-tls")
			stage, kind := m.stage, m.kind
			m = press(t, m, key)
			if len(m.vs.marks) != 0 || m.stage != stage || m.kind != kind || len(m.hist) != hist {
				t.Fatalf("first %s: marks %d stage %s/%s hist %d (want %d) status %q", key, len(m.vs.marks), m.stage, m.kind, len(m.hist), hist, m.status)
			}
			if m.status != "cleared 2 selections" || m.toast.text != "cleared 2 selections" {
				t.Fatalf("%s flash status %q toast %q", key, m.status, m.toast.text)
			}
			m = pressSettle(t, m, key)
			if m.stage != "workpods" || m.kind != "pods" {
				t.Fatalf("second %s: stage %s kind %s", key, m.stage, m.kind)
			}
		})
	}
}

func TestBackWithoutMarksStillGoesBack(t *testing.T) {
	m := histModel(t)
	m = open(t, m, "secrets")
	m = pressSettle(t, m, "esc")
	if m.stage != "workpods" || m.kind != "pods" {
		t.Fatalf("stage %s kind %s", m.stage, m.kind)
	}
}

func TestBackClearsOnePodMark(t *testing.T) {
	m := histModel(t)
	m = markRows(t, m, "db-0")
	m = press(t, m, "esc")
	if len(m.vs.marks) != 0 || m.stage != "workpods" || m.status != "cleared 1 selection" {
		t.Fatalf("marks %d stage %s status %q", len(m.vs.marks), m.stage, m.status)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "nav" {
		t.Fatalf("second esc: %s", m.stage)
	}
}

func TestBackClearsMarksBeforeFilter(t *testing.T) {
	m := histModel(t)
	m = markRows(t, m, "db-0")
	m = pressSettle(t, m, "/")
	m = typeText(t, m, "db")
	m = pressSettle(t, m, "enter")
	if m.content.FilterState() != list.FilterApplied || m.content.FilterValue() != "db" {
		t.Fatalf("filter %v %q", m.content.FilterState(), m.content.FilterValue())
	}
	if len(m.vs.marks) != 1 {
		t.Fatalf("marks lost under filter: %d", len(m.vs.marks))
	}
	m = press(t, m, "esc")
	if len(m.vs.marks) != 0 || m.content.FilterState() != list.FilterApplied || m.stage != "workpods" {
		t.Fatalf("first esc: marks %d filter %v stage %s", len(m.vs.marks), m.content.FilterState(), m.stage)
	}
	m = press(t, m, "esc")
	if m.content.FilterState() == list.FilterApplied || m.stage != "workpods" {
		t.Fatalf("second esc: filter %v stage %s", m.content.FilterState(), m.stage)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "nav" {
		t.Fatalf("third esc: %s", m.stage)
	}
}

func TestBackClearsMarksInOnePaneOnly(t *testing.T) {
	m := histModel(t)
	m.width, m.height = 200, 40
	m = markRows(t, m, "db-0", "web-1")
	m = pressSettle(t, m, "ctrl+a", "v")
	if len(m.panes) != 2 {
		t.Fatalf("panes %d", len(m.panes))
	}
	left := 0
	if len(m.panes[left].vs.marks) != 2 {
		t.Fatalf("left marks %d after split", len(m.panes[left].vs.marks))
	}
	m = open(t, m, "pods")
	m = markRows(t, m, "cache-0")
	hist, stage := len(m.hist), m.stage
	m = press(t, m, "esc")
	if len(m.vs.marks) != 0 || m.stage != stage || len(m.hist) != hist {
		t.Fatalf("focused: marks %d stage %s hist %d (want %d)", len(m.vs.marks), m.stage, len(m.hist), hist)
	}
	if len(m.panes[left].vs.marks) != 2 || m.panes[left].stage != "workpods" {
		t.Fatalf("other pane marks %d stage %s", len(m.panes[left].vs.marks), m.panes[left].stage)
	}
}

func TestClearMarksHintInFooter(t *testing.T) {
	m := histModel(t)
	if strings.Contains(ansi.Strip(m.legend()), "clear") {
		t.Fatalf("clear hint without marks: %s", ansi.Strip(m.legend()))
	}
	m = markRows(t, m, "db-0", "web-1")
	leg := ansi.Strip(m.legend())
	if !strings.Contains(leg, "q/esc") || !strings.Contains(leg, "clear 2") {
		t.Fatalf("legend %q", leg)
	}
	if !strings.Contains(m.status, "2 selected") || !strings.Contains(m.status, "q/esc clear") {
		t.Fatalf("status %q", m.status)
	}
	foot := ansi.Strip(m.toastLabel(80))
	if !strings.Contains(foot, "2 selected") || !strings.Contains(foot, "q/esc clear") {
		t.Fatalf("footer %q", foot)
	}
	m = press(t, m, "esc")
	if strings.Contains(ansi.Strip(m.legend()), "clear 2") {
		t.Fatalf("legend after clear: %s", ansi.Strip(m.legend()))
	}
	if !strings.Contains(ansi.Strip(m.toastLabel(80)), "cleared 2 selections") {
		t.Fatalf("toast footer %q", ansi.Strip(m.toastLabel(80)))
	}
}

func TestHelpMentionsClearMarks(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.width, m.height = 120, 35
	for _, c := range []struct{ stage, kind string }{{"workpods", ""}, {"res", "secrets"}} {
		text := helpText(helpAt(t, m, c.stage, c.kind))
		if !strings.Contains(text, "mark rows") || !strings.Contains(text, "clear marks") {
			t.Fatalf("%s %s help:\n%s", c.stage, c.kind, text)
		}
	}
}

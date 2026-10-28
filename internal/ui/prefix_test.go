package ui

import (
	"strings"
	"testing"

	"k2/internal/config"

	"github.com/charmbracelet/x/ansi"
)

func TestPrefixKeys(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = press(t, m, "ctrl+a")
	if !m.prefixOn || !strings.HasPrefix(m.status, "prefix ctrl+a") {
		t.Fatalf("no indicator: %v %q", m.prefixOn, m.status)
	}
	l := ansi.Strip(m.legend())
	if !strings.Contains(l, "prefix ^a") || !strings.Contains(l, "esc cancel") {
		t.Fatalf("legend %q", l)
	}

	for _, want := range []string{`h/" split ↓`, "v/% split →", "o next pane", "; last", "t/c tab", "w/& close tab", ", rename", "n/p tabs", "z/Z full"} {
		if !strings.Contains(l, want) {
			t.Fatalf("legend lacks %q: %q", want, l)
		}
	}
	if strings.Contains(l, "prev pane") {
		t.Fatalf("legend %q", l)
	}
	m = press(t, m, "esc")
	if m.prefixOn || m.status != "" || len(m.tabIDs()) != 1 {
		t.Fatalf("esc: %v %q", m.prefixOn, m.status)
	}

	m = press(t, m, "ctrl+a")
	stale, _ := m.Update(prefixTimeoutMsg{tok: m.prefixTok - 1})
	if !stale.(model).prefixOn {
		t.Fatal("a stale timeout ended the wait")
	}
	next, _ := m.Update(prefixTimeoutMsg{tok: m.prefixTok})
	m = next.(model)
	if m.prefixOn || strings.HasPrefix(m.status, "prefix") {
		t.Fatalf("timeout: %v %q", m.prefixOn, m.status)
	}

	m = press(t, m, "ctrl+a", "d")
	if m.stage != "workpods" || !strings.Contains(m.status, "ctrl+a d: not a pane or tab key") {
		t.Fatalf("stage %q status %q", m.stage, m.status)
	}

	m = pressSettle(t, m, "ctrl+a", "v")
	if len(m.tabIDs()) != 2 || m.tabPos(m.logID) != 1 {
		t.Fatalf("ctrl+a v: panes %d", len(m.tabIDs()))
	}
	m = pressSettle(t, m, "ctrl+a", ";")
	if m.tabPos(m.logID) != 0 {
		t.Fatal("ctrl+a ;")
	}

	m = showLog(m, "a\nb\n")
	m = pressSettle(t, m, "ctrl+a", "h")
	if len(m.tabIDs()) != 3 {
		t.Fatalf("ctrl+a h from logs: panes %d", len(m.tabIDs()))
	}
	m = press(t, m, "ctrl+a", "o", "ctrl+a", "o", "ctrl+a", "t")
	if len(m.tabs) != 2 || m.tab != 1 {
		t.Fatalf("ctrl+a t: tabs %d", len(m.tabs))
	}
	m = press(t, m, "ctrl+a", "1")
	if m.tab != 0 {
		t.Fatal("ctrl+a 1")
	}
	m = press(t, m, "ctrl+a", "]", "ctrl+a", "[", "ctrl+a", "]")
	if m.tab != 1 {
		t.Fatalf("ctrl+a ] [ ]: tab %d", m.tab)
	}
	m = press(t, m, "ctrl+a", "w", "y")
	if len(m.tabs) != 1 {
		t.Fatal("ctrl+a w y")
	}
	m = press(t, m, "ctrl+a", "x", "y")
	if len(m.tabIDs()) != 2 {
		t.Fatalf("ctrl+a x y: panes %d", len(m.tabIDs()))
	}

	m = press(t, m, "ctrl+a", "z")
	if !m.zoom {
		t.Fatal("ctrl+a z")
	}
}

func TestPrefixNotInInputs(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = press(t, m, ":", "a", "b", "c", "left", "left", "ctrl+a", "v")
	if m.prefixOn || len(m.tabIDs()) != 1 || m.cmd.Value() != "vabc" {
		t.Fatalf(": line: prefix %v panes %d value %q", m.prefixOn, len(m.tabIDs()), m.cmd.Value())
	}
	m = press(t, m, "esc", "ctrl+a", "h", "ctrl+a", "x")
	if m.asking() != "✗ close pane" {
		t.Fatalf("confirm %q", m.asking())
	}
	m = press(t, m, "ctrl+a")
	if m.prefixOn {
		t.Fatal("prefix during a y/n prompt")
	}
	m = press(t, m, "n")
}

func TestPrefixOverridesAndDirect(t *testing.T) {
	for _, p := range []string{"ctrl+k", "ctrl+\\"} {
		cfg := testEnv(t)
		cfg.Keys.Prefix = config.Binding{p}
		cfg.Keys.NextPane = config.Binding{"n", config.DirectMark + "O"}
		m := newTestModel(t, cfg, nil)
		m = press(t, m, "ctrl+a", "v")
		if len(m.tabIDs()) != 1 {
			t.Fatalf("%s: ctrl+a still the prefix", p)
		}
		m = press(t, m, p, "v")
		if len(m.tabIDs()) != 2 || m.tabPos(m.logID) != 1 {
			t.Fatalf("%s v: panes %d", p, len(m.tabIDs()))
		}
		m = press(t, m, "O")
		if m.tabPos(m.logID) != 0 {
			t.Fatalf("%s: direct O did not cycle", p)
		}
		m = press(t, m, p, "n")
		if m.tabPos(m.logID) != 1 {
			t.Fatalf("%s n did not cycle", p)
		}
		if l := ansi.Strip(m.legend()); !strings.Contains(l, strings.ReplaceAll(p, "ctrl+", "^")+" x close") {
			t.Fatalf("legend %q", l)
		}
	}
}

func TestTabKeysDirectAndPrefixed(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = press(t, m, "ctrl+a", "t", "ctrl+a", "t")
	m = press(t, m, "]")
	if m.tab != 0 {
		t.Fatalf("] from a table: tab %d", m.tab)
	}
	m = press(t, m, "[", "[")
	if m.tab != 1 {
		t.Fatalf("[ [: tab %d", m.tab)
	}
	m = press(t, m, "ctrl+a", "]")
	if m.tab != 2 {
		t.Fatalf("ctrl+a ]: tab %d", m.tab)
	}
	m = showLog(m, "x\n")
	m = press(t, m, "]")
	if m.tab != 0 {
		t.Fatalf("] from a viewer: tab %d", m.tab)
	}

	m = press(t, m, ":", "[", "]")
	if m.tab != 0 || m.cmd.Value() != "[]" {
		t.Fatalf(": line: tab %d value %q", m.tab, m.cmd.Value())
	}
	m = press(t, m, "esc")
	if l := ansi.Strip(m.legend()); !strings.Contains(l, "[/] tabs") {
		t.Fatalf("legend %q", l)
	}
}

func TestTabKeysInsideEditor(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := newEdHarness(t, m)
	h.keys("ctrl+a", "t", "ctrl+a", "1")
	startCat(t, h)
	h.keys("]", "[")
	h.pump(func() bool { return strings.Contains(h.screen(), "][") }, "cat to echo ][")
	if h.m.tab != 0 {
		t.Fatalf("] in the editor switched tabs: %d", h.m.tab)
	}
	h.keys("ctrl+a", "]")
	if h.m.tab != 1 {
		t.Fatalf("ctrl+a ] in the editor: tab %d", h.m.tab)
	}
	h.keys("ctrl+a", "[")
	if h.m.tab != 0 || !h.m.ed.live() {
		t.Fatal("ctrl+a [ back to the editor tab")
	}
	h.keys("ctrl+a", "Q")
}

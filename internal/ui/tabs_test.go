package ui

import (
	"k2/internal/config"
	"strings"
	"testing"

	"k2/internal/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func tabbed(t *testing.T, prod ...string) model {
	t.Helper()
	cfg := testEnv(t)
	cfg.Prod = prod
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "ctrl+a", "t")
	m.context = "kind-prod"
	m = press(t, m, "ctrl+a", "t")
	m.context, m.namespace = "", "olm"
	return m
}

func TestNewTabInheritsTargetAndShowsBar(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	if strings.Contains(ansi.Strip(m.View()), " 1 kind-dev ") {
		t.Fatal("tab bar shown with one tab (ui.tabbar auto)")
	}
	h := m.paneArea().H
	m = press(t, m, "ctrl+a", "t")
	if len(m.tabs) != 2 || m.tab != 1 || len(m.panes) != 2 || m.context != "kind-dev" || m.namespace != "shop" || m.status != "tab 2/2" {
		t.Fatalf("tabs %d tab %d panes %d ctx %q status %q", len(m.tabs), m.tab, len(m.panes), m.context, m.status)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.HasPrefix(lines[0], "  1 kind-dev   2 kind-dev ") {
		t.Fatalf("tab bar %q", lines[0])
	}
	if len(lines) != 40 || m.paneArea().H != h-1 {
		t.Fatalf("view %d lines, pane area %d (was %d)", len(lines), m.paneArea().H, h)
	}

	m = press(t, m, "ctrl+a", "v", "ctrl+a", "v", "ctrl+a", "h", "ctrl+a", "h")
	if m.tabs[1].root.Count() != 4 || m.tabs[0].root.Count() != 1 || m.status != "4 panes max" {
		t.Fatalf("tab panes %d / %d status %q", m.tabs[1].root.Count(), m.tabs[0].root.Count(), m.status)
	}
}

func TestSwitchTabs(t *testing.T) {
	m := tabbed(t)
	steps := []struct {
		key  string
		want int
	}{{"ctrl+a ]", 0}, {"ctrl+a ]", 1}, {"ctrl+a [", 0}, {"ctrl+a [", 2}, {"ctrl+a 2", 1}, {"ctrl+a 1", 0}, {"ctrl+a 3", 2}}
	for _, s := range steps {
		m = press(t, m, strings.Fields(s.key)...)
		if m.tab != s.want {
			t.Fatalf("after %s tab %d, want %d", s.key, m.tab, s.want)
		}
	}
	m = press(t, m, "ctrl+a", "5")
	if m.tab != 2 || m.status != "no tab 5" {
		t.Fatalf("tab %d status %q", m.tab, m.status)
	}

	m = press(t, m, "ctrl+a", "1")
	if m.context != "kind-dev" {
		t.Fatalf("tab 1 context %q", m.context)
	}
	m = press(t, m, "ctrl+a", "2")
	if m.context != "kind-prod" {
		t.Fatalf("tab 2 context %q", m.context)
	}
	m = press(t, m, "ctrl+a", "h", "ctrl+up", "ctrl+a", "1", "ctrl+a", "2")
	if m.tabPos(m.logID) != 0 {
		t.Fatal("tab 2 forgot its focused pane")
	}
}

func TestCloseTab(t *testing.T) {
	m := tabbed(t)
	m = press(t, m, "ctrl+a", "2", "ctrl+a", "h")
	m = press(t, m, "ctrl+a", "w")
	if m.asking() != "✗ close tab" || !strings.Contains(ansi.Strip(m.View()), "close tab 2?  y / n") {
		t.Fatalf("confirm %q view:\n%s", m.asking(), ansi.Strip(m.View()))
	}
	m = press(t, m, "n")
	if len(m.tabs) != 3 || m.asking() != "" {
		t.Fatal("n closed the tab")
	}
	m = press(t, m, "ctrl+a", "w", "y")
	if len(m.tabs) != 2 || m.tab != 1 || len(m.panes) != 2 || m.namespace != "olm" {
		t.Fatalf("tabs %d tab %d panes %d ns %q", len(m.tabs), m.tab, len(m.panes), m.namespace)
	}
	m = press(t, m, "ctrl+a", "w", "y", "ctrl+a", "w")
	if len(m.tabs) != 1 || m.status != "last tab" || m.asking() != "" || strings.Contains(ansi.Strip(m.View()), " 1 kind-dev ") {
		t.Fatalf("tabs %d status %q", len(m.tabs), m.status)
	}
}

func TestTabLimitAndBarModes(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	for i := 0; i < 9; i++ {
		m = press(t, m, "ctrl+a", "t")
	}
	if len(m.tabs) != 9 || m.status != "9 tabs max" {
		t.Fatalf("tabs %d status %q", len(m.tabs), m.status)
	}
	cfg := testEnv(t)
	cfg.UI.Tabbar = "always"
	one := newTestModel(t, cfg, nil)
	if !strings.HasPrefix(ansi.Strip(one.View()), "  1 home ") {
		t.Fatal("tabbar always: no bar with one tab")
	}
	cfg.UI.Tabbar = "never"
	two := press(t, newTestModel(t, cfg, nil), "ctrl+a", "t")
	if strings.Contains(ansi.Strip(two.View()), " 2 home ") || two.tabBarRows() != 0 {
		t.Fatal("tabbar never: bar shown")
	}
}

func TestProdTabOnEveryTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		cfg := testEnv(t)
		cfg.Theme = name
		cfg.Prod = []string{"kind-prod"}
		m := newTestModel(t, cfg, nil)
		m.kubeconfig, m.context = "/kc", "kind-dev"
		m = press(t, m, "ctrl+a", "t", "ctrl+a", "v")
		m.context = "kind-prod"
		m = press(t, m, "ctrl+left")
		m = press(t, m, "ctrl+a", "1")
		bar := m.tabBar(120)
		if !strings.Contains(bar, lipgloss.NewStyle().Foreground(th.Danger).Render(" 2 "+skull+" kind-dev ")) {
			t.Fatalf("%s: inactive prod tab not red: %q", name, ansi.Strip(bar))
		}
		if !strings.Contains(bar, selectedItemStyle.Render(" 1 kind-dev ")) {
			t.Fatalf("%s: active tab not highlighted: %q", name, bar)
		}
		m = press(t, m, "ctrl+a", "2")
		bar = m.tabBar(120)
		if !strings.Contains(bar, lipgloss.NewStyle().Foreground(th.Bg).Background(th.Danger).Bold(true).Render(" 2 "+skull+" kind-dev ")) {
			t.Fatalf("%s: active prod tab: %q", name, bar)
		}
	}
}

func TestTabKeysInsideInputs(t *testing.T) {
	m := tabbed(t)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m = press(t, m, "U", "g", "e", "t", " ", "[", "]", " ", "x")
	if m.tab != 2 || m.sh.Value() != "get [] x" {
		t.Fatalf("tab %d line %q", m.tab, m.sh.Value())
	}
	m = press(t, m, "ctrl+w")
	if len(m.tabs) != 3 || m.asking() != "" || m.sh.Value() != "get [] " {
		t.Fatalf("ctrl+w: tabs %d confirm %q line %q", len(m.tabs), m.asking(), m.sh.Value())
	}

	m = press(t, m, "left", "left", "ctrl+a", "1")
	if m.tab != 2 || m.prefixOn || m.sh.Value() != "1get [] " {
		t.Fatalf("ctrl+a in the kubectl line: tab %d prefix %v line %q", m.tab, m.prefixOn, m.sh.Value())
	}

	m.cfg.Keys.Prefix = config.Binding{"ctrl+\\"}
	m = press(t, m, "ctrl+\\", "1")
	if m.tab != 0 {
		t.Fatalf("ctrl+\\ 1 from the kubectl line: tab %d", m.tab)
	}
}

func TestResultsReachHiddenTabs(t *testing.T) {
	m := tabbed(t)
	m = press(t, m, "ctrl+a", "1")
	id := m.logID
	m = press(t, m, "ctrl+a", "2")
	nm, _ := m.Update(kOutMsg{id: id, text: "done in tab 1", code: 0})
	m = nm.(model)
	if m.tab != 1 || m.stage == "kout" {
		t.Fatal("result landed in the visible tab")
	}
	m = press(t, m, "ctrl+a", "1")
	if m.stage != "kout" || !strings.Contains(m.logText, "done in tab 1") {
		t.Fatalf("tab 1 stage %q text %q", m.stage, m.logText)
	}
}

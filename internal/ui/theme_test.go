package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/theme"
)

func TestProdPaneIsRedWithSkullOnEveryTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			cfg := testEnv(t)
			cfg.Theme = name
			cfg.Prod = []string{"prod-eu-1", "kind-prod"}
			m := newTestModel(t, cfg, nil)
			if th.Name != name {
				t.Fatalf("active theme %q, want %q", th.Name, name)
			}
			danger := sgr(th.Danger)

			m.context = "kind-prod"
			m.commit()
			second := freshPane(m.cfg.Keys)
			second.context = "kind-dev"
			m.panes = append(m.panes, second)
			m.tabs[0].root = m.tabs[0].root.SplitDown(m.logID, second.logID)

			out := m.View()
			if got := colorBefore(out, "1 "+skull+" "); got != danger {
				t.Fatalf("prod pane border color %q, want %q", got, danger)
			}

			if got := colorBefore(out[strings.Index(out, "1 "+skull):], "kind-prod"); !strings.Contains(got, danger[2:]) {
				t.Fatalf("prod context color %q, want %q", got, danger)
			}
			if got := colorBefore(out, "2 \x1b[0m"); got != sgr(th.Muted) {
				t.Fatalf("dev pane border color %q, want muted %q", got, sgr(th.Muted))
			}
			if !strings.Contains(ansi.Strip(out), skull+" kind-prod") {
				t.Fatal("sidebar does not show the skull next to the context")
			}
			if m.accent() != th.Danger {
				t.Fatal("focused accent must be danger on prod")
			}

			m.context = "kind-dev"
			out = m.View()
			if strings.Contains(out, skull) {
				t.Fatal("skull shown for a non-prod context")
			}
			if got := colorBefore(out, "1 \x1b[0m"); got != sgr(th.Accent) {
				t.Fatalf("focused dev pane color %q, want accent %q", got, sgr(th.Accent))
			}
		})
	}
	useTheme(theme.Nord())
}

func TestUnknownThemeFallsBackToNord(t *testing.T) {
	cfg := testEnv(t)
	cfg.Theme = "nope"
	newTestModel(t, cfg, nil)
	if th != theme.Nord() {
		t.Fatalf("theme %q", th.Name)
	}
}

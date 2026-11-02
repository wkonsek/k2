package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestProdPatternGetsSafety(t *testing.T) {
	trueColor(t)
	eks := "arn:aws:eks:eu-west-1:1:cluster/prod-x"
	for ctx, hot := range map[string]bool{eks: true, "kind-prod": true, "kind-dev": false} {
		cfg := testEnv(t)
		cfg.UI.ActiveBorder = "thick"
		cfg.Prod = []string{"*prod*"}
		m := newTestModel(t, cfg, nil)
		m.kubeconfig, m.context = "/kc", ctx
		next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
		m = next.(model)
		view := m.View()
		if m.dangerous() != hot || (colorBefore(view, "┃") == sgr(th.Danger)) != hot || strings.Contains(ansi.Strip(view), skull) != hot {
			t.Fatalf("%s: prod safety should be %v", ctx, hot)
		}
	}

	f := deleteCluster()
	m := actionModel(t, f)
	m.cfg.Prod = []string{"*-dev"}
	m = press(t, selectRow(t, m, "web-1"), "ctrl+d")
	if m.dlg == nil || !strings.Contains(ansi.Strip(m.View()), skull+" ✗ delete pod") {
		t.Fatalf("no red popup for a pattern match: %+v", m.dlg)
	}
}

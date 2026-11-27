package ui

import (
	"strings"
	"testing"

	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func TestKindStageSavedAndRestored(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	cfg := testEnv(t)
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = open(t, m, "kind")
	if m.stage != "kind" {
		t.Fatalf("stage %s", m.stage)
	}
	m.save()
	st := state.Load(cfg.StateDir)
	if len(st.Panes) != 1 || st.Panes[0].Stage != "kind" {
		t.Fatalf("saved panes %+v", st.Panes)
	}

	back := newTestModel(t, cfg, f)
	if back.stage != "kind" {
		t.Fatalf("restored stage %s status %q", back.stage, back.status)
	}
	back = settle(t, back, back.Init())
	view := ansi.Strip(back.View())
	if back.stage != "kind" || !strings.Contains(view, "dev") || !strings.Contains(view, "running") {
		t.Fatalf("restored view stage %s:\n%s", back.stage, view)
	}
}

func TestKindLogSavedAsKind(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2"})
	cfg := testEnv(t)
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context = "/kc", "kind-k2"
	m = open(t, m, "kind")
	m.stage = "kindlog"
	m.save()
	if st := state.Load(cfg.StateDir); len(st.Panes) != 1 || st.Panes[0].Stage != "kind" {
		t.Fatalf("saved panes %+v", st.Panes)
	}
}

func TestKindRestoredWithoutKubeconfig(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	cfg := testEnv(t)
	state.Save(cfg.StateDir, state.State{
		Version: state.Version,
		Panes:   []state.Pane{{Stage: "kind"}},
		Tabs:    []state.Tab{{Layout: state.Node{Pane: 0}}},
	})
	m := newTestModel(t, cfg, f)
	if m.kubeconfig != "" || m.stage != "kind" {
		t.Fatalf("kubeconfig %q stage %s", m.kubeconfig, m.stage)
	}
}

func TestKindJoinsViewHistory(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"k2", "dev"})
	m := appModel(t, f)
	m = open(t, m, "pods")
	if m.stage != "workpods" {
		t.Fatalf("pods stage %s", m.stage)
	}
	m = pressSettle(t, m, "~")
	if m.stage != "kind" {
		t.Fatalf("~ stage %s", m.stage)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "workpods" {
		t.Fatalf("back from kind went to %s", m.stage)
	}

	m = pressSettle(t, m, "~")
	m = selectKindTile(t, m, "dev")
	m = open(t, m, "helm")
	if m.stage != "helm" {
		t.Fatalf("helm stage %s", m.stage)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "kind" {
		t.Fatalf("back from helm went to %s", m.stage)
	}
	if got, ok := m.selectedKind(); !ok || got != "dev" {
		t.Fatalf("kind selection after back %q", got)
	}
	m = pressSettle(t, m, "q")
	if m.stage != "workpods" {
		t.Fatalf("second back went to %s", m.stage)
	}
}

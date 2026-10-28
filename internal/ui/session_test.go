package ui

import (
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/forward"
	"k2/internal/layout"
	"k2/internal/shell"
	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func TestHomeHintsFlowAndScroll(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.context = "dev"
	m.boxH = 14
	body := ansi.Strip(m.statusBody(90))
	var shared bool
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "list pods") && strings.Contains(line, "run kubectl") {
			shared = true
		}
	}
	if !shared || !strings.Contains(body, "helm") {
		t.Fatalf("hints did not share a row:\n%s", body)
	}

	m.boxH = 8
	m.vs.homeAt = 0
	top := ansi.Strip(m.statusBody(30))
	m.vs.homeAt = 6
	scrolled := ansi.Strip(m.statusBody(30))
	if top == scrolled || !strings.Contains(top, "list pods") {
		t.Fatalf("scroll did nothing\nTOP\n%s\nSCROLLED\n%s", top, scrolled)
	}
}

func TestSpaceSelectsPods(t *testing.T) {
	m := newTestModel(t, testEnv(t), &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(strings.Join(r.Args, " "), "get pods") {
			return shell.OK(podsJSON)
		}
		return shell.OK("")
	}})
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openPods()
	m = settle(t, next.(model), cmd)
	if m.stage != "workpods" || len(m.content.Items()) < 2 {
		t.Fatalf("pods %q items %d", m.stage, len(m.content.Items()))
	}
	m = press(t, m, " ", " ")
	if len(m.vs.marks) != 2 {
		t.Fatalf("marks %d status %q", len(m.vs.marks), m.status)
	}
	m = press(t, m, "M")
	if m.stage != "mlogs" {
		t.Fatalf("M stage %q status %q", m.stage, m.status)
	}
}

func TestProfileSaveAndOpen(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, testEnv(t), nsCluster())
	m.cfg.StateDir = dir
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.width, m.height = 140, 40
	next, cmd := m.saveProfile("dev")
	m = settle(t, next.(model), cmd)
	if _, err := config.LoadProfileFile(dir, "dev"); err != nil {
		t.Fatal(err)
	}
	m = pressSettle(t, m, ":", "p", "o", "d", "s", "enter")
	m = pressSettle(t, m, ":", "p", "r", "o", "f", "i", "l", "e", " ", "d", "e", "v", "enter")
	if m.tabs[m.tab].root.Count() != 1 || m.stage != "nav" {
		t.Fatalf("one pane profile: panes %d stage %q status %q", m.tabs[m.tab].root.Count(), m.stage, m.status)
	}

	if err := config.SaveProfile(dir, "two", config.Profile{
		Split: "right",
		Panes: []config.ProfilePane{
			{Context: "kind-dev", Namespace: "shop", View: "pods"},
			{Context: "kind-dev", Namespace: "pay", View: "pulse"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	next, cmd = m.loadProfile("two")
	m = settle(t, next.(model), cmd)
	if m.tabs[m.tab].root.Count() != 2 || m.stage != "pulse" || m.namespace != "pay" {
		t.Fatalf("profile: panes %d stage %q ns %q status %q", m.tabs[m.tab].root.Count(), m.stage, m.namespace, m.status)
	}
	if m.panes[0].context != "kind-dev" || m.panes[0].namespace != "shop" {
		t.Fatalf("first pane ctx %q ns %q stage %q", m.panes[0].context, m.panes[0].namespace, m.panes[0].stage)
	}
	if m.tabs[m.tab].root.Dir != layout.Row {
		t.Fatalf("flat profile split %v", m.tabs[m.tab].root.Dir)
	}
}

func TestProfileKeepsTheSplitTree(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, testEnv(t), nsCluster())
	m.cfg.StateDir = dir
	m.cfg.MaxPanes = 8
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.width, m.height = 200, 60
	a := m.logID
	b, c, d := freshPane(m.cfg.Keys), freshPane(m.cfg.Keys), freshPane(m.cfg.Keys)
	for _, p := range []*paneSnap{&b, &c, &d} {
		p.kubeconfig, p.context, p.namespace = "/kc", "kind-dev", "shop"
		p.stage = "nav"
	}
	m.panes = []paneSnap{m.snap(), b, c, d}
	m.focus = 0
	m.tabs[m.tab].root = &layout.Node{Dir: layout.Col, Kids: []*layout.Node{
		layout.New(a),
		{Dir: layout.Row, Kids: []*layout.Node{
			layout.New(b.logID),
			{Dir: layout.Col, Kids: []*layout.Node{layout.New(c.logID), layout.New(d.logID)}},
		}},
	}}
	m.tabs[m.tab].focus = a
	if got := shape(m, m.tabs[m.tab].root); got != "col(1,row(2,col(3,4)))" {
		t.Fatalf("built %s", got)
	}
	next, cmd := m.saveProfile("quad")
	m = settle(t, next.(model), cmd)
	prof, err := config.LoadProfileFile(dir, "quad")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Split != "down" || prof.Layout.Split != "down" || len(prof.Layout.Children) != 2 || prof.Layout.Children[1].Split != "right" || prof.Layout.Children[1].Children[1].Split != "down" {
		t.Fatalf("saved split %q layout %+v", prof.Split, prof.Layout)
	}
	next, cmd = m.loadProfile("quad")
	m = settle(t, next.(model), cmd)
	if got := shape(m, m.tabs[m.tab].root); got != "col(1,row(2,col(3,4)))" {
		t.Fatalf("restored %s panes %d", got, m.tabs[m.tab].root.Count())
	}
}

func TestProfileCommandCompletes(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, ":", "p", "r", "tab")
	if m.cmd.Value() != "profile" && !strings.Contains(m.cmdHint, "profile") {
		t.Fatalf("value %q hint %q", m.cmd.Value(), m.cmdHint)
	}
	m.cmd.SetValue("prof")
	m = press(t, m, "tab")
	if m.cmd.Value() != "profile" {
		t.Fatalf("prof: %q hint %q", m.cmd.Value(), m.cmdHint)
	}
	m = press(t, m, " ", "tab")
	if !strings.Contains(m.cmdHint, "no profiles") {
		t.Fatalf("args %q hint %q", m.cmd.Value(), m.cmdHint)
	}
}

func TestProfileTabCyclesFiles(t *testing.T) {
	cfg := testEnv(t)
	for _, name := range []string{"dev", "lab"} {
		if err := config.SaveProfile(cfg.StateDir, name, config.Profile{Panes: []config.ProfilePane{{View: "home"}}}); err != nil {
			t.Fatal(err)
		}
	}
	m := newTestModel(t, cfg, nil)
	m = press(t, m, ":", "p", "r", "o", "f", "i", "l", "e", " ", "tab")
	if !strings.Contains(m.cmdHint, "dev") || !strings.Contains(m.cmdHint, "lab") {
		t.Fatalf("hint %q value %q", m.cmdHint, m.cmd.Value())
	}
	m = press(t, m, "tab")
	first := m.cmd.Value()
	m = press(t, m, "tab")
	second := m.cmd.Value()
	if first == second || first != "profile dev" && first != "profile lab" || second != "profile dev" && second != "profile lab" {
		t.Fatalf("cycle %q -> %q", first, second)
	}
}

func TestFreshDropsTabsAndPanes(t *testing.T) {
	cfg := testEnv(t)
	st := state.State{
		Version: state.Version, Kubeconfig: "/kc", Context: "kind-dev", Namespace: "shop",
		Panes: []state.Pane{
			{Stage: "nav", Kubeconfig: "/kc", Context: "kind-dev", Namespace: "shop"},
			{Stage: "workpods", Kubeconfig: "/kc", Context: "kind-dev", Namespace: "shop"},
		},
		Tabs: []state.Tab{{
			Layout: state.Node{Split: "row", Kids: []state.Node{{Pane: 0}, {Pane: 1}}},
			Focus:  1,
		}},
		Forwards: []forward.Forward{{Local: "8080", Remote: "80", Namespace: "shop", Pod: "web"}},
	}
	state.Save(cfg.StateDir, st)
	kept := newTestModel(t, cfg, nil)
	if len(kept.panes) != 2 || len(kept.tabs) != 1 {
		t.Fatalf("restore panes %d tabs %d", len(kept.panes), len(kept.tabs))
	}
	fresh := initialModel(Options{Runner: kept.run, Config: &cfg, Fresh: true})
	if len(fresh.panes) != 1 || len(fresh.tabs) != 1 || fresh.stage != "nav" || fresh.tabs[0].root.Count() != 1 {
		t.Fatalf("flag panes %d tabs %d stage %q", len(fresh.panes), len(fresh.tabs), fresh.stage)
	}
	if fresh.context != "kind-dev" || fresh.namespace != "shop" || len(fresh.forwards) != 1 {
		t.Fatalf("flag lost cluster ctx %q ns %q forwards %d", fresh.context, fresh.namespace, len(fresh.forwards))
	}

	m := kept
	m = press(t, m, ":", "f", "r", "e", "tab")
	if m.cmd.Value() != "fresh" {
		t.Fatalf("complete %q hint %q", m.cmd.Value(), m.cmdHint)
	}
	next, cmd := m.runCommand("fresh")
	m = settle(t, next.(model), cmd)
	if len(m.tabs) != 1 || m.tabs[0].root.Count() != 1 || m.stage != "nav" || m.context != "kind-dev" || len(m.forwards) != 1 {
		t.Fatalf("command panes %d stage %q ctx %q forwards %d", m.tabs[0].root.Count(), m.stage, m.context, len(m.forwards))
	}
	again := state.Load(cfg.StateDir)
	if len(again.Panes) != 1 || len(again.Tabs) != 1 || again.Context != "kind-dev" {
		t.Fatalf("saved panes %d tabs %d", len(again.Panes), len(again.Tabs))
	}
}

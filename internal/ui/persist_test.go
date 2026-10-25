package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/layout"
	"k2/internal/state"
)

func shape(m model, n *layout.Node) string {
	if n.Dir == layout.Leaf {
		return string(rune('1' + m.tabPos(n.Pane)))
	}
	var parts []string
	for _, k := range n.Kids {
		parts = append(parts, shape(m, k))
	}
	name := map[layout.Dir]string{layout.Row: "row", layout.Col: "col"}[n.Dir]
	return name + "(" + strings.Join(parts, ",") + ")"
}

func TestTabsAndLayoutsSurviveRestart(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "ctrl+a", "v", "ctrl+a", "h")
	m = press(t, m, "ctrl+a", "t")
	m.context = "kind-prod"
	m = press(t, m, "ctrl+a", "h", "f", "ctrl+up")
	m = press(t, m, "ctrl+a", "1", "ctrl+left")
	m = press(t, m, "ctrl+a", "2")
	m.forwards = []forward.Forward{{Namespace: "shop", Pod: "web", Local: "8080", Remote: "80"}}
	m.save()

	again := newTestModel(t, cfg, nil)
	if len(again.tabs) != 2 || again.tab != 1 || len(again.panes) != 5 {
		t.Fatalf("tabs %d active %d panes %d", len(again.tabs), again.tab, len(again.panes))
	}
	if got := shape(again, again.tabs[0].root); got != "row(1,col(2,3))" {
		t.Fatalf("tab 1 %s", got)
	}
	if got := shape(again, again.tabs[1].root); got != "col(1,2)" {
		t.Fatalf("tab 2 %s", got)
	}
	if again.tabPos(again.logID) != 0 || again.context != "kind-prod" {
		t.Fatalf("focus %d ctx %q", again.tabPos(again.logID), again.context)
	}
	again = press(t, again, "ctrl+down")
	if again.stage != "fwds" {
		t.Fatalf("tab 2 pane 2 stage %q", again.stage)
	}
	again = press(t, again, "ctrl+a", "1")
	if again.tabPos(again.logID) != 0 || again.context != "kind-dev" {
		t.Fatalf("tab 1 focus %d ctx %q", again.tabPos(again.logID), again.context)
	}
}

func TestOldStateOpensAsOneStackedTab(t *testing.T) {
	cfg := testEnv(t)
	b, err := os.ReadFile(filepath.Join("..", "state", "testdata", "state-old.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(cfg.StateDir, 0o755)
	_ = os.WriteFile(state.Path(cfg.StateDir), b, 0o644)
	m := newTestModel(t, cfg, nil)
	if len(m.tabs) != 1 || len(m.panes) != 3 || shape(m, m.tabs[0].root) != "col(1,2,3)" {
		t.Fatalf("tabs %d panes %d", len(m.tabs), len(m.panes))
	}
	if m.tabPos(m.logID) != 2 || m.context != "kind-prod" || m.namespace != "olm" || len(m.forwards) != 1 {
		t.Fatalf("focus %d ctx %q ns %q forwards %d", m.tabPos(m.logID), m.context, m.namespace, len(m.forwards))
	}
	if strings.Contains(m.View(), " 1 kind-dev ") {
		t.Fatal("a single tab must not show the tab bar")
	}
}

func TestBrokenLayoutFallsBack(t *testing.T) {
	cfg := testEnv(t)
	cfg.MaxPanes = 2
	st := state.State{
		Kubeconfig: "/kc", Context: "kind-dev",
		Panes: []state.Pane{{Stage: "nav"}, {Stage: "nav"}, {Stage: "nav"}},
		Tabs:  []state.Tab{{Layout: state.Node{Split: "row", Kids: []state.Node{{Pane: 0}, {Pane: 0}}}}},
	}
	state.Save(cfg.StateDir, st)
	m := newTestModel(t, cfg, nil)
	if len(m.tabs) != 1 || len(m.panes) != 2 || shape(m, m.tabs[0].root) != "col(1,2)" {
		t.Fatalf("tabs %d panes %d %s", len(m.tabs), len(m.panes), shape(m, m.tabs[0].root))
	}
}

func TestSternSurvivesRestart(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	q, err := kube.ParseLogQuery(`app=kafka -c broker --since=5m --tail=20 --all=false`, true)
	if err != nil {
		t.Fatal(err)
	}
	m.stage = "mlogs"
	m.vs.ml = &mlogView{q: q, title: "statefulsets/kafka", from: "res", cur: kube.LogCursor{}, colors: map[string]color{}}
	m.save()

	again := newTestModel(t, cfg, nil)
	if again.stage != "mlogs" || again.namespace != "shop" || again.vs.ml == nil {
		t.Fatalf("stage %s ns %s ml %v", again.stage, again.namespace, again.vs.ml)
	}
	got := again.vs.ml.q
	if got.Selector != "app=kafka" || got.Container != "broker" || got.All || got.Since != 5*time.Minute || got.Tail != 20 || again.vs.ml.title != "statefulsets/kafka" {
		t.Fatalf("query %+v title %q", got, again.vs.ml.title)
	}
	if again.vs.ml.from != "nav" {
		t.Fatalf("back target %q", again.vs.ml.from)
	}
	if len(again.resumeStern()) != 1 || !again.rowBusy["ml"+again.logID] {
		t.Fatal("stern was not polled on startup")
	}

	m = newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.stage = "mlogs"
	m.vs.ml = &mlogView{q: kube.LogQuery{Regex: regexp.MustCompile(`^kafka-[0-9]+$`), All: true, Tail: -1}, title: "pods like kafka-0", cur: kube.LogCursor{}, colors: map[string]color{}}
	m.save()
	again = newTestModel(t, cfg, nil)
	if again.stage != "mlogs" || again.vs.ml == nil || again.vs.ml.q.Regex == nil || again.vs.ml.q.Regex.String() != `^kafka-[0-9]+$` || !again.vs.ml.q.All {
		t.Fatalf("regex %+v", again.vs.ml)
	}
}

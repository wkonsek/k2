package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"k2/internal/config"
	"k2/internal/shell"
	"k2/internal/state"
)

func start(t *testing.T, m model) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
}

func send(t *testing.T, tm *teatest.TestModel, keys ...string) {
	t.Helper()
	for _, k := range keys {
		tm.Send(key(t, k))
	}
}

func finish(t *testing.T, tm *teatest.TestModel) model {
	t.Helper()
	send(t, tm, "ctrl+c")
	return tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(model)
}

func waitFor(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(text)) },
		teatest.WithDuration(3*time.Second))
}

func clusterFake() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "api-resources" {
			return shell.OK("pods  po  v1  true  Pod\nsubscriptions  sub,subs  operators.coreos.com/v1alpha1  true  Subscription\nsecrets    v1  true  Secret\n")
		}
		return shell.Fail(1, "not in this test")
	}}
}

func withCluster(t *testing.T, cfg config.Config) {
	t.Helper()
	kc := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	state.Save(cfg.StateDir, state.State{Kubeconfig: kc, Context: "kind-dev", Namespace: "shop"})
}

func TestCommandLineTabCompletionCycles(t *testing.T) {
	cfg := testEnv(t)
	tm := start(t, newTestModel(t, cfg, nil))

	send(t, tm, ":", "f", "tab")
	waitFor(t, tm, "fav  forward  forwards")
	send(t, tm, "tab")
	waitFor(t, tm, "[fav]")
	send(t, tm, "tab")
	waitFor(t, tm, "[forward]")
	send(t, tm, "tab")
	waitFor(t, tm, "[forwards]")
	m := finish(t, tm)
	if !m.cmdOn || m.cmd.Value() != "forwards" || m.cmdCycleAt != 2 {
		t.Fatalf("cmd=%q on=%v at=%d", m.cmd.Value(), m.cmdOn, m.cmdCycleAt)
	}
}

func TestCommandLineCompletesClusterResourcesAndAliases(t *testing.T) {
	cfg := testEnv(t)
	cfg.Aliases = map[string]string{"subx": "subscriptions"}
	withCluster(t, cfg)
	tm := start(t, newTestModel(t, cfg, clusterFake()))
	send(t, tm, ":")
	waitFor(t, tm, "tab completes cluster resources")
	send(t, tm, "s", "u", "tab")
	waitFor(t, tm, "sub  subs  subscriptions  subx")
	send(t, tm, "tab", "tab", "tab")
	m := finish(t, tm)
	if m.cmd.Value() != "subscriptions" {
		t.Fatalf("cycled to %q", m.cmd.Value())
	}

	m.cmd.SetValue("subx")
	next, cmd := m.updateCmd(key(t, "enter"))
	if st := settle(t, next.(model), cmd).status; !strings.Contains(st, "not in this test") {
		t.Fatalf("alias not resolved, status %q", st)
	}
}

func TestLowerQGoesBackUpperQQuits(t *testing.T) {
	cfg := testEnv(t)
	tm := start(t, newTestModel(t, cfg, nil))
	send(t, tm, "q")
	send(t, tm, "f")
	waitFor(t, tm, "no forwards")
	send(t, tm, "q")
	send(t, tm, "f")
	send(t, tm, "Q")
	m := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(model)
	if !m.quitting || m.stage != "fwds" {
		t.Fatalf("quitting=%v stage=%q", m.quitting, m.stage)
	}
	if got := press(t, m, "q").stage; got != "nav" {
		t.Fatalf("q from forwards went to %q", got)
	}
}

func TestConfirmClosePaneYesNo(t *testing.T) {
	cfg := testEnv(t)
	tm := start(t, newTestModel(t, cfg, nil))
	send(t, tm, "ctrl+a", "h", "ctrl+a", "x")
	waitFor(t, tm, "close pane?  y / n")
	send(t, tm, "x")
	send(t, tm, "n")
	send(t, tm, "ctrl+a", "x", "y")
	send(t, tm, "ctrl+a", "x")
	waitFor(t, tm, "last pane")
	m := finish(t, tm)
	if len(m.panes) != 1 || m.asking() != "" {
		t.Fatalf("panes=%d confirm=%q", len(m.panes), m.asking())
	}

	m = press(t, newTestModel(t, cfg, nil), "ctrl+a", "h", "ctrl+a", "x", "n")
	if len(m.panes) != 2 || m.asking() != "" {
		t.Fatalf("n: panes=%d confirm=%q", len(m.panes), m.asking())
	}
}

func TestPaneSwitching(t *testing.T) {
	cfg := testEnv(t)
	tm := start(t, newTestModel(t, cfg, nil))
	send(t, tm, "ctrl+a", "h", "ctrl+a", "h", "ctrl+a", "h", "ctrl+a", "h")
	waitFor(t, tm, "4 home")
	m := finish(t, tm)
	if len(m.panes) != 4 || m.focus != 3 || m.status != "4 panes max" {
		t.Fatalf("panes=%d focus=%d", len(m.panes), m.focus)
	}
	steps := []struct {
		key   string
		focus int
	}{

		{"ctrl+down", 3}, {"ctrl+a o", 0}, {"ctrl+down", 1}, {"ctrl+right", 1}, {"ctrl+left", 1},
		{"ctrl+down", 2}, {"ctrl+up", 1}, {"ctrl+up", 0}, {"ctrl+up", 0}, {"ctrl+a o", 1},
	}
	for _, s := range steps {
		m = press(t, m, strings.Fields(s.key)...)
		if m.focus != s.focus {
			t.Fatalf("after %s focus=%d want %d", s.key, m.focus, s.focus)
		}
	}
}

func TestEachPaneKeepsItsOwnTarget(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/a", "kind-dev", "shop"
	m = press(t, m, "ctrl+a", "h")
	m.kubeconfig, m.context, m.namespace = "/b", "kind-prod", "olm"
	m = press(t, m, "ctrl+up")
	if m.kubeconfig != "/a" || m.context != "kind-dev" || m.namespace != "shop" {
		t.Fatalf("pane 1 target lost: %s %s %s", m.kubeconfig, m.context, m.namespace)
	}
	m = press(t, m, "ctrl+a", "o")
	if m.context != "kind-prod" || m.namespace != "olm" {
		t.Fatalf("pane 2 target lost: %s %s", m.context, m.namespace)
	}
}

func TestPanesAndForwardsSurviveRestart(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", ""
	m = press(t, m, "ctrl+a", "h", "f")
	m.save()
	again := newTestModel(t, cfg, nil)
	if len(again.panes) != 2 || again.focus != 1 || again.kubeconfig != "/kc" || again.context != "kind-dev" {
		t.Fatalf("restored %d panes focus %d %s %s", len(again.panes), again.focus, again.kubeconfig, again.context)
	}
}

func TestConfigWarningsInStatusLine(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil, `config: keys.delete: unknown key "hyper+d" (using ctrl+d)`, "config: other")
	if m.status != `config: keys.delete: unknown key "hyper+d" (using ctrl+d)  (+1 more)` {
		t.Fatalf("status %q", m.status)
	}
	if !strings.Contains(m.View(), "hyper+d") {
		t.Fatal("warning not rendered")
	}
}

func TestRemappedKeys(t *testing.T) {
	cfg := testEnv(t)
	cfg.Keys.Quit = config.Binding{"ctrl+c"}
	cfg.Keys.Forwards = config.Binding{"F"}
	cfg.Keys.Fullscreen = config.Binding{"z"}
	m := newTestModel(t, cfg, nil)
	next, cmd := m.Update(key(t, "Q"))
	if next.(model).quitting || cmd != nil && isQuit(cmd) {
		t.Fatal("Q still quits after remap")
	}
	if got := press(t, m, "F").stage; got != "fwds" {
		t.Fatalf("remapped forwards key opened %q", got)
	}
	if !press(t, m, "z").zoom {
		t.Fatal("remapped fullscreen key")
	}
	if !strings.Contains(m.legend(), "F") || !strings.Contains(m.legend(), "^c") {
		t.Fatal("legend does not show remapped keys")
	}
}

func isQuit(cmd tea.Cmd) bool {
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestKubectlViewBuildsArguments(t *testing.T) {
	cfg := testEnv(t)
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result { return shell.OK("NAME\nweb\n") }}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "U")
	if m.stage != "kcmd" {
		t.Fatalf("U opened %q", m.stage)
	}
	m = press(t, m, "g", "e", "t", " ", "p", "o", "ctrl+o")
	next, cmd := m.Update(key(t, "enter"))
	m = next.(model)
	if !m.load.active() || !strings.Contains(m.View(), "running kubectl") {
		t.Fatal("no spinner while kubectl runs")
	}
	m = settle(t, m, cmd)
	got := f.Calls()[len(f.Calls())-1].Line()
	if got != "kubectl get po --kubeconfig /kc --context kind-dev -n shop -o wide" {
		t.Fatalf("ran %q", got)
	}
	if m.load.active() || m.stage != "kout" || !strings.HasPrefix(m.status, "exit 0 · done · ") || !strings.Contains(m.logText, "web") {
		t.Fatalf("stage=%q status=%q", m.stage, m.status)
	}
	if m = press(t, m, "q"); m.stage != "kcmd" {
		t.Fatalf("q from output returned to %q", m.stage)
	}

	cfg.KubectlOutput = "yaml"
	if newTestModel(t, cfg, f).shFmt != 2 {
		t.Fatal("kubectl_output not applied")
	}
}

func TestSecretDecodeView(t *testing.T) {
	cfg := testEnv(t)
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.OK(`{"data":{"password":"aHVudGVyMg==","blob":"AAEC"}}`)
	}}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace, m.kind, m.stage = "/kc", "c", "shop", "secrets", "res"
	m.content.SetItems([]list.Item{item{id: "db", title: "db"}})
	m = pressSettle(t, m, "x")
	if m.stage != "view" || !strings.Contains(m.logText, "  hunter2") || !strings.Contains(m.logText, "[binary 3 bytes]") {
		t.Fatalf("stage %q text %q", m.stage, m.logText)
	}
	if m = press(t, m, "esc"); m.stage != "res" {
		t.Fatalf("esc returned to %q, want the list", m.stage)
	}
}

func TestKubectlLineTakesEveryPrintableKey(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m = press(t, m, "ctrl+a", "h", "U")
	if m.stage != "kcmd" {
		t.Fatalf("U opened %q", m.stage)
	}
	typed := "get pods -l app=QFO,q=fnKCU:x"
	for _, r := range typed {
		m = press(t, m, string(r))
	}
	if m.sh.Value() != typed {
		t.Fatalf("typed %q, got %q", typed, m.sh.Value())
	}
	if m.quitting || m.zoom || m.focus != 1 || m.stage != "kcmd" || m.cmdOn {
		t.Fatalf("a shortcut fired: quit=%v zoom=%v focus=%d stage=%q cmd=%v", m.quitting, m.zoom, m.focus, m.stage, m.cmdOn)
	}

	m = press(t, m, "ctrl+j", "q", "O")
	if m.jq.Value() != "qO" || m.sh.Value() != typed {
		t.Fatalf("jq %q", m.jq.Value())
	}
	m = press(t, m, "esc")
	if m.jqOn || m.stage != "kcmd" {
		t.Fatal("esc should leave the jq filter first")
	}

	if m = press(t, m, "ctrl+up"); m.focus != 0 {
		t.Fatal("ctrl+up must still switch panes")
	}
	m = press(t, m, "ctrl+down", "esc")
	if m.stage != "nav" {
		t.Fatalf("esc left to %q", m.stage)
	}
	next, cmd := m.Update(key(t, "Q"))
	if cmd == nil || !isQuit(cmd) || !next.(model).quitting {
		t.Fatal("Q must quit again once the input is gone")
	}
}

func TestCommandLineTakesQ(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m = press(t, m, ":", "Q", "q", "O", "F")
	if !m.cmdOn || m.cmd.Value() != "QqOF" || m.quitting || m.zoom {
		t.Fatalf("cmd %q on=%v", m.cmd.Value(), m.cmdOn)
	}
}

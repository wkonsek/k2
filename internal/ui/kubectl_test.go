package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/shell"
	"k2/internal/state"
)

func TestExpandKubectlLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	secret := filepath.Join(home, "secrets", "master.yml")
	kube := filepath.Join(home, ".kube", "config")
	cases := []struct{ in, want string }{
		{"get pods", "get pods"},
		{"apply -f ~/secrets/master.yml", "apply -f " + secret},
		{"apply -f=~/secrets/master.yml", "apply -f=" + secret},
		{"apply --filename=~/secrets/master.yml", "apply --filename=" + secret},
		{"get pods --kubeconfig=~/.kube/config", "get pods --kubeconfig=" + kube},
		{"apply -f ~", "apply -f " + filepath.Clean(home)},
		{"apply -f=~", "apply -f=" + filepath.Clean(home)},
		{"get pods -l app=web", "get pods -l app=web"},
		{"get pods ~foo", "get pods ~foo"},
		{"label pod hello app=~prod", "label pod hello app=~prod"},
		{"apply -f '~/secrets/master.yml'", "apply -f '~/secrets/master.yml'"},
		{`apply -f "~/secrets/master.yml"`, `apply -f "~/secrets/master.yml"`},
		{`apply --filename="~/secrets/master.yml"`, `apply --filename="~/secrets/master.yml"`},
		{"apply -f ~/secrets/master.yml -n shop", "apply -f " + secret + " -n shop"},
	}
	for _, c := range cases {
		if got := expandKubectlLine(c.in); got != c.want {
			t.Errorf("expandKubectlLine(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestRememberShellLine(t *testing.T) {
	var hist []string
	hist = rememberShellLine(hist, "  get pods  ")
	hist = rememberShellLine(hist, "get pods")
	if len(hist) != 1 || hist[0] != "get pods" {
		t.Fatalf("dedupe: %q", hist)
	}
	hist = rememberShellLine(hist, "")
	hist = rememberShellLine(hist, "   ")
	if len(hist) != 1 {
		t.Fatalf("empty pushed: %q", hist)
	}
	hist = rememberShellLine(hist, "get ns")
	hist = rememberShellLine(hist, "get pods")
	if got := strings.Join(hist, ","); got != "get pods,get ns,get pods" {
		t.Fatalf("order %q", got)
	}
	hist = nil
	for i := 0; i < shHistCap+10; i++ {
		hist = rememberShellLine(hist, fmt.Sprintf("cmd-%d", i))
	}
	if len(hist) != shHistCap {
		t.Fatalf("cap %d, want %d", len(hist), shHistCap)
	}
	if hist[0] != "cmd-10" || hist[shHistCap-1] != "cmd-109" {
		t.Fatalf("window %s..%s", hist[0], hist[shHistCap-1])
	}
}

func runShell(t *testing.T, m model, line string) model {
	t.Helper()
	m.sh.SetValue(line)
	m.sh.CursorEnd()
	return pressSettle(t, m, "enter")
}

func TestKubectlHistoryUpDown(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result { return shell.OK("ok\n") }}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "U")
	if m.stage != "kcmd" {
		t.Fatalf("U opened %q", m.stage)
	}

	m = runShell(t, m, "get ns")
	if m.stage != "kout" {
		t.Fatalf("after get ns: %s", m.stage)
	}
	m = pressSettle(t, m, "q")
	m = runShell(t, m, "get pods")
	m = pressSettle(t, m, "q")
	m = runShell(t, m, "get pods")
	m = pressSettle(t, m, "q")
	if got := strings.Join(m.shHist, ","); got != "get ns,get pods" {
		t.Fatalf("hist %q", got)
	}

	m.sh.SetValue("get deploy")
	m.sh.CursorEnd()
	m = press(t, m, "up")
	if m.sh.Value() != "get pods" || m.sh.Position() != len("get pods") {
		t.Fatalf("up: %q pos %d", m.sh.Value(), m.sh.Position())
	}
	m = press(t, m, "up")
	if m.sh.Value() != "get ns" || m.sh.Position() != len("get ns") {
		t.Fatalf("up again: %q pos %d", m.sh.Value(), m.sh.Position())
	}
	m = press(t, m, "up")
	if m.sh.Value() != "get ns" {
		t.Fatalf("up past oldest: %q", m.sh.Value())
	}
	m = press(t, m, "down")
	if m.sh.Value() != "get pods" {
		t.Fatalf("down: %q", m.sh.Value())
	}
	m = press(t, m, "down")
	if m.sh.Value() != "get deploy" || m.sh.Position() != len("get deploy") {
		t.Fatalf("draft: %q pos %d", m.sh.Value(), m.sh.Position())
	}
	m = press(t, m, "down")
	if m.sh.Value() != "get deploy" {
		t.Fatalf("down past draft: %q", m.sh.Value())
	}

	m = press(t, m, "j", "k")
	if m.sh.Value() != "get deployjk" {
		t.Fatalf("j/k typed: %q", m.sh.Value())
	}

	m.sh.SetValue("apply -f ~/secrets/master.yml")
	m = pressSettle(t, m, "enter")
	m = pressSettle(t, m, "q")
	if m.shHist[len(m.shHist)-1] != "apply -f ~/secrets/master.yml" {
		t.Fatalf("history kept tilde: %q", m.shHist)
	}
	got := f.Calls()[len(f.Calls())-1].Line()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := "kubectl apply -f " + filepath.Join(home, "secrets", "master.yml") + " --kubeconfig /kc --context kind-dev -n shop"
	if got != want {
		t.Fatalf("ran %q\nwant %q", got, want)
	}
}

func TestKubectlHistorySkipsJqAndEmpty(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result { return shell.OK("ok\n") }}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "U")
	m = runShell(t, m, "get ns")
	m = pressSettle(t, m, "q")
	m.sh.SetValue("draft")
	m = press(t, m, "ctrl+j", "up")
	if !m.jqOn || m.sh.Value() != "draft" || m.shHistAt != 0 {
		t.Fatalf("up in jq: jqOn=%v line=%q at=%d", m.jqOn, m.sh.Value(), m.shHistAt)
	}
	m = press(t, m, "ctrl+j")
	m.sh.SetValue("")
	m = pressSettle(t, m, "enter")
	if m.stage != "kcmd" || strings.Join(m.shHist, ",") != "get ns" {
		t.Fatalf("empty submit: stage %s hist %q", m.stage, m.shHist)
	}
}

func TestKubectlHistorySurvivesRestart(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result { return shell.OK("ok\n") }}
	cfg := testEnv(t)
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "U")
	m = pressSettle(t, runShell(t, m, "get ns"), "q")
	m = pressSettle(t, runShell(t, m, "get pods -A"), "q")
	m = pressSettle(t, runShell(t, m, "get pods -A"), "q")
	if got := strings.Join(state.LoadKubectlHistory(cfg.StateDir), ","); got != "get ns,get pods -A" {
		t.Fatalf("on disk %q", got)
	}

	back := newTestModel(t, cfg, f)
	back.kubeconfig, back.context, back.namespace = "/kc", "kind-dev", "shop"
	back = press(t, back, "U")
	back.sh.SetValue("draft")
	back.sh.CursorEnd()
	back = press(t, back, "up")
	if back.sh.Value() != "get pods -A" {
		t.Fatalf("up after restart %q", back.sh.Value())
	}
	back = press(t, back, "up")
	if back.sh.Value() != "get ns" {
		t.Fatalf("up again %q", back.sh.Value())
	}
	back = press(t, back, "down", "down")
	if back.sh.Value() != "draft" {
		t.Fatalf("draft %q", back.sh.Value())
	}
	back = pressSettle(t, runShell(t, back, "get svc"), "q")
	if got := strings.Join(state.LoadKubectlHistory(cfg.StateDir), ","); got != "get ns,get pods -A,get svc" {
		t.Fatalf("appended %q", got)
	}
}

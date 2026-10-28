package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const deployJSON = `{"kind":"Deployment","metadata":{"name":"web","namespace":"shop"},"spec":{"template":{"spec":{
 "initContainers":[{"name":"migrate","image":"acme/migrate:1.0"}],
 "containers":[{"name":"app","image":"acme/app:1.0"},{"name":"side","image":"envoy:1.30"}]}}}}`

func imageCluster(setErr string) *shell.Fake {
	base := actionCluster()
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "get deploy/web") && strings.Contains(line, "-o json"):
			return shell.OK(deployJSON)
		case len(r.Args) > 1 && r.Args[0] == "set" && r.Args[1] == "image":
			if setErr != "" {
				return shell.Fail(1, setErr)
			}
			return shell.OK("deployment.apps/web image updated")
		}
		return base.Handler(r)
	}}
}

func TestPodSpecPaths(t *testing.T) {
	pod := map[string]any{"kind": "Pod", "spec": map[string]any{"containers": []any{map[string]any{"name": "a", "image": "x:1"}}}}
	cron := map[string]any{"kind": "CronJob", "spec": map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
		"containers": []any{map[string]any{"name": "c", "image": "y:2"}}, "initContainers": []any{map[string]any{"name": "i", "image": "z:3"}}}}}}}}
	if got := containerImages(pod); len(got) != 1 || got[0].name != "a" || got[0].old != "x:1" {
		t.Fatalf("pod %+v", got)
	}
	got := containerImages(cron)
	if len(got) != 2 || got[0].name != "c" || !got[1].init || got[1].old != "z:3" {
		t.Fatalf("cronjob %+v", got)
	}
}

func TestSetImage(t *testing.T) {
	f := imageCluster("")
	m := open(t, actionModel(t, f), "deploy")
	m = pressSettle(t, m, "i")
	if m.dlg == nil || m.dlg.act != "setimage" || len(m.dlg.images) != 3 {
		t.Fatalf("popup %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"set image deploy/web", "app", "acme/app:1.0", "side", "envoy:1.30", "migrate", "init", "acme/migrate:1.0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup lacks %q:\n%s", want, view)
		}
	}

	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.msg != "nothing changed" {
		t.Fatalf("unchanged %+v", m.dlg)
	}

	m = pressSettle(t, m, "down", "down")
	if m.dlg.idx != 2 {
		t.Fatalf("idx %d", m.dlg.idx)
	}
	for range "1.0" {
		m = pressSettle(t, m, "backspace")
	}
	m = typeText(t, m, "2.0j")
	m = pressSettle(t, m, "backspace")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.ask == "" {
		t.Fatalf("no confirmation %+v", m.dlg)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{"deploy shop/web", "context kind-dev", "migrate (init): acme/migrate:1.0 → acme/migrate:2.0", "default no"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "enter")
	if len(calls(f, "set")) != 0 || m.dlg == nil {
		t.Fatal("enter ran the change")
	}
	m = pressSettle(t, m, "y")
	s := calls(f, "set")
	if len(s) != 1 || !strings.HasPrefix(s[0], "set image deploy/web -n shop migrate=acme/migrate:2.0 --kubeconfig /kc --context kind-dev") {
		t.Fatalf("set %v", s)
	}
	if m.dlg != nil || !strings.Contains(m.toast.text, "set image deploy/web") || m.load.active() {
		t.Fatalf("after: dlg %v toast %q spinner %v", m.dlg != nil, m.toast.text, m.load.active())
	}
}

func TestSetImageCancel(t *testing.T) {
	f := imageCluster("")
	m := open(t, actionModel(t, f), "deploy")
	m = pressSettle(t, m, "i", "esc")
	if m.dlg != nil {
		t.Fatal("esc did not close")
	}
	m = pressSettle(t, m, "i", "tab")
	m = typeText(t, m, "-x")
	m = pressSettle(t, m, "enter", "n")
	if m.dlg == nil || m.dlg.ask != "" || len(m.dlg.details) != 0 || len(m.dlg.images) != 3 {
		t.Fatalf("n should go back to the fields: %+v", m.dlg)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg != nil || len(calls(f, "set")) != 0 {
		t.Fatal("cancel ran something")
	}
}

func TestSetImageError(t *testing.T) {
	f := imageCluster(`error: unable to find container named "nope"`)
	m := open(t, actionModel(t, f), "deploy")
	m = pressSettle(t, m, "i")
	m = typeText(t, m, "-rc")
	m = pressSettle(t, m, "enter", "y")
	if !strings.Contains(m.toast.text, "failed") || !strings.Contains(m.lastErr.text, "unable to find container") || m.load.active() {
		t.Fatalf("toast %q err %q spinner %v", m.toast.text, m.lastErr.text, m.load.active())
	}

	f2 := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(strings.Join(r.Args, " "), "-o json") {
			return shell.Fail(1, `Error from server (Forbidden): deployments.apps "web" is forbidden`)
		}
		return actionCluster().Handler(r)
	}}
	m = open(t, actionModel(t, f2), "deploy")
	m = pressSettle(t, m, "i")
	if m.dlg != nil || !strings.Contains(m.lastErr.text, "forbidden") {
		t.Fatalf("read error: dlg %v err %q", m.dlg != nil, m.lastErr.text)
	}
}

func TestSetImageProd(t *testing.T) {
	f := imageCluster("")
	m := actionModel(t, f)
	m.cfg.Prod = []string{"kind-dev"}
	m = open(t, m, "deploy")
	m = pressSettle(t, m, "i")
	m = typeText(t, m, "-p")
	m = pressSettle(t, m, "enter")
	if v := m.View(); !strings.Contains(v, skull) || !strings.Contains(ansi.Strip(v), "production") {
		t.Fatal("no ☠ on prod")
	}
}

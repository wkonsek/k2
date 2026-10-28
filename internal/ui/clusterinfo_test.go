package ui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const infoVersionJSON = `{"clientVersion": {"gitVersion": "v1.31.2"}, "serverVersion": {"gitVersion": "v1.29.3"}}`

func versionFake(calls *int32, ocp string) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		args := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(args, "version -o json"):
			atomic.AddInt32(calls, 1)
			if strings.Contains(args, "--context broken") {
				res := shell.Fail(1, "")
				res.Stdout = []byte(`{"clientVersion": {"gitVersion": "v1.31.2"}}`)
				return res
			}
			return shell.OK(infoVersionJSON)
		case strings.HasPrefix(args, "get clusterversion version"):
			if ocp == "" {
				return shell.Fail(1, "Error from server (Forbidden): forbidden")
			}
			return shell.OK(ocp)
		}
		return shell.Fail(1, "no cluster in tests")
	}}
}

func tickOnce(t *testing.T, m model) model {
	t.Helper()
	next, cmd := m.Update(tickMsg(time.Now()))
	return settle(t, next.(model), cmd)
}

func sidebarText(m model) string {
	return ansi.Strip(m.sidebarBody(m.cfg.UI.SidebarWidth-2, m.height-2))
}

func TestClusterInfoInSidebar(t *testing.T) {
	var calls int32
	m := newTestModel(t, testEnv(t), versionFake(&calls, "4.16.3"))
	m.version = "v0.4.1"
	m.kubeconfig, m.context = "/kc", "dev"

	side := sidebarText(m)
	if !strings.Contains(side, "k2          v0.4.1") || strings.Contains(side, "N/A") {
		t.Fatalf("before fetch:\n%s", side)
	}
	m = tickOnce(t, m)
	side = sidebarText(m)
	for _, want := range []string{"kubectl     v1.31.2", "k8s server  v1.29.3", "ocp         4.16.3"} {
		if !strings.Contains(side, want) {
			t.Fatalf("missing %q:\n%s", want, side)
		}
	}

	m = tickOnce(t, m)
	m = tickOnce(t, m)
	if calls != 1 {
		t.Fatalf("fetched %d times", calls)
	}

	m.context = "broken"
	m = tickOnce(t, m)
	side = sidebarText(m)
	if calls != 2 || !strings.Contains(side, "k8s server  N/A") || !strings.Contains(side, "kubectl     v1.31.2") || strings.Contains(side, "ocp") {
		t.Fatalf("broken context (calls %d):\n%s", calls, side)
	}

	m.context = "dev"
	m = tickOnce(t, m)
	m = tickOnce(t, m)
	if calls != 3 || !strings.Contains(sidebarText(m), "k8s server  v1.29.3") {
		t.Fatalf("back to dev: calls %d", calls)
	}
}

func TestClusterInfoForbiddenOCP(t *testing.T) {
	var calls int32
	m := newTestModel(t, testEnv(t), versionFake(&calls, ""))
	m.kubeconfig, m.context = "/kc", "dev"
	m = tickOnce(t, m)
	side := sidebarText(m)
	if strings.Contains(side, "ocp") || strings.Contains(side, "N/A") || m.toast.text != "" {
		t.Fatalf("forbidden ocp:\n%s\ntoast %q", side, m.toast.text)
	}
}

func TestClusterInfoOffAndSmall(t *testing.T) {
	var calls int32
	cfg := testEnv(t)
	cfg.UI.ClusterInfo = false
	m := newTestModel(t, cfg, versionFake(&calls, "4.16.3"))
	m.kubeconfig, m.context = "/kc", "dev"
	m = tickOnce(t, m)
	if calls != 0 || strings.Contains(sidebarText(m), "kubectl ") {
		t.Fatalf("off: calls %d\n%s", calls, sidebarText(m))
	}

	m = newTestModel(t, testEnv(t), versionFake(&calls, "4.16.3"))
	m.kubeconfig, m.context = "/kc", "a-very-long-context-name-that-wraps"
	m = tickOnce(t, m)

	h := 10
	off := m
	off.cfg.UI.ClusterInfo = false
	for !strings.Contains(ansi.Strip(off.sidebarBody(20, h)), "Kustomize") {
		h++
	}
	short := ansi.Strip(m.sidebarBody(20, h))
	if strings.Contains(short, "kubectl ") || !strings.Contains(short, "Kustomize") {
		t.Fatalf("short sidebar (%d rows):\n%s", h, short)
	}
	if tall := ansi.Strip(m.sidebarBody(20, h+8)); !strings.Contains(tall, "kubectl ") {
		t.Fatalf("block missing with room:\n%s", tall)
	}
	narrow := m.sidebarBody(12, 40)
	for _, l := range strings.Split(narrow, "\n") {
		if w := ansi.StringWidth(l); w > 12 {
			t.Fatalf("line %q is %d wide", ansi.Strip(l), w)
		}
	}
	if !strings.Contains(ansi.Strip(narrow), "kubectl ") {
		t.Fatalf("narrow sidebar lost the block:\n%s", ansi.Strip(narrow))
	}

	for _, size := range [][2]int{{60, 20}, {80, 24}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		_ = m.View()
	}
}

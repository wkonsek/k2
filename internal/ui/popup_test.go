package ui

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"k2/internal/forward"
	"k2/internal/shell"
)

type forwardCluster struct {
	mu    sync.Mutex
	busy  map[string]bool
	spawn []string
}

func (c *forwardCluster) fake() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case r.Name == "sh" && strings.Contains(line, "port-forward"):
			c.mu.Lock()
			defer c.mu.Unlock()
			c.spawn = append(c.spawn, line)
			for p := range c.busy {
				if strings.Contains(line, " "+p+":") {
					delete(c.busy, p)
					return shell.Fail(1, "Unable to listen on port "+p+": bind: address already in use")
				}
			}
			return shell.OK(strconv.Itoa(os.Getpid()) + "\n")
		case len(r.Args) > 1 && r.Args[0] == "get" && r.Args[1] == "pods":
			return shell.OK(podsJSON)
		case strings.Contains(line, "get pod web-1 -n shop"):
			return shell.OK(`{"metadata":{"labels":{"app":"web"}},"spec":{"containers":[{"ports":[{"name":"http","containerPort":8080},{"containerPort":9090}]}]}}`)
		case strings.Contains(line, "get svc -n shop"):
			return shell.OK(`{"items":[{"metadata":{"name":"web"},"spec":{"selector":{"app":"web"},"ports":[{"port":80,"targetPort":"http"}]}}]}`)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func (c *forwardCluster) spawned() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.spawn...)
}

func podsModel(t *testing.T, f *shell.Fake) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	n, c := m.openPods()
	m = settle(t, n.(model), c)
	for i, it := range m.content.Items() {
		if it.(item).id == "shop/web-1" {
			m.content.Select(i)
		}
	}
	return m
}

func TestFAlsoOpensPortForward(t *testing.T) {
	m := podsModel(t, podCluster())
	m = press(t, m, "F")
	if !m.pfOn || !strings.Contains(ansi.Strip(m.View()), "port-forward pod/web-1") {
		t.Fatalf("F did not open the forward popup: pf %v status %q", m.pfOn, m.status)
	}
}

func TestTeaPortForwardOpenCancel(t *testing.T) {
	c := &forwardCluster{}
	m := podsModel(t, c.fake())
	tm := start(t, m)
	send(t, tm, "ctrl+f")
	waitFor(t, tm, "port-forward pod/web-1")
	send(t, tm, "esc")
	final := finish(t, tm)
	if final.pfOn || final.status != "port-forward cancelled" || final.stage != "workpods" || len(c.spawned()) != 0 || len(final.forwards) != 0 {
		t.Fatalf("pfOn %v stage %q spawned %v", final.pfOn, final.stage, c.spawned())
	}
}

func TestTeaPortForwardTypingAndSuggestions(t *testing.T) {
	c := &forwardCluster{}
	m := podsModel(t, c.fake())
	tm := start(t, m)
	send(t, tm, "ctrl+f")
	waitFor(t, tm, "port-forward pod/web-1")

	send(t, tm, "tab", "tab", "shift+tab", "down", "up", "up")
	send(t, tm, "ctrl+u")
	for _, k := range []string{"q", "f", "d", "O", "F", "backspace", "backspace", "backspace", "backspace", "backspace"} {
		send(t, tm, k)
	}
	send(t, tm, "1", "8", "0", "8", "0", ":", "8", "0", "8", "0", "enter")
	waitFor(t, tm, "forward 18080 → pod/web-1")
	final := finish(t, tm)
	if final.pfOn || final.stage != "workpods" || final.zoom || len(final.forwards) != 1 {
		t.Fatalf("pfOn %v stage %q zoom %v forwards %v", final.pfOn, final.stage, final.zoom, final.forwards)
	}
	sp := c.spawned()
	if len(sp) != 1 || !strings.Contains(sp[0], "-n shop pod/web-1 18080:8080 --kubeconfig /kc --context kind-dev") {
		t.Fatalf("spawned %v", sp)
	}
}

func TestPortForwardSuggestionCycle(t *testing.T) {
	c := &forwardCluster{}
	m := podsModel(t, c.fake())
	m = press(t, m, "ctrl+f")
	want := []string{"80:8080", "8080:8080", "9090:9090"}
	if len(m.suggestions) != 3 || m.portInput.Value() != want[0] {
		t.Fatalf("suggestions %+v value %q", m.suggestions, m.portInput.Value())
	}
	for _, step := range []struct {
		key  string
		want int
	}{{"tab", 1}, {"down", 2}, {"tab", 0}, {"shift+tab", 2}, {"up", 1}} {
		m = press(t, m, step.key)
		if m.portInput.Value() != want[step.want] {
			t.Fatalf("%s: %q, want %q", step.key, m.portInput.Value(), want[step.want])
		}
	}
	m = press(t, m, "j", "k")
	if m.portInput.Value() != "8080:8080jk" || !m.pfOn {
		t.Fatalf("typed %q", m.portInput.Value())
	}
	m = press(t, m, "enter")
	if !m.pfOn || !strings.Contains(m.pfMsg, "local:remote") {
		t.Fatalf("bad ports accepted: %q", m.pfMsg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "type local:remote") || !strings.Contains(view, "╭ ⇄ port-forward pod/web-1") {
		t.Fatalf("popup view:\n%s", view)
	}
}

func TestTeaPortForwardBusyPort(t *testing.T) {
	c := &forwardCluster{busy: map[string]bool{"8080": true}}
	m := podsModel(t, c.fake())
	tm := start(t, m)
	send(t, tm, "ctrl+f", "tab", "enter")
	waitFor(t, tm, "port 8080 is in use. kill it and forward again?")
	send(t, tm, "n")
	waitFor(t, tm, "kept the port")
	c.mu.Lock()
	c.busy["8080"] = true
	c.mu.Unlock()
	send(t, tm, "enter")
	waitFor(t, tm, "port 8080 is in use")
	send(t, tm, "y")
	waitFor(t, tm, "forward 8080 → pod/web-1")
	final := finish(t, tm)
	if final.pfOn || len(final.forwards) != 1 || final.forwards[0].Local != "8080" {
		t.Fatalf("pfOn %v forwards %+v", final.pfOn, final.forwards)
	}
	if n := len(c.spawned()); n != 3 {
		t.Fatalf("%d starts, want 3: %v", n, c.spawned())
	}
}

func TestPortForwardProdAndKinds(t *testing.T) {
	cfg := testEnv(t)
	cfg.Prod = []string{"kind-prod"}
	c := &forwardCluster{}
	m := newTestModel(t, cfg, c.fake())
	m.kubeconfig, m.context = "/kc", "kind-prod"
	n, cmd := m.openPods()
	m = settle(t, n.(model), cmd)
	m = press(t, m, "ctrl+f")
	if !strings.Contains(ansi.Strip(m.View()), "☠ ⇄ port-forward") {
		t.Fatalf("no skull:\n%s", ansi.Strip(m.View()))
	}
	m = press(t, m, "esc")
	m.stage, m.kind = "res", "configmaps"
	if m = press(t, m, "ctrl+f"); m.pfOn || !strings.Contains(m.status, "cannot be forwarded") {
		t.Fatalf("configmap: pfOn %v status %q", m.pfOn, m.status)
	}
}

func TestForwardsListWithoutNew(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.forwards = append(m.forwards, forwardFor(os.Getpid(), "8080", "80", "svc", "web"))
	n, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = press(t, n.(model), "f")
	if m.stage != "fwds" {
		t.Fatalf("stage %q", m.stage)
	}
	for _, k := range []string{"N", "enter"} {
		if got := press(t, m, k); got.stage != "fwds" || got.pfOn {
			t.Fatalf("%s: stage %q", k, got.stage)
		}
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"STATUS  PORTS    TARGET   NAMESPACE  CONTEXT", "alive   8080→80  svc/web  shop       /kc · kind-dev"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if l := ansi.Strip(m.legend()); strings.Contains(l, "new") || !strings.Contains(l, "^d delete") || !strings.Contains(l, "D delete all") {
		t.Fatalf("legend %q", l)
	}
	if got := press(t, m, "ctrl+d"); got.asking() != "■ stop port-forward" {
		t.Fatalf("ctrl+d confirm %q", got.asking())
	}
}

func forwardFor(pid int, local, remote, kind, name string) forward.Forward {
	return forward.Forward{Namespace: "shop", Pod: name, Kind: kind, Local: local, Remote: remote, PID: pid,
		Started: time.Now(), Kubeconfig: "/kc", Context: "kind-dev"}
}

func TestOverlay(t *testing.T) {
	screen := "abcdefgh\n\x1b[31mredredre\x1b[0m\n12345678"
	got := overlay(screen, "XY\nZW\nQQ", 3, 1)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 || lines[0] != "abcdefgh" {
		t.Fatalf("lines %q", lines)
	}
	if ansi.Strip(lines[1]) != "redXYdre" || ansi.Strip(lines[2]) != "123ZW678" {
		t.Fatalf("overlay %q", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) != 8 {
			t.Fatalf("width of %q", l)
		}
	}
}

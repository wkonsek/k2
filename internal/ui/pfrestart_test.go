package ui

import (
	"os"
	"os/exec"
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

func livePID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skip("no sleep binary:", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	go func() { _, _ = cmd.Process.Wait() }()
	return cmd.Process.Pid
}

const deadPID = 1 << 22

type pfCluster struct {
	mu      sync.Mutex
	pods    map[string]bool
	owners  map[string]string
	spawn   func() shell.Result
	spawns  []string
	listing string
	log     string
	probe   func() shell.Result
	probes  int
}

func (c *pfCluster) fake() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		c.mu.Lock()
		defer c.mu.Unlock()
		args := strings.Join(r.Args, " ")
		switch {
		case r.Name == "sh":
			for i, a := range r.Args {
				if a == "port-forward" {
					c.spawns = append(c.spawns, r.Args[i+3]+" "+r.Args[i+4])
				}
			}
			if c.log != "" {
				for _, e := range r.Env {
					if path, ok := strings.CutPrefix(e, "K2_LOG="); ok {
						_ = os.WriteFile(path, []byte(c.log), 0o644)
					}
				}
			}
			return c.spawn()
		case r.Name == "kubectl" && strings.HasPrefix(args, "get deployments,statefulsets,pods "):
			return shell.OK(c.listing)
		case r.Name == "kubectl" && strings.Contains(args, "--request-timeout=5s"):
			c.probes++
			if c.probe == nil {
				return shell.OK("")
			}
			return c.probe()
		case r.Name == "kubectl" && strings.HasPrefix(args, "get pod ") && strings.Contains(args, "deletionTimestamp"):
			if c.pods[r.Args[2]] {
				return shell.OK("")
			}
			return shell.Fail(1, `Error from server (NotFound): pods "`+r.Args[2]+`" not found`)
		case r.Name == "kubectl" && strings.HasPrefix(args, "get "):
			return shell.OK(c.owners[r.Args[1]+"/"+r.Args[2]])
		}
		return shell.Fail(1, "no cluster in tests")
	}}
}

func pfModel(t *testing.T, c *pfCluster, auto bool, fs ...forward.Forward) model {
	t.Helper()
	cfg := testEnv(t)
	cfg.PortForward.AutoRestart = auto
	m := newTestModel(t, cfg, c.fake())
	m.forwards = fs
	m = press(t, m, "f")
	if m.stage != "fwds" {
		t.Fatalf("stage %q", m.stage)
	}
	return m
}

func run(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()

	for _, msg := range drainWait(cmd, 1500*time.Millisecond) {
		switch msg.(type) {
		case pfRestartMsg, pfOwnerMsg, pfProbeMsg:
			next, more := m.Update(msg)
			m = run(t, next.(model), more)
		}
	}
	return m
}

func svcForward(pid int) forward.Forward {
	return forward.Forward{Namespace: "shop", Pod: "web", Kind: "svc", Local: "8080", Remote: "80", PID: pid, Started: time.Now().Add(-time.Hour), Kubeconfig: "/kc", Context: "dev"}
}

func TestPfRestartService(t *testing.T) {
	pid := livePID(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.OK(strconv.Itoa(pid)) }}
	m := pfModel(t, c, false, svcForward(deadPID), forward.Forward{Namespace: "shop", Pod: "db", Kind: "svc", Local: "5432", Remote: "5432", PID: deadPID + 1})
	m.content.Select(0)
	if !strings.Contains(ansi.Strip(m.content.SelectedItem().(item).title), "dead") {
		t.Fatalf("row %q", m.content.SelectedItem().(item).title)
	}
	next, cmd := m.Update(key(t, "r"))
	m = next.(model)
	if !m.load.active() {
		t.Fatal("no spinner while restarting")
	}
	m = run(t, m, cmd)
	if m.load.active() {
		t.Fatal("spinner left running")
	}
	f := m.forwards[0]
	if f.PID != pid || f.Local != "8080" || forward.Target(f) != "svc/web" || time.Since(f.Started) > time.Minute || len(m.forwards) != 2 {
		t.Fatalf("forward %+v", f)
	}
	if row := ansi.Strip(m.content.Items()[0].(item).title); !strings.Contains(row, "alive") || !strings.Contains(row, "svc/web") {
		t.Fatalf("row %q", row)
	}
	if c.spawns[0] != "svc/web 8080:80" || m.toast.text != "pf 8080 restarted" {
		t.Fatalf("spawns %v toast %q", c.spawns, m.toast.text)
	}
}

func TestPfRestartPod(t *testing.T) {
	pid := livePID(t)
	c := &pfCluster{
		pods:   map[string]bool{"web-1": true},
		owners: map[string]string{"pod/web-2": "ReplicaSet/web-5d9", "rs/web-5d9": "Deployment/kafka-connect"},
		spawn:  func() shell.Result { return shell.OK(strconv.Itoa(pid)) },
	}
	pod := func(name, local string) forward.Forward {
		return forward.Forward{Namespace: "shop", Pod: name, Local: local, Remote: "80", PID: deadPID, Kubeconfig: "/kc", Context: "dev"}
	}

	m := pfModel(t, c, false, pod("web-2", "8082"))
	m = run(t, m, ownerCmd(m.k, m.forwards[0]))
	if m.forwards[0].Owner != "deploy/kafka-connect" {
		t.Fatalf("owner %q", m.forwards[0].Owner)
	}
	next, cmd := m.Update(key(t, "r"))
	m = run(t, next.(model), cmd)
	if f := m.forwards[0]; forward.Target(f) != "deploy/kafka-connect" || f.PID != pid {
		t.Fatalf("fallback %+v", f)
	}
	if m.toast.text != "pod gone → forwarding via deploy/kafka-connect" {
		t.Fatalf("toast %q", m.toast.text)
	}

	m = pfModel(t, c, false, pod("web-1", "8081"))
	next, cmd = m.Update(key(t, "r"))
	m = run(t, next.(model), cmd)
	if forward.Target(m.forwards[0]) != "pod/web-1" || m.forwards[0].PID != pid {
		t.Fatalf("existing pod %+v", m.forwards[0])
	}

	m = pfModel(t, c, false, pod("lonely", "8083"))
	next, cmd = m.Update(key(t, "r"))
	m = run(t, next.(model), cmd)
	if m.load.active() || m.forwards[0].PID != deadPID || !strings.Contains(m.status, "pod lonely not found; no owner to fall back to") {
		t.Fatalf("no owner: spinner %v status %q", m.load.active(), m.status)
	}
	if m.lastErr.text == "" || !strings.Contains(m.lastErr.text, "no owner") {
		t.Fatalf("last error %q", m.lastErr.text)
	}
}

func TestPfRestartBusyPort(t *testing.T) {
	old := livePID(t)
	c := &pfCluster{spawn: func() shell.Result {
		return shell.Fail(1, "Unable to listen on port 8080: bind: address already in use")
	}}
	m := pfModel(t, c, false, svcForward(old))
	next, cmd := m.Update(key(t, "r"))
	m = run(t, next.(model), cmd)
	if !strings.Contains(m.status, "local port 8080 is busy") || m.load.active() {
		t.Fatalf("status %q spinner %v", m.status, m.load.active())
	}
}

func fakeNow(t *testing.T) *time.Time {
	at := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
	return &at
}

func tickAuto(t *testing.T, m model) model {
	cmds := m.autoRestarts()
	return run(t, m, tea.Batch(cmds...))
}

func TestPfAutoOff(t *testing.T) {
	at := fakeNow(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.OK("1") }}
	m := pfModel(t, c, false, svcForward(deadPID))
	for i := 0; i < 5; i++ {
		*at = at.Add(time.Minute)
		m = tickAuto(t, m)
	}
	if len(c.spawns) != 0 || m.pfStatus(m.forwards[0]) != "dead" {
		t.Fatalf("spawns %v status %q", c.spawns, m.pfStatus(m.forwards[0]))
	}
}

func TestPfAutoBackoff(t *testing.T) {
	at := fakeNow(t)
	pid := livePID(t)
	fails := 2
	c := &pfCluster{spawn: func() shell.Result {
		if fails > 0 {
			fails--
			return shell.Fail(1, "error: connection refused")
		}
		return shell.OK(strconv.Itoa(pid))
	}}
	m := pfModel(t, c, true, svcForward(deadPID))
	m = tickAuto(t, m)
	if len(c.spawns) != 0 || m.pfStatus(m.forwards[0]) != "restarting (1)" {
		t.Fatalf("not scheduled: %v %q", c.spawns, m.pfStatus(m.forwards[0]))
	}
	*at = at.Add(time.Second)
	m = tickAuto(t, m)
	if len(c.spawns) != 1 || m.pfStatus(m.forwards[0]) != "restarting (2)" {
		t.Fatalf("attempt 1: %v %q", c.spawns, m.pfStatus(m.forwards[0]))
	}
	*at = at.Add(time.Second)
	if m = tickAuto(t, m); len(c.spawns) != 1 {
		t.Fatal("attempt 2 before its 2s backoff")
	}
	*at = at.Add(time.Second)
	m = tickAuto(t, m)
	*at = at.Add(3 * time.Second)
	if m = tickAuto(t, m); len(c.spawns) != 2 {
		t.Fatalf("attempt 3 before its 4s backoff: %d", len(c.spawns))
	}
	*at = at.Add(time.Second)
	m = tickAuto(t, m)
	if len(c.spawns) != 3 || m.forwards[0].PID != pid || m.toast.text != "pf 8080 restored" || m.pfStatus(m.forwards[0]) != "alive" {
		t.Fatalf("restore: %v pid %d toast %q", c.spawns, m.forwards[0].PID, m.toast.text)
	}

	if !strings.Contains(m.lastErr.text, "connection refused") {
		t.Fatalf("last error %q", m.lastErr.text)
	}

	*at = at.Add(31 * time.Second)
	m = tickAuto(t, m)
	if st := m.pfAuto["8080"]; st.attempts != 0 {
		t.Fatalf("attempts %d after 30 s alive", st.attempts)
	}
	if m.backoff(0) != time.Second || m.backoff(3) != 8*time.Second || m.backoff(10) != 30*time.Second {
		t.Fatalf("backoff %v %v %v", m.backoff(0), m.backoff(3), m.backoff(10))
	}
}

func TestPfAutoOwnerFallback(t *testing.T) {
	at := fakeNow(t)
	pid := livePID(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.OK(strconv.Itoa(pid)) }}
	f := forward.Forward{Namespace: "shop", Pod: "kc-5d9-abc", Local: "8083", Remote: "8083", PID: deadPID, Owner: "deploy/kafka-connect"}
	m := pfModel(t, c, true, f)
	m = tickAuto(t, m)
	*at = at.Add(time.Second)
	m = tickAuto(t, m)
	if forward.Target(m.forwards[0]) != "deploy/kafka-connect" || m.toast.text != "pf 8083 restored via deploy/kafka-connect" {
		t.Fatalf("forward %+v toast %q", m.forwards[0], m.toast.text)
	}
}

func TestPfAutoManualStop(t *testing.T) {
	at := fakeNow(t)
	pid := livePID(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.OK("1") }}
	m := pfModel(t, c, true, svcForward(pid))
	m = press(t, m, "ctrl+d", "y")
	if len(m.forwards) != 0 {
		t.Fatalf("not stopped: %d", len(m.forwards))
	}
	for i := 0; i < 5; i++ {
		*at = at.Add(time.Minute)
		m = tickAuto(t, m)
	}
	if len(c.spawns) != 0 {
		t.Fatalf("a stopped forward was restarted: %v", c.spawns)
	}
}

func TestPfAutoGivesUp(t *testing.T) {
	at := fakeNow(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.Fail(1, "error: lost connection") }}
	cfgMax := 3
	m := pfModel(t, c, true, svcForward(deadPID))
	m.cfg.PortForward.MaxRestarts = cfgMax
	for i := 0; i < 20; i++ {
		*at = at.Add(time.Minute)
		m = tickAuto(t, m)
	}
	if len(c.spawns) != cfgMax || m.pfStatus(m.forwards[0]) != "dead" || !strings.Contains(m.status, "gave up after 3 restarts") {
		t.Fatalf("spawns %d status %q line %q", len(c.spawns), m.pfStatus(m.forwards[0]), m.status)
	}

	next, cmd := m.Update(key(t, "r"))
	m = run(t, next.(model), cmd)
	if len(c.spawns) != cfgMax+1 {
		t.Fatalf("manual restart after giving up: %d", len(c.spawns))
	}
}

func TestPfAutoToggle(t *testing.T) {
	at := fakeNow(t)
	c := &pfCluster{spawn: func() shell.Result { return shell.Fail(1, "x") }}
	m := pfModel(t, c, false, svcForward(deadPID))
	m = press(t, m, "a")
	if m.forwards[0].Auto != "on" || !m.autoOn(m.forwards[0]) || !strings.Contains(m.status, "auto-restart on") {
		t.Fatalf("toggle on: %+v %q", m.forwards[0], m.status)
	}
	m = tickAuto(t, m)
	*at = at.Add(2 * time.Second)
	m = tickAuto(t, m)
	if len(c.spawns) != 1 {
		t.Fatalf("per-forward on: %d spawns", len(c.spawns))
	}
	m = press(t, m, "a")
	if m.forwards[0].Auto != "off" || m.autoOn(m.forwards[0]) {
		t.Fatalf("toggle off: %+v", m.forwards[0])
	}
}

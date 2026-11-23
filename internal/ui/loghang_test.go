package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func TestQuietFollowShowsWaitingAndLeaves(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 200)
	d.idle(200)
	if d.m.load.active() {
		t.Fatal("follow still shows a loading spinner")
	}
	if f.running.Load() != 1 {
		t.Fatalf("follow not running: %d", f.running.Load())
	}
	body := ansi.Strip(d.m.logsBody(90))
	if !strings.Contains(body, "no log lines yet, waiting") {
		t.Fatalf("missing empty state:\n%s", body)
	}
	d.press(":")
	if !d.m.cmdOn {
		t.Fatal(": did not open the command line")
	}
	d.press("esc")
	if d.m.cmdOn {
		t.Fatal("esc did not close the command line")
	}
	d.press("q")
	d.until("the follow stops", func() bool { return f.running.Load() == 0 })
	if d.m.stage == "logs" || d.m.load.active() {
		t.Fatalf("q left stage %s load %v", d.m.stage, d.m.load.active())
	}
}

func TestQuietFollowAllowsPodsCommand(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 200)
	d.idle(100)
	next, cmd := d.m.runCommand("pods")
	d.m = next.(model)
	d.run(cmd)
	d.until("left logs", func() bool { return d.m.stage != "logs" })
	d.until("the follow stops", func() bool { return f.running.Load() == 0 })
	if d.m.stage != "workpods" || d.m.load.active() {
		t.Fatalf("after :pods: stage %s load %v", d.m.stage, d.m.load.active())
	}
}

func TestSwitchContainerWhileFollowIsQuiet(t *testing.T) {
	f := &feed{}
	m := containersModel(t, f.over(containerCluster()))
	m.cfg.Logs.MaxLines = 200
	m.height, m.width = 30, 100
	d := newDriver(t, m)
	d.press("l")
	if d.m.stage != "logs" || d.m.vs.logCont == "" {
		t.Fatalf("logs stage %s container %q", d.m.stage, d.m.vs.logCont)
	}
	d.idle(80)
	first := d.m.vs.logCont
	d.press("q")
	d.until("back from first", func() bool { return d.m.stage != "logs" && f.running.Load() == 0 })
	d.press("j", "l")
	if d.m.stage != "logs" {
		t.Fatalf("second logs %s", d.m.stage)
	}
	d.idle(80)
	if d.m.load.active() {
		t.Fatal("second container still loading")
	}
	if d.m.vs.logCont == first {
		t.Fatalf("still on %q", first)
	}
	body := ansi.Strip(d.m.logsBody(90))
	if !strings.Contains(body, "no log lines yet, waiting") {
		t.Fatalf("second container missing empty state:\n%s", body)
	}
	d.press("q")
	d.until("the follow stops", func() bool { return f.running.Load() == 0 })
}

func TestOpenCloseFollowDoesNotLeak(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 200)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	const cycles = 20
	for i := 0; i < cycles; i++ {
		d.press("q")
		d.until("stopped", func() bool { return f.running.Load() == 0 && d.m.stage != "logs" })
		d.press("l")
		d.until("started", func() bool { return f.running.Load() == 1 && d.m.stage == "logs" })
	}
	if d.m.load.active() {
		t.Fatal("spinner stuck after reopen")
	}
	d.press("q")
	d.until("final stop", func() bool { return f.running.Load() == 0 })
	if f.running.Load() != 0 {
		t.Fatalf("leaked %d follow runs", f.running.Load())
	}
}

type muxRun struct {
	fake *shell.Fake
	exec shell.Exec
}

func (m muxRun) Run(req shell.Request) shell.Result {
	line := strings.Join(req.Args, " ")
	if len(req.Args) > 0 && req.Args[0] == "logs" && strings.Contains(line, "-f") {
		return m.exec.Run(req)
	}
	return m.fake.Run(req)
}

func (m muxRun) LookPath(name string) (string, error) { return m.fake.LookPath(name) }

func livePIDs(path string) (all, live []int) {
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil || n <= 0 {
			continue
		}
		all = append(all, n)
		if syscall.Kill(n, 0) == nil {
			live = append(live, n)
		}
	}
	return all, live
}

func TestLeavingLogsKillsKubectlChildren(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pids")
	stub := filepath.Join(dir, "kubectl")
	script := fmt.Sprintf("#!/bin/sh\necho $$ >> %q\nif printf '%%s' \"$*\" | grep -q -- '-f'; then\n  sleep 600 &\n  child=$!\n  echo $child >> %q\n  wait $child\n  exit 0\nfi\nexit 0\n", pidFile, pidFile)
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := testEnv(t)
	run := muxRun{fake: actionCluster(), exec: shell.Exec{}}
	m := initialModel(Options{Runner: run, Config: &cfg})
	t.Cleanup(m.streams.stopAll)
	m.width, m.height = 120, 40
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	d := newDriver(t, m)
	next, cmd := m.openPods()
	d.m = next.(model)
	d.run(cmd)
	d.until("pods", func() bool { return d.m.stage == "workpods" && len(d.m.content.Items()) > 0 })
	const cycles = 8
	for i := 0; i < cycles; i++ {
		d.press("l")
		deadline := time.Now().Add(2 * time.Second)
		for {
			_, live := livePIDs(pidFile)
			if d.m.stage == "logs" && len(live) >= 2 || time.Now().After(deadline) {
				break
			}
			d.idle(15)
		}
		if d.m.stage != "logs" {
			t.Fatalf("cycle %d: stage %s", i, d.m.stage)
		}
		if d.m.load.active() {
			t.Fatalf("cycle %d: spinner still up", i)
		}
		d.press("q")
		d.until("left logs", func() bool { return d.m.stage != "logs" })
		deadline = time.Now().Add(3 * time.Second)
		for {
			_, live := livePIDs(pidFile)
			if len(live) == 0 || time.Now().After(deadline) {
				break
			}
			d.idle(15)
		}
		_, live := livePIDs(pidFile)
		if len(live) != 0 {
			all, _ := livePIDs(pidFile)
			t.Fatalf("cycle %d leaked %d after q; recorded %d live %v", i, len(live), len(all), live)
		}
	}
	all, live := livePIDs(pidFile)
	if len(live) != 0 {
		t.Fatalf("after %d cycles leftover %d of %d: %v", cycles, len(live), len(all), live)
	}
	if len(all) < 2*cycles {
		t.Fatalf("stub was not started enough times: recorded %d want >= %d", len(all), 2*cycles)
	}
}

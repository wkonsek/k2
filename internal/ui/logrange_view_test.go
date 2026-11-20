package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

const kafkaAuthLine = "[2026-11-20 21:48:49,072] INFO [SocketServer listenerType=BROKER, nodeId=4] Failed authentication with /10.131.1.197 (channelId=10.130.2.50:39093-10.131.1.197:52758-620643) (SSL handshake failed) (org.apache.kafka.common.network.Selector)"

func TestLessPagerIgnoresLessopen(t *testing.T) {
	cmd := defaultLessCommand("/tmp/k2.log")
	got := strings.Join(cmd.Args, " ")
	if got != "less -R -L +G -- /tmp/k2.log" {
		t.Fatalf("args %q", got)
	}
	env := strings.Join(lessPagerEnv([]string{
		"PATH=/bin",
		"LESS=-S -x40",
		"LESSOPEN=|column -t -s, %s",
		"LESSCLOSE=rm %s",
		"LESSKEY=/tmp/lesskey",
		"HOME=/tmp",
	}), "\n")
	if strings.Contains(env, "LESS=-S") || strings.Contains(env, "LESSOPEN=|") || strings.Contains(env, "LESSCLOSE=rm") || strings.Contains(env, "LESSKEY=/tmp") {
		t.Fatalf("pager still inherited less env:\n%s", env)
	}
	if !strings.Contains(env, "LESS=\n") && !strings.HasSuffix(env, "LESS=") {
		t.Fatalf("LESS not cleared:\n%s", env)
	}
	if !strings.Contains(env, "PATH=/bin") || !strings.Contains(env, "HOME=/tmp") {
		t.Fatalf("lost unrelated env:\n%s", env)
	}
}

func TestLogRangeLessKeepsKafkaCommaInPlace(t *testing.T) {
	if _, err := exec.LookPath("less"); err != nil {
		t.Skip("less not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "lessopen")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntr , '\\t' < \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LESSOPEN", "|"+script+" %s")
	t.Setenv("LESS", "-S")
	body := strings.Repeat(kafkaAuthLine+"\n", 12)
	inner := actionCluster()
	orig := inner.Handler
	inner.Handler = func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "logs" && !slices.Contains(r.Args, "-f") {
			return shell.OK(body)
		}
		return orig(r)
	}
	m := actionModel(t, inner)
	m.cfg.UI.EditorInline = true
	m.cfg.UI.Animations = false
	m.width, m.height = 200, 40
	h := newEdHarness(t, selectRow(t, m, "db-0"))
	h.m.width, h.m.height = 200, 40
	h.keys("l")
	h.pump(func() bool { return h.m.stage == "logs" && !h.m.load.active() }, "logs ready")
	h.keys("1")
	h.pump(func() bool {
		return h.m.ed.live() && strings.Contains(ansi.Strip(h.m.View()), "21:48:49")
	}, "less 30m")
	view := ansi.Strip(h.m.View())
	if !strings.Contains(h.m.paneLabel(h.m.focus), "less 30m") {
		t.Fatalf("title %q", h.m.paneLabel(h.m.focus))
	}
	flat := strings.ReplaceAll(view, "\n", "")
	if !strings.Contains(flat, "21:48:49,072") {
		t.Fatalf("LESSOPEN rewrote the timestamp comma:\n%s", view)
	}
	if !strings.Contains(flat, "BROKER, nodeId=4") {
		t.Fatalf("LESSOPEN rewrote the BROKER comma:\n%s", view)
	}
}

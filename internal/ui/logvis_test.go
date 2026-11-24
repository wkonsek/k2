package ui

import (
	"strings"
	"testing"
)

func TestJoinLogsAndLineCount(t *testing.T) {
	if got := joinLogs("a\nb\n", "c\n"); got != "a\nb\nc\n" {
		t.Fatalf("join trailing: %q", got)
	}
	if got := joinLogs("a\nb", "c"); got != "a\nb\nc" {
		t.Fatalf("join bare: %q", got)
	}
	if got := joinLogs("a\nb\n", ""); got != "a\nb\n" {
		t.Fatalf("join empty more: %q", got)
	}
	if splitLineCount("a\nb\nc\nd\n") != 4 || splitLineCount("a\nb") != 2 || splitLineCount("") != 0 || splitLineCount("\n") != 0 {
		t.Fatalf("counts %d %d %d %d", splitLineCount("a\nb\nc\nd\n"), splitLineCount("a\nb"), splitLineCount(""), splitLineCount("\n"))
	}
	got, n := trimLogDrop("a\nb\nc\nd\n", 2)
	if n != 2 || got != "c\nd\n" {
		t.Fatalf("trim %q drop %d", got, n)
	}
}

func TestApplyLogsKeepsVisibleCap(t *testing.T) {
	m, _ := bigCluster(t, 20, 0)
	m = fillLogs(m, 20)
	m.cfg.Logs.MaxLines = 20
	_ = m.View()
	batch := "new-1\nnew-2\nnew-3\n"
	m.applyLogs(logsMsg{id: m.logID, pod: logKey(m.logPod, m.vs.logOpts()), text: batch, partial: true, stream: true})
	if strings.Contains(m.logText, "\n\n") {
		t.Fatalf("blank line inserted:\n%q", m.logText)
	}
	n := splitLineCount(m.logText)
	if n != 20 {
		t.Fatalf("cap %d, want 20", n)
	}
	if !strings.Contains(m.logText, "new-3") || strings.Contains(m.logText, "line-000000") {
		t.Fatalf("ring did not rotate:\n%s", m.logText[:120])
	}
}

func TestLogVisPatchMatchesRebuild(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		m, _ := bigCluster(t, 20, 0)
		m = fillLogs(m, 80)
		m.cfg.Logs.MaxLines = 80
		if wrap != m.wrapping() {
			m.vs.wrapFlip = !m.vs.wrapFlip
		}
		_ = m.View()
		w := m.viewerWidth()
		before := cloneVis(m.visualLines(w))
		if len(before) < 80 {
			t.Fatalf("wrap=%v: cached %d lines", wrap, len(before))
		}
		batch := strings.TrimRight(strings.Repeat("extra-follow-line-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", 15), "\n") + "\n"
		m.applyLogs(logsMsg{id: m.logID, pod: logKey(m.logPod, m.vs.logOpts()), text: batch, partial: true, stream: true})
		patched := cloneVis(m.visualLines(w))
		m.dropVis(m.logID)
		rebuilt := cloneVis(m.visualLines(w))
		if len(patched) != len(rebuilt) {
			t.Fatalf("wrap=%v: patched %d rebuilt %d", wrap, len(patched), len(rebuilt))
		}
		for i := range patched {
			if patched[i] != rebuilt[i] {
				t.Fatalf("wrap=%v line %d: patched %#v rebuilt %#v", wrap, i, patched[i], rebuilt[i])
			}
		}
	}
}

func cloneVis(in []vline) []vline {
	out := make([]vline, len(in))
	copy(out, in)
	return out
}

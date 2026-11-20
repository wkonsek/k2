package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"k2/internal/kube"

	"github.com/charmbracelet/x/ansi"
)

func burst(from, n int) string {
	var b strings.Builder
	for i := from; i < from+n; i++ {
		fmt.Fprintf(&b, "keep-%05d\n", i)
	}
	return b.String()
}

func bodyRows(body string) string {
	lines := strings.Split(body, "\n")
	return strings.Join(lines[1:], "\n")
}

func busyLogsModel(t *testing.T, limit int) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), nil)
	m.height = 20
	m.width = 100
	m.stage = "logs"
	m.cfg.Logs.MaxLines = limit
	m.logText = strings.TrimRight(burst(0, limit), "\n")
	m.logStick = true
	return m
}

func pollLogs(m *model, from, n int) {
	m.applyLogs(logsMsg{id: m.logID, pod: logKey(m.logPod, m.vs.logOpts()), text: burst(from, n), partial: true})
}

func TestPausedLogsDoNotMoveUnderHighRate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(m *model)
	}{
		{"top", func(m *model) { *m = press(t, *m, "g") }},
		{"middle", func(m *model) { m.scroll(-30) }},
		{"one page up", func(m *model) { m.scroll(1 - m.textRows()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := busyLogsModel(t, 100)
			tc.setup(&m)
			if m.logStick {
				t.Fatal("still following")
			}
			before := bodyRows(ansi.Strip(m.logsBody(90)))
			if !strings.Contains(before, "keep-") {
				t.Fatalf("nothing shown:\n%s", before)
			}
			next := 100
			for i := 0; i < 40; i++ {
				pollLogs(&m, next, 80)
				next += 80
				if got := bodyRows(ansi.Strip(m.logsBody(90))); got != before {
					t.Fatalf("poll %d moved the paused view\nbefore:\n%s\nafter:\n%s", i, before, got)
				}
				if m.logStick {
					t.Fatalf("poll %d resumed follow", i)
				}
			}
			if head := strings.SplitN(ansi.Strip(m.logsBody(90)), "\n", 2)[0]; !strings.Contains(head, "paused +") || !strings.Contains(head, " new") {
				t.Fatalf("header %q", head)
			}
			if n := strings.Count(m.logText, "\n") + 1; n > 2*m.cfg.Logs.MaxLines {
				t.Fatalf("paused buffer grew to %d lines", n)
			}

			m = press(t, m, "G")
			if !m.logStick {
				t.Fatal("G did not resume follow")
			}
			last := fmt.Sprintf("keep-%05d", next-1)
			if !strings.HasSuffix(m.logText, last) {
				t.Fatalf("no catch up, text ends %q want %q", m.logText[max(0, len(m.logText)-30):], last)
			}
			if n := strings.Count(m.logText, "\n") + 1; n > m.cfg.Logs.MaxLines {
				t.Fatalf("followed buffer %d lines", n)
			}
			if body := ansi.Strip(m.logsBody(90)); !strings.Contains(body, last) || !strings.HasPrefix(body, "follow ") {
				t.Fatalf("follow view:\n%s", body)
			}
			pollLogs(&m, next, 5)
			if !strings.HasSuffix(m.logText, fmt.Sprintf("keep-%05d", next+4)) || !m.logStick {
				t.Fatal("follow did not continue")
			}
		})
	}
}

func TestFollowingScrollUpDoesNotDrift(t *testing.T) {
	m := busyLogsModel(t, 100)
	pollLogs(&m, 100, 80)
	m = press(t, m, "k", "k", "k")
	if m.logStick {
		t.Fatal("k did not pause")
	}
	before := bodyRows(ansi.Strip(m.logsBody(90)))
	next := 180
	for i := 0; i < 30; i++ {
		pollLogs(&m, next, 80)
		next += 80
		if got := bodyRows(ansi.Strip(m.logsBody(90))); got != before {
			t.Fatalf("poll %d drifted\nbefore:\n%s\nafter:\n%s", i, before, got)
		}
	}
	m = press(t, m, "pgup")
	moved := bodyRows(ansi.Strip(m.logsBody(90)))
	if moved == before {
		t.Fatal("pgup did not scroll")
	}
	for i := 0; i < 10; i++ {
		pollLogs(&m, next, 80)
		next += 80
	}
	if got := bodyRows(ansi.Strip(m.logsBody(90))); got != moved {
		t.Fatalf("drifted after pgup\nbefore:\n%s\nafter:\n%s", moved, got)
	}
}

func TestPausedLogsKeepMarksOnCatchUp(t *testing.T) {
	m := busyLogsModel(t, 100)
	m = press(t, m, "m", "g")
	if len(m.vs.logMarks) != 1 {
		t.Fatalf("marks %v", m.vs.logMarks)
	}
	next := 100
	for i := 0; i < 10; i++ {
		pollLogs(&m, next, 80)
		next += 80
	}
	m = press(t, m, "G")
	for _, n := range m.vs.logMarks {
		if n < 0 || n >= strings.Count(m.logText, "\n")+1 {
			t.Fatalf("mark %d outside the buffer", n)
		}
	}
}

func busyStern(t *testing.T, limit int) (model, *int) {
	t.Helper()
	m := newTestModel(t, testEnv(t), nil)
	m.height = 20
	m.width = 120
	t0 := time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC)
	var lines []kube.LogLine
	for i := 0; i < limit; i++ {
		lines = append(lines, kube.LogLine{Pod: "kafka-0", Container: "kafka", At: t0.Add(time.Duration(i) * time.Millisecond), Text: fmt.Sprintf("keep-%05d", i)})
	}
	v := &mlogView{lines: lines, colors: map[string]color{"kafka-0/kafka": "#ff3b30"}, polls: 1}
	v.cur = kube.LogCursor{"kafka-0/kafka": lines[len(lines)-1].At}
	m.stage = "mlogs"
	m.vs.ml = v
	m.logText = v.render()
	m.logStick = true
	m.cfg.Logs.MultiMaxLines = limit
	next := limit
	return m, &next
}

func sternPoll(m *model, next *int, n int) {
	t0 := time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC)
	var fresh []kube.LogLine
	for i := 0; i < n; i++ {
		fresh = append(fresh, kube.LogLine{Pod: "kafka-0", Container: "kafka", At: t0.Add(time.Duration(*next) * time.Millisecond), Text: fmt.Sprintf("keep-%05d", *next)})
		*next++
	}
	m.applyMlog(mlogMsg{pane: m.logID, key: mlogKey(m.namespace, m.vs.ml.q), lines: fresh})
}

func TestPausedSternDoesNotMoveUnderHighRate(t *testing.T) {
	for _, name := range []string{"top", "middle"} {
		t.Run(name, func(t *testing.T) {
			m, next := busyStern(t, 100)
			if name == "top" {
				m = press(t, m, "g")
			} else {
				m.scroll(-30)
			}
			if m.logStick {
				t.Fatal("still following")
			}
			before := bodyRows(ansi.Strip(m.logsBody(110)))
			for i := 0; i < 40; i++ {
				sternPoll(&m, next, 80)
				if got := bodyRows(ansi.Strip(m.logsBody(110))); got != before {
					t.Fatalf("poll %d moved the paused view\nbefore:\n%s\nafter:\n%s", i, before, got)
				}
			}
			if head := strings.SplitN(ansi.Strip(m.logsBody(110)), "\n", 2)[0]; !strings.Contains(head, "paused +") {
				t.Fatalf("header %q", head)
			}
			if len(m.vs.ml.lines) > 2*m.cfg.Logs.MultiMaxLines {
				t.Fatalf("paused buffer %d lines", len(m.vs.ml.lines))
			}
			m = press(t, m, "G")
			last := fmt.Sprintf("keep-%05d", *next-1)
			if !m.logStick || !strings.HasSuffix(strings.TrimRight(m.logText, "\n"), last) || len(m.vs.ml.lines) > m.cfg.Logs.MultiMaxLines || len(m.vs.ml.pend) != 0 {
				t.Fatalf("catch up: stick %v lines %d pend %d suffix %v tail %q", m.logStick, len(m.vs.ml.lines), len(m.vs.ml.pend), strings.HasSuffix(strings.TrimRight(m.logText, "\n"), last), m.logText[max(0, len(m.logText)-60):])
			}
		})
	}
}

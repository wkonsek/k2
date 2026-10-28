package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func copyClip(t *testing.T) *[]string {
	t.Helper()
	old := clipboardRun
	t.Cleanup(func() { clipboardRun = old })
	var got []string
	clipboardRun = func(mode, text string) (string, error) { got = append(got, text); return "wl-copy", nil }
	return &got
}

func runCmd(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.BatchMsg); !ok {
			next, _ := m.Update(msg)
			return next.(model)
		}
	}
	return m
}

const copyLog = "first line\n\tat org.Example.run(Example.java:12)\nthird \x1b[31mred\x1b[0m line\nfourth\n"

func TestCopyModeRendersOnlyText(t *testing.T) {
	for _, zoom := range []bool{false, true} {
		m := actionModel(t, actionCluster())
		m = pressSettle(t, m, "ctrl+a", "v")
		m = showLog(m, copyLog)
		m.zoom = zoom
		next, cmd := m.Update(key(t, "y"))
		m = next.(model)
		if m.copy == nil || cmd == nil {
			t.Fatal("y did not enter copy mode")
		}
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != m.height {
			t.Fatalf("copy mode has %d lines, want %d", len(lines), m.height)
		}
		plain := ansi.Strip(view)
		for _, bad := range []string{"│", "╭", "─", "kafka-0", "previous", "Kubeconfig", "1 first"} {
			if strings.Contains(plain, bad) {
				t.Fatalf("zoom %v: copy mode draws %q:\n%s", zoom, bad, plain)
			}
		}
		if !strings.HasPrefix(plain, "first line") || !strings.Contains(plain, "\n        at org.Example.run") || !strings.Contains(plain, "third red line") {
			t.Fatalf("text not raw / sanitized:\n%s", plain)
		}
		if !strings.Contains(lines[len(lines)-1], "COPY") {
			t.Fatalf("no hint line: %q", lines[len(lines)-1])
		}
		for i, l := range lines {
			if ansi.StringWidth(l) > m.width {
				t.Fatalf("line %d wider than the terminal", i)
			}
			for _, r := range ansi.Strip(l) {
				if r < 0x20 {
					t.Fatalf("control %q in line %d", r, i)
				}
			}
		}

		m = press(t, m, "q")
		if m.copy != nil || m.stage != "logs" || m.zoom != zoom || !strings.Contains(ansi.Strip(m.View()), "│") {
			t.Fatal("q did not go back to the viewer")
		}
	}

	m := actionModel(t, actionCluster())
	m.cfg.UI.CopyModeHint = false
	m = showLog(m, copyLog)
	m = press(t, m, "y")
	if strings.Contains(ansi.Strip(m.View()), "COPY") {
		t.Fatal("hint shown with copy_mode_hint: false")
	}
	m = press(t, m, "esc")
	if m.copy != nil {
		t.Fatal("esc did not leave copy mode")
	}
}

func TestCopyModeSelectAndYank(t *testing.T) {
	trueColor(t)
	got := copyClip(t)
	m := actionModel(t, actionCluster())
	m = showLog(m, copyLog)
	m.logStick = false
	m = press(t, m, "y")

	next, cmd := m.Update(key(t, "y"))
	m = runCmd(next.(model), cmd)
	if len(*got) != 1 || (*got)[0] != "first line" || !strings.Contains(m.status, "copied 1 line (wl-copy)") {
		t.Fatalf("cursor line: %q status %q", *got, m.status)
	}
	if !strings.Contains(ansi.Strip(m.View()), "copied 1 line") {
		t.Fatal("copy result not on the hint line")
	}

	m = press(t, m, "j", "v", "j", "j")
	if m.copy.anchor != 1 || m.copy.cur != 3 {
		t.Fatalf("selection %+v", *m.copy)
	}
	if v := m.View(); !strings.Contains(v, "\x1b[7m") {
		t.Fatalf("selection not highlighted: %q", v)
	}
	next, cmd = m.Update(key(t, "y"))
	m = runCmd(next.(model), cmd)
	want := "        at org.Example.run(Example.java:12)\nthird red line\nfourth"
	if len(*got) != 2 || (*got)[1] != want || !strings.Contains(m.status, "copied 3 lines") || m.copy.anchor != -1 {
		t.Fatalf("selection yank %q status %q", *got, m.status)
	}

	next, cmd = m.Update(key(t, "Y"))
	m = runCmd(next.(model), cmd)
	if (*got)[2] != "first line\n"+want || !strings.Contains(m.status, "copied all 4 lines") {
		t.Fatalf("Y %q %q", (*got)[2], m.status)
	}

	m = press(t, m, "G")
	if m.copy.cur != 3 {
		t.Fatalf("G cur %d", m.copy.cur)
	}
	m = press(t, m, "g")
	if m.copy.cur != 0 {
		t.Fatalf("g cur %d", m.copy.cur)
	}
}

func TestCopyModeWrap(t *testing.T) {
	got := copyClip(t)
	long := strings.Repeat("word ", 60) + "END"
	m := actionModel(t, actionCluster())
	m = showLog(m, "a\n"+long+"\nb\n")
	m.logStick, m.logAt = false, 0
	m = press(t, m, "y")
	if strings.Contains(ansi.Strip(m.View()), "END") {
		t.Fatal("wrap off shows the end of a wider line")
	}
	m = press(t, m, "w", "j")
	if !strings.Contains(ansi.Strip(m.View()), "END") || !m.wrapping() {
		t.Fatal("w in copy mode did not wrap")
	}
	m = press(t, m, "v", "j")
	next, cmd := m.Update(key(t, "y"))
	m = runCmd(next.(model), cmd)
	if len(*got) != 1 || (*got)[0] != long {
		t.Fatalf("wrapped yank %q", *got)
	}
	m = press(t, m, "q")
	if m.copy != nil || !m.wrapping() || m.logAt != 0 || m.logStick {
		t.Fatalf("exit: wrap %v at %d stick %v", m.wrapping(), m.logAt, m.logStick)
	}
}

func TestCopyModeOnlyInViewers(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = press(t, m, "y")
	if m.copy != nil {
		t.Fatal("copy mode from a table")
	}
	for _, stage := range []string{"describe", "yaml", "view", "kout"} {
		m := actionModel(t, actionCluster())
		m.stage, m.logText = stage, "x\ny\n"
		m = press(t, m, "y")
		if m.copy == nil {
			t.Fatalf("%s: no copy mode", stage)
		}
	}
}

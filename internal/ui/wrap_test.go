package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func wrapLog() string {
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		switch i {
		case 30:
			b.WriteString("ERROR " + strings.Repeat("payload-segment ", 30) + "END\n")
		case 31:
			b.WriteString("\tat org.apache.kafka.clients.consumer.internals.ConsumerCoordinator.onJoinComplete(ConsumerCoordinator.java:412) and more frames\n")
		default:
			fmt.Fprintf(&b, "line %d\n", i)
		}
	}
	return b.String()
}

func TestWrapToggleInLogs(t *testing.T) {
	trueColor(t)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {57, 20}} {
		m := v4Model(t, v4Cluster())
		n, c := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = settle(t, n.(model), c)
		m = showLog(m, wrapLog())
		m.logStick, m.logAt = false, 27
		off := ansi.Strip(m.View())
		assertPaneWidths(t, m.View(), size[0])
		if strings.Contains(off, "END") || (size[0] == 120 && !strings.Contains(off, "w wrap off")) {
			t.Fatalf("%v: wrap off should cut the long line and say so:\n%s", size, off)
		}
		m = pressSettle(t, m, "w")
		if !m.wrapping() {
			t.Fatal("w did not turn wrap on")
		}
		on := ansi.Strip(m.View())
		assertPaneWidths(t, m.View(), size[0])
		if (size[0] >= 80 && !strings.Contains(on, "END")) || (size[0] == 120 && !strings.Contains(on, "w wrap on")) {
			t.Fatalf("%v: wrap on should show the whole line:\n%s", size, on)
		}
		if !strings.Contains(on, "line 28") || strings.Contains(on, "line 27") {
			t.Fatalf("%v: toggling should keep line 28 on top:\n%s", size, on)
		}

		lines := m.visualLines(m.viewerWidth())
		for i, l := range lines {
			if l.num == 30 {
				if lines[i+1].num != -1 || !strings.HasPrefix(lines[i+1].text, "        ") {
					t.Fatalf("continuation of the frame %q is not indented", lines[i+1].text)
				}
			}
			if w := ansi.StringWidth(l.text); w > logRoom(m.viewerWidth(), 60) {
				t.Fatalf("row %d is %d wide, room %d", i, w, logRoom(m.viewerWidth(), 60))
			}
		}

		m = pressSettle(t, m, "G")
		if s := ansi.Strip(m.View()); !strings.Contains(s, "line 60") {
			t.Fatalf("G with wrap on does not reach the end:\n%s", s)
		}
		m = pressSettle(t, m, "g")
		if s := ansi.Strip(m.View()); !strings.Contains(s, "  1 line 1") {
			t.Fatalf("g with wrap on does not reach the top:\n%s", s)
		}
		m = pressSettle(t, m, "w")
		if m.wrapping() {
			t.Fatal("second w did not turn wrap off")
		}
	}
}

func TestWrapConfigDefaultAndPerPane(t *testing.T) {
	m := v4Model(t, v4Cluster())
	m.cfg.UI.Wrap = true
	m = showLog(m, wrapLog())
	if !m.wrapping() {
		t.Fatal("ui.wrap: true should start wrapped")
	}
	m = pressSettle(t, m, "ctrl+a", "v")
	m = showLog(m, wrapLog())
	m = pressSettle(t, m, "w")
	if m.wrapping() {
		t.Fatal("w should turn wrap off in this pane")
	}
	assertPaneWidths(t, m.View(), m.width)
	m = pressSettle(t, m, "ctrl+a", "o")
	m = showLog(m, wrapLog())
	if !m.wrapping() {
		t.Fatal("the other pane keeps its own wrap state")
	}
}

func TestWrapAllViewers(t *testing.T) {
	trueColor(t)
	for _, stage := range []string{"describe", "yaml", "view", "kout"} {
		m := v4Model(t, v4Cluster())
		m.stage, m.logText = stage, "key: "+strings.Repeat("value ", 40)+"TAIL\n\t"+strings.Repeat("x", 300)+"\n"
		if strings.Contains(ansi.Strip(m.View()), "TAIL") {
			t.Fatalf("%s: wrap off shows the cut text", stage)
		}
		m = pressSettle(t, m, "w")
		s := ansi.Strip(m.View())
		assertPaneWidths(t, m.View(), m.width)
		if !strings.Contains(s, "TAIL") || !strings.Contains(s, "wrap on") {
			t.Fatalf("%s: wrap on should show the end of the line:\n%s", stage, s)
		}
	}
	old := clipboardRun
	t.Cleanup(func() { clipboardRun = old })
	copied := ""
	clipboardRun = func(mode, text string) (string, error) { copied = text; return "test", nil }
	m := v4Model(t, v4Cluster())
	m = pressSettle(t, m, "w")
	if copied == "" {
		t.Fatal("w in a table did not copy")
	}
	if m.wrapping() {
		t.Fatal("w in a table should copy, not toggle wrap")
	}
}

func TestWrapLine(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  []string
	}{
		{"short", 10, []string{"short"}},
		{"aaaa bbbb cccc", 10, []string{"aaaa bbbb", "cccc"}},
		{"abcdefghijkl", 5, []string{"abcde", "fghij", "kl"}},
		{"  ab cd ef gh", 8, []string{"  ab cd", "  ef gh"}},
		{"\tx " + strings.Repeat("y", 12), 12, []string{"        x", "      yyyyyy", "      yyyyyy"}},
		{"日本語日本語", 5, []string{"日本", "語日", "本語"}},
	}
	for _, c := range cases {
		got := wrapLine(c.in, c.width)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapLine(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
		for _, r := range got {
			if ansi.StringWidth(r) > c.width {
				t.Errorf("wrapLine(%q): row %q wider than %d", c.in, r, c.width)
			}
		}
	}

	rows := wrapLine("\x1b[31m"+strings.Repeat("r", 10)+"\x1b[0m", 4)
	if len(rows) != 3 || ansi.Strip(strings.Join(rows, "")) != strings.Repeat("r", 10) {
		t.Fatalf("colored wrap: %q", rows)
	}
}

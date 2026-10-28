package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func fourPanes(t *testing.T, m model) (model, [4]string) {
	t.Helper()
	a := m.logID
	m = press(t, m, "ctrl+a", "h")
	d := m.logID
	m = press(t, m, "ctrl+up", "ctrl+a", "v")
	b := m.logID
	m = press(t, m, "ctrl+a", "h")
	c := m.logID
	m = m.endRepeat(repeatTimeoutMsg{tok: m.repeatTok})
	return m, [4]string{a, b, c, d}
}

func TestPrefixArrowsMoveFocus(t *testing.T) {
	m, p := fourPanes(t, newTestModel(t, testEnv(t), nil))
	a, b, c, d := p[0], p[1], p[2], p[3]
	steps := []struct{ key, want string }{
		{"left", a}, {"right", b}, {"down", c}, {"down", d}, {"up", a}, {"down", d},
	}
	for i, s := range steps {

		m = m.endRepeat(repeatTimeoutMsg{tok: m.repeatTok})
		m = press(t, m, "ctrl+a", s.key)
		if m.logID != s.want || !m.repeatOn || m.prefixOn {
			t.Fatalf("step %d ctrl+a %s: focus %s want %s (repeat %v)", i, s.key, m.logID, s.want, m.repeatOn)
		}
	}

	cfg := testEnv(t)
	cfg.Keys.Prefix = []string{"ctrl+k"}
	m, p = fourPanes(t, newTestModel(t, cfg, nil))
	_ = p
	m = press(t, m, "ctrl+k", "left")
	if m.logID != p[0] {
		t.Fatalf("ctrl+k left: focus %s want %s", m.logID, p[0])
	}
}

func TestPrefixArrowRepeat(t *testing.T) {
	m, p := fourPanes(t, newTestModel(t, testEnv(t), nil))
	a, b, c, d := p[0], p[1], p[2], p[3]
	m = press(t, m, "ctrl+a", "left")
	if m.logID != a {
		t.Fatal("ctrl+a left")
	}
	first := m.repeatTok
	m = press(t, m, "right", "down")
	if m.logID != c || !m.repeatOn {
		t.Fatalf("repeat: focus %s want %s", m.logID, c)
	}
	m = m.endRepeat(repeatTimeoutMsg{tok: first})
	if !m.repeatOn {
		t.Fatal("an old timeout closed the renewed window")
	}
	if l := ansi.Strip(m.legend()); !strings.Contains(l, "repeat") || !strings.Contains(l, "←↓↑→ move pane") {
		t.Fatalf("repeat legend %q", l)
	}
	m = press(t, m, "down")
	m = m.endRepeat(repeatTimeoutMsg{tok: m.repeatTok})
	if m.repeatOn || m.logID != d {
		t.Fatalf("window still open or focus %s", m.logID)
	}
	m = press(t, m, "up")
	if m.logID != d {
		t.Fatal("an arrow after the window moved the focus")
	}
	_ = b
}

func TestPrefixArrowRepeatEndsOnOtherKey(t *testing.T) {
	m, p := fourPanes(t, newTestModel(t, testEnv(t), nil))
	m = press(t, m, "ctrl+a", "down")
	m = press(t, m, ":")
	if m.repeatOn || !m.cmdOn || m.logID != p[3] {
		t.Fatalf(": after the window: repeat %v cmd %v", m.repeatOn, m.cmdOn)
	}
	m = press(t, m, "esc")

	m = actionModel(t, actionCluster())
	m = press(t, m, "ctrl+a", "v")
	m = press(t, m, "ctrl+a", "left")
	at := m.content.Index()
	m = press(t, m, "j")
	if m.repeatOn || m.content.Index() != at+1 || m.stage != "workpods" {
		t.Fatalf("j after the window: repeat %v index %d → %d", m.repeatOn, at, m.content.Index())
	}

	m = press(t, m, "ctrl+a", "right", "ctrl+a")
	if m.repeatOn || !m.prefixOn {
		t.Fatalf("prefix in the window: repeat %v prefix %v", m.repeatOn, m.prefixOn)
	}
	if l := ansi.Strip(m.prefixLegend()); !strings.Contains(l, "←↓↑→ move") {
		t.Fatalf("prefix legend %q", l)
	}
}

func TestPrefixArrowSkipsSidebar(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, "ctrl+a", "v", "ctrl+a", "left")
	left := m.logID
	m = press(t, m, "left", "left", "up", "down")
	if m.logID != left || m.focus < 0 || m.focus >= len(m.panes) {
		t.Fatalf("focus left the panes: %s (focus %d)", m.logID, m.focus)
	}

	m = newTestModel(t, testEnv(t), nil)
	m = press(t, m, "ctrl+a", "left")
	if m.repeatOn {
		t.Fatal("repeat window with a single pane")
	}
}

func TestPrefixArrowsInEditor(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := newEdHarness(t, m)
	h.keys("ctrl+a", "v")
	startCat(t, h)
	right := h.m.logID
	h.keys("left")
	if h.m.logID != right || h.m.ed == nil {
		t.Fatal("a plain arrow left the editor")
	}
	h.keys("ctrl+a", "left")
	if h.m.logID == right || h.m.ed != nil || !h.m.repeatOn {
		t.Fatal("ctrl+a left did not leave the editor pane")
	}
	h.keys("right")
	if h.m.logID != right || !h.m.ed.live() {
		t.Fatal("repeat right did not return to the editor")
	}
	h.keys("right")
	h.m = h.m.endRepeat(repeatTimeoutMsg{tok: h.m.repeatTok})
	h.typeText("abc")
	h.keys("left", "left")
	if h.m.logID != right || h.m.ed == nil || h.m.repeatOn {
		t.Fatal("arrows after the window did not go to the editor")
	}
	h.pump(func() bool { return strings.Contains(h.screen(), "abc") }, "cat to echo")

	h.keys("ctrl+a", "left", "right", "x")
	h.typeText("yz")
	h.pump(func() bool { return strings.Contains(h.screen(), "xyz") }, "x after the window to reach cat")
}

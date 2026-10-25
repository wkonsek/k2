package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"k2/internal/config"
	"k2/internal/shell"
)

func testEnv(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")
	cfg := config.Default()
	cfg.StateDir = t.TempDir()
	return cfg
}

func newTestModel(t *testing.T, cfg config.Config, f *shell.Fake, warnings ...string) model {
	t.Helper()
	if f == nil {
		f = &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.Fail(1, "no cluster in tests") }}
	}
	m := initialModel(Options{Runner: f, Config: &cfg, Warnings: warnings})
	m.width, m.height = 120, 40
	return m
}

func key(t *testing.T, s string) tea.KeyMsg {
	t.Helper()
	types := map[string]tea.KeyType{
		"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc,
		"ctrl+x": tea.KeyCtrlX, "ctrl+n": tea.KeyCtrlN, "ctrl+v": tea.KeyCtrlV, "ctrl+o": tea.KeyCtrlO, "ctrl+j": tea.KeyCtrlJ,
		"ctrl+d": tea.KeyCtrlD, "ctrl+c": tea.KeyCtrlC, "ctrl+\\": tea.KeyCtrlBackslash,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"shift+tab": tea.KeyShiftTab, "backspace": tea.KeyBackspace, "pgdown": tea.KeyPgDown, "pgup": tea.KeyPgUp,
		"ctrl+up": tea.KeyCtrlUp, "ctrl+down": tea.KeyCtrlDown, "ctrl+left": tea.KeyCtrlLeft, "ctrl+right": tea.KeyCtrlRight,
	}
	k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	switch {
	case types[s] != 0:
		k = tea.KeyMsg{Type: types[s]}
	case len(s) == 6 && strings.HasPrefix(s, "ctrl+") && s[5] >= 'a' && s[5] <= 'z':
		k = tea.KeyMsg{Type: tea.KeyCtrlA + tea.KeyType(s[5]-'a')}
	case strings.HasPrefix(s, "alt+") && len([]rune(s)) == 5:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s[4:]), Alt: true}
	case len([]rune(s)) > 1 && strings.Contains(s, "+"):
		t.Fatalf("key(%q): unknown key", s)
	}
	if k.String() != s {
		t.Fatalf("key(%q) produced %q", s, k.String())
	}
	return k
}

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(key(t, k))
		m = next.(model)
	}
	return m
}

func sgr(c lipgloss.Color) string {
	out := lipgloss.NewStyle().Foreground(c).Render("x")
	return out[:strings.Index(out, "x")]
}

func colorBefore(s, marker string) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	head := s[:i]
	j := strings.LastIndex(head, "\x1b[")
	if j < 0 {
		return ""
	}
	return head[j:]
}

func settle(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 200 {
			t.Fatal("settle: too many steps")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(300 * time.Millisecond):
			continue
		}
		switch msg := msg.(type) {
		case nil, spinner.TickMsg, toastExpiredMsg, tickMsg:
			continue
		case tea.BatchMsg:
			queue = append(queue, msg...)
			continue
		default:
			next, more := m.Update(msg)
			m = next.(model)
			queue = append(queue, more)
		}
	}
	return m
}

func pressSettle(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(key(t, k))
		m = settle(t, next.(model), cmd)
	}
	return m
}

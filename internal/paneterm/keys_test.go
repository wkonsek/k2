package paneterm

import (
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"
)

func TestKeyBytes(t *testing.T) {
	cases := []struct {
		k    tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}, "é"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, "\x1bx"},
		{tea.KeyMsg{Type: tea.KeyEscape}, "\x1b"},
		{tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{tea.KeyMsg{Type: tea.KeyTab}, "\t"},
		{tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{tea.KeyMsg{Type: tea.KeyCtrlW}, "\x17"},
		{tea.KeyMsg{Type: tea.KeyCtrlV}, "\x16"},
		{tea.KeyMsg{Type: tea.KeyCtrlBackslash}, "\x1c"},
		{tea.KeyMsg{Type: tea.KeySpace}, " "},
		{tea.KeyMsg{Type: tea.KeyF5}, "\x1b[15~"},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, "\x1b[Z"},
		{tea.KeyMsg{Type: tea.KeyCtrlLeft}, "\x1b[1;5D"},
		{tea.KeyMsg{Type: tea.KeyDelete}, "\x1b[3~"},
	}
	for _, c := range cases {
		got, ok := keyBytes(c.k)
		if !ok || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.k, got, ok, c.want)
		}
	}
}

func TestArrowsFollowCursorKeyMode(t *testing.T) {
	e := vt.NewEmulator(10, 3)
	read := func(k tea.KeyMsg) string {
		ev, ok := uvKey(k)
		if !ok {
			t.Fatal("not an arrow")
		}
		ch := make(chan string)
		go func() { b := make([]byte, 16); n, _ := e.Read(b); ch <- string(b[:n]) }()
		e.SendKey(ev)
		return <-ch
	}
	if got := read(tea.KeyMsg{Type: tea.KeyUp}); got != "\x1b[A" {
		t.Fatalf("normal mode up = %q", got)
	}
	_, _ = io.WriteString(e, "\x1b[?1h")
	if got := read(tea.KeyMsg{Type: tea.KeyUp}); got != "\x1bOA" {
		t.Fatalf("application mode up = %q", got)
	}
}

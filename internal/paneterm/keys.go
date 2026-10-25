package paneterm

import (
	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

func uvKey(k tea.KeyMsg) (uv.KeyPressEvent, bool) {
	var code rune
	switch k.Type {
	case tea.KeyUp:
		code = uv.KeyUp
	case tea.KeyDown:
		code = uv.KeyDown
	case tea.KeyLeft:
		code = uv.KeyLeft
	case tea.KeyRight:
		code = uv.KeyRight
	default:
		return uv.KeyPressEvent{}, false
	}
	ev := uv.KeyPressEvent{Code: code}
	if k.Alt {
		ev.Mod |= uv.ModAlt
	}
	return ev, true
}

func keyBytes(k tea.KeyMsg) (string, bool) {
	s, ok := "", true
	switch {
	case k.Type == tea.KeyRunes:
		s = string(k.Runes)
	case k.Type == tea.KeySpace:
		s = " "
	case k.Type >= 0 && k.Type <= 31, k.Type == tea.KeyBackspace:

		s = string(rune(k.Type))
	default:
		s, ok = special[k.Type]
	}
	if !ok {
		return "", false
	}
	if k.Alt {
		s = "\x1b" + s
	}
	return s, true
}

var special = map[tea.KeyType]string{
	tea.KeyShiftTab: "\x1b[Z",
	tea.KeyHome:     "\x1b[H", tea.KeyEnd: "\x1b[F",
	tea.KeyPgUp: "\x1b[5~", tea.KeyPgDown: "\x1b[6~",
	tea.KeyInsert: "\x1b[2~", tea.KeyDelete: "\x1b[3~",
	tea.KeyCtrlUp: "\x1b[1;5A", tea.KeyCtrlDown: "\x1b[1;5B", tea.KeyCtrlRight: "\x1b[1;5C", tea.KeyCtrlLeft: "\x1b[1;5D",
	tea.KeyShiftUp: "\x1b[1;2A", tea.KeyShiftDown: "\x1b[1;2B", tea.KeyShiftRight: "\x1b[1;2C", tea.KeyShiftLeft: "\x1b[1;2D",
	tea.KeyCtrlShiftUp: "\x1b[1;6A", tea.KeyCtrlShiftDown: "\x1b[1;6B", tea.KeyCtrlShiftRight: "\x1b[1;6C", tea.KeyCtrlShiftLeft: "\x1b[1;6D",
	tea.KeyCtrlHome: "\x1b[1;5H", tea.KeyCtrlEnd: "\x1b[1;5F", tea.KeyShiftHome: "\x1b[1;2H", tea.KeyShiftEnd: "\x1b[1;2F",
	tea.KeyCtrlPgUp: "\x1b[5;5~", tea.KeyCtrlPgDown: "\x1b[6;5~",
	tea.KeyF1: "\x1bOP", tea.KeyF2: "\x1bOQ", tea.KeyF3: "\x1bOR", tea.KeyF4: "\x1bOS",
	tea.KeyF5: "\x1b[15~", tea.KeyF6: "\x1b[17~", tea.KeyF7: "\x1b[18~", tea.KeyF8: "\x1b[19~",
	tea.KeyF9: "\x1b[20~", tea.KeyF10: "\x1b[21~", tea.KeyF11: "\x1b[23~", tea.KeyF12: "\x1b[24~",
}

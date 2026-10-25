package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type toast struct {
	id   int
	text string
	tone tone
}

type toastExpiredMsg struct{ id int }

func (m *model) notify(t tone, text string) tea.Cmd {
	if !m.cfg.UI.Toasts || text == "" {
		return nil
	}
	if t == toneErr && m.freshErr() && !strings.HasSuffix(text, " details") {
		text += " · " + m.cfg.Keys.LastError.Short() + " details"
	}
	m.toastSeq++
	m.toast = toast{id: m.toastSeq, text: text, tone: t}
	id := m.toastSeq
	return tea.Tick(toastLife(t, m.cfg.UI.ToastSeconds), func(time.Time) tea.Msg {
		return toastExpiredMsg{id: id}
	})
}

const errToastMin = 5

func toastLife(t tone, seconds int) time.Duration {
	s := max(1, seconds)
	if t == toneErr {
		s = max(s, errToastMin)
	}
	return time.Duration(s) * time.Second
}

func (m *model) expireToast(msg toastExpiredMsg) {
	if msg.id == m.toast.id {
		m.toast = toast{}
	}
}

func toastIcon(t tone) string {
	switch t {
	case toneOK, toneDone:
		return "✓"
	case toneWarn:
		return "!"
	case toneErr:
		return "✗"
	default:
		return "•"
	}
}

func (m model) toastLabel(innerW int) string {
	if m.toast.text == "" || innerW < 12 {
		return ""
	}
	text := " " + toastIcon(m.toast.tone) + " " + m.toast.text + " "
	if room := innerW - 4; ansi.StringWidth(text) > room {
		text = ansi.Truncate(text, room-1, "…") + " "
	}
	st := toneStyle(m.toast.tone)
	if m.toast.tone == toneNormal {
		st = lipgloss.NewStyle().Foreground(th.Accent)
	}
	return st.Bold(true).Render(text)
}

type noticeMsg struct{ text string }

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func baseSeq() string {
	out := lipgloss.NewStyle().Background(th.Bg).Foreground(th.Text).Render("x")
	i := strings.Index(out, "x")
	if i <= 0 {
		return ""
	}
	return out[:i]
}

func paintBackground(screen string, width, height int) string {
	on := baseSeq()
	if on == "" {
		return screen
	}
	lines := strings.Split(screen, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\x1b[0m", "\x1b[0m"+on)
		l = strings.ReplaceAll(l, "\x1b[m", "\x1b[m"+on)
		l = strings.ReplaceAll(l, "\x1b[49m", on)
		if pad := width - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		lines[i] = on + l + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

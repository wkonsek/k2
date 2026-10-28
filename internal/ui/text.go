package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"
)

func wrapText(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	var out []string
	rs := []rune(s)
	for len(rs) > 0 {
		if ansi.StringWidth(string(rs)) <= width {
			out = append(out, string(rs))
			break
		}
		n, w := 0, 0
		for n < len(rs) {
			rw := ansi.StringWidth(string(rs[n]))
			if w+rw > width {
				break
			}
			w += rw
			n++
		}
		if n == 0 {
			n = 1
		}
		out = append(out, string(rs[:n]))
		rs = rs[n:]
	}
	return out
}

func padLines(s string, n int) string {
	if n < 1 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func padPlain(s string, width int) string {
	s = clipLine(s, width)
	w := ansi.StringWidth(s)
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

const markSquare = "■"

func iconSlot(s string) string {
	w := ansi.StringWidth(s)
	switch {
	case w <= 0:
		return "  "
	case w == 1:
		return s + " "
	default:
		return ansi.Truncate(s, 2, "")
	}
}

func clampWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

func clipLine(s string, width int) string {
	if width < 1 {
		return ""
	}
	s = sanitizeLine(s)
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func crop(s string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = clipLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func fitWidth(s string, n int) string {
	if n < 4 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func altLeak(msg tea.KeyMsg) bool { return msg.Type == tea.KeyRunes && msg.Alt }

func printable(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		return !msg.Alt
	case tea.KeySpace:
		return true
	}
	return false
}

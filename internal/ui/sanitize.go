package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const tabWidth = 8

func needsSanitize(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return true
		}
		if c == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f {
			return true
		}
	}
	return false
}

func sanitizeLine(s string) string {
	if !needsSanitize(s) {
		return s
	}

	if strings.Contains(s, "\r") {
		s = strings.TrimRight(s, "\r")
		if i := strings.LastIndexByte(s, '\r'); i >= 0 {
			s = s[i+1:]
		}
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	col := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\t':
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			i++
		case c == 0x1b:
			i += escapeSeq(&b, s[i:])
		case c < 0x20 || c == 0x7f:
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			if r >= 0x80 && r <= 0x9f {
				i += size
				continue
			}
			b.WriteString(s[i : i+size])
			col += ansi.StringWidth(s[i : i+size])
			i += size
		}
	}
	return b.String()
}

func escapeSeq(b *strings.Builder, s string) int {
	if len(s) < 2 {
		return 1
	}
	switch s[1] {
	case '[':
		for j := 2; j < len(s); j++ {
			c := s[j]
			if c >= 0x40 && c <= 0x7e {
				if c == 'm' && sgrParams(s[2:j]) {
					b.WriteString(s[:j+1])
				}
				return j + 1
			}
			if c < 0x20 || c > 0x3f {
				return j
			}
		}
		return len(s)
	case ']', 'P', '_', '^', 'X':
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	}

	j := 1
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	if j < len(s) {
		j++
	}
	return j
}

func sgrParams(p string) bool {
	for i := 0; i < len(p); i++ {
		if c := p[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

func sanitizeText(s string) string {
	if !needsSanitize(strings.ReplaceAll(s, "\n", "")) {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = sanitizeLine(l)
	}
	return strings.Join(lines, "\n")
}

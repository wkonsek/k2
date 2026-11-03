package ui

import (
	"strings"

	"k2/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const tagline = "kubectl, elevated"

func taglineRows(text string, inner int) []string {
	text = strings.Join(strings.Fields(sanitizeLine(text)), " ")
	switch {
	case text == "":
		return nil
	case ansi.StringWidth(text) <= inner:
		return []string{text}
	}
	best, bestW := -1, 1<<30
	for i, r := range text {
		if r != ' ' {
			continue
		}
		a, b := text[:i], text[i+1:]
		wa, wb := ansi.StringWidth(a), ansi.StringWidth(b)
		if wa <= inner && wb <= inner && max(wa, wb) < bestW {
			best, bestW = i, max(wa, wb)
		}
	}
	if best >= 0 {
		return []string{text[:best], text[best+1:]}
	}
	return wrapLine(text, max(inner, 2))
}

func logoTitle(color string) lipgloss.Style {
	if color == "accent" {
		return lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(th.OK).Bold(true)
}

var wordmark = []string{"█ ▄ ▀█", "█▀▄ █▄"}

func brandRows(cfg config.UI, inner int, big bool) []string {
	style := logoTitle(cfg.LogoTitleColor)
	var rows []string
	if big && ansi.StringWidth(wordmark[0]) <= inner {
		for _, row := range wordmark {
			rows = append(rows, style.Render(row))
		}
	} else {
		rows = append(rows, style.Render("k2"))
	}
	for _, t := range taglineRows(tagline, max(inner, 1)) {
		rows = append(rows, dimStyle.Render(t))
	}
	return rows
}

var mountain = []string{"      ▄", "     ▟█▙", "    ▟███▙", "  ▄▟█████▙▄", "▄▟█████████▙▄"}

var mountainMask = []string{"      S", "     SSS", "    AAAAA", "  BAAAAAAAB", "BBAAAAAAAAABB"}

func mountainWidth() int {
	w := 0
	for _, row := range mountain {
		w = max(w, ansi.StringWidth(row))
	}
	return w - mountainIndent()
}

func mountainIndent() int {
	n := 1 << 30
	for _, row := range mountain {
		n = min(n, len(row)-len(strings.TrimLeft(row, " ")))
	}
	return n
}

func paintMountain(i int) string {
	cut := mountainIndent()
	r, mk := []rune(mountain[i])[cut:], []rune(mountainMask[i])[cut:]
	var b strings.Builder
	for j := 0; j < len(r); {
		k := j + 1
		for k < len(r) && mk[k] == mk[j] {
			k++
		}
		run := string(r[j:k])
		switch mk[j] {
		case 'S':
			run = lipgloss.NewStyle().Foreground(th.Bright).Render(run)
		case 'A':
			run = lipgloss.NewStyle().Foreground(th.Accent).Render(run)
		case 'B':
			run = dimStyle.Render(run)
		}
		b.WriteString(run)
		j = k
	}
	return b.String()
}

func mountainRows(cfg config.UI, inner int) []string {
	mw := mountainWidth()
	tag := taglineRows(tagline, max(inner, 1))
	block := mw
	for _, t := range tag {
		block = max(block, ansi.StringWidth(t))
	}
	at := func(s string, w int) string {
		row := strings.Repeat(" ", (block-w)/2) + s
		if cfg.SidebarPosition == "right" {
			row = padPlain(row, block)
		}
		return row
	}
	var rows []string
	for i := range mountain {
		rows = append(rows, at(paintMountain(i), mw))
	}
	rows = append(rows, "", at(logoTitle(cfg.LogoTitleColor).Render("k2"), 2))
	for _, t := range tag {
		rows = append(rows, at(dimStyle.Render(t), ansi.StringWidth(t)))
	}
	return rows
}

func (m model) mark(inner, level int) string {
	if level >= 2 && m.cfg.UI.Logo == "mountain" && mountainWidth() <= inner {
		return strings.Join(mountainRows(m.cfg.UI, inner), "\n") + "\n\n"
	}
	return strings.Join(brandRows(m.cfg.UI, inner, level >= 1), "\n") + "\n\n"
}

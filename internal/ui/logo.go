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

var wordmark = []string{"█ ▄ ▀▀▄", "█▀▄ ▄▄▀"}

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

func (m model) mark(inner int, big bool) string {
	return strings.Join(brandRows(m.cfg.UI, inner, big), "\n") + "\n\n"
}

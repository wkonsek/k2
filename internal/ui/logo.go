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

var mountain = []string{"    ▄█▄    ", "  ▄█████▄  ", "▄█████████▄"}

var mountainMask = []string{"    SSS    ", "  ASSSSSA  ", "BAAAAAAAAAB"}

func paintMountain(i int) string {
	r, mk := []rune(mountain[i]), []rune(mountainMask[i])
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

const lockIcon = ""

func readOnlyMark(full bool) string {
	if full {
		return warnStyle.Render(lockIcon + " READ-ONLY")
	}
	return warnStyle.Render("READ-ONLY")
}

func decorateReadOnly(rows []string, on bool, inner int) []string {
	if !on || len(rows) == 0 {
		return rows
	}
	full, short := readOnlyMark(true), readOnlyMark(false)
	icon := warnStyle.Render(lockIcon)
	tryAttach := func(mark string) ([]string, bool) {
		for i := range rows {
			cand := rows[i] + " " + mark
			if ansi.StringWidth(ansi.Strip(cand)) <= inner {
				out := append([]string(nil), rows...)
				out[i] = cand
				return out, true
			}
		}
		return nil, false
	}
	if out, ok := tryAttach(full); ok {
		return out
	}
	if ansi.StringWidth(ansi.Strip(full)) <= inner {
		return append(append([]string(nil), rows...), full)
	}
	if out, ok := tryAttach(short); ok {
		return out
	}
	if ansi.StringWidth(ansi.Strip(short)) <= inner {
		return append(append([]string(nil), rows...), short)
	}
	if out, ok := tryAttach(icon); ok {
		return out
	}
	if ansi.StringWidth(ansi.Strip(icon)) <= inner {
		return append(append([]string(nil), rows...), icon)
	}
	return rows
}

func mountainRows(cfg config.UI, inner int, lock bool) []string {
	var rows []string
	for i := range mountain {
		row := paintMountain(i)
		if cfg.SidebarPosition != "right" {
			row = strings.TrimRight(row, " ")
		}
		rows = append(rows, row)
	}
	rows = append(rows, logoTitle(cfg.LogoTitleColor).Render("k2"))
	for _, t := range taglineRows(tagline, max(inner, 1)) {
		rows = append(rows, dimStyle.Render(t))
	}
	return decorateReadOnly(rows, lock, inner)
}

func brandRowsLocked(cfg config.UI, inner int, big, lock bool) []string {
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
	return decorateReadOnly(rows, lock, inner)
}

func logoRows(cfg config.UI, inner, room int, big, lock bool) []string {
	if cfg.Logo == "mountain" && ansi.StringWidth(mountain[0]) <= inner {
		if rows := mountainRows(cfg, inner, lock); len(rows) <= room {
			return rows
		}
	}
	return brandRowsLocked(cfg, inner, big, lock)
}

func (m model) mark(inner, level int) string {
	if level >= 2 && m.cfg.UI.Logo == "mountain" && ansi.StringWidth(mountain[0]) <= inner {
		return strings.Join(mountainRows(m.cfg.UI, inner, m.readOnly), "\n") + "\n\n"
	}
	return strings.Join(brandRowsLocked(m.cfg.UI, inner, level >= 1, m.readOnly), "\n") + "\n\n"
}

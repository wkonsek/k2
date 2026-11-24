package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"k2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const tagline = "kubectl, elevated"

const (
	twinklePeriod = 13
	twinkleHold   = 3
	twinkleStride = 5
	twinkleStep   = time.Second
)

type twinkleMsg struct{ seq int }

func twinkleTick(seq int) tea.Cmd {
	return tea.Tick(twinkleStep, func(time.Time) tea.Msg { return twinkleMsg{seq} })
}

func (m model) twinkleOn() bool {
	return m.cfg.UI.Animations && m.cfg.UI.LogoTwinkle && !m.zoom
}

func (m model) logoFrame() int {
	if !m.twinkleOn() {
		return 0
	}
	return m.twinkle
}

func (m *model) twinkleStep(msg twinkleMsg) tea.Cmd {
	if msg.seq != m.twinkleSeq {
		return nil
	}
	if !m.twinkleOn() {
		m.twinkleAt = time.Time{}
		return nil
	}
	m.twinkle++
	m.twinkleAt = now()
	return twinkleTick(m.twinkleSeq)
}

func (m *model) twinkleStart() tea.Cmd {
	if now().Sub(m.twinkleAt) < time.Second || !m.twinkleOn() {
		return nil
	}
	m.twinkleSeq++
	m.twinkleAt = now()
	return twinkleTick(m.twinkleSeq)
}

func twinkleLevel(frame, idx int) int {
	if frame <= 0 {
		return 0
	}
	phase := (frame + idx*twinkleStride) % twinklePeriod
	switch phase {
	case 0, 2:
		return 1
	case 1:
		return 2
	default:
		return 0
	}
}

func twinkleAny(frame, n int) bool {
	if frame <= 0 || n < 1 {
		return false
	}
	for i := 0; i < n; i++ {
		if twinkleLevel(frame, i) > 0 {
			return true
		}
	}
	return false
}

func parseHex(s string) (r, g, b int, ok bool) {
	s = strings.TrimPrefix(strings.ToLower(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16), int((v >> 8) & 0xff), int(v & 0xff), true
}

func mixHex(a, b lipgloss.Color, num, den int) lipgloss.Color {
	ar, ag, ab, ok1 := parseHex(string(a))
	br, bg, bb, ok2 := parseHex(string(b))
	if !ok1 || !ok2 || den == 0 {
		if den != 0 && num*2 >= den {
			return b
		}
		return a
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", ar+(br-ar)*num/den, ag+(bg-ag)*num/den, ab+(bb-ab)*num/den))
}

func twinkleColor(base, bright lipgloss.Color, level int) lipgloss.Color {
	switch level {
	case 1:
		return mixHex(base, bright, 1, 2)
	case 2:
		return bright
	default:
		return base
	}
}

func glyphHalves(ch rune) (top, bot bool) {
	return ch == '█' || ch == '▀', ch == '█' || ch == '▄'
}

func paintCell(ch rune, topLv, botLv int, base, bright lipgloss.Color, style lipgloss.Style) string {
	topOn, botOn := glyphHalves(ch)
	if !topOn && !botOn {
		return " "
	}
	topC, botC := twinkleColor(base, bright, topLv), twinkleColor(base, bright, botLv)
	switch {
	case topOn && botOn && topC == botC:
		return style.Foreground(topC).Render("█")
	case topOn && botOn:
		return style.Foreground(topC).Background(botC).Render("▀")
	case topOn:
		return style.Foreground(topC).Render("▀")
	default:
		return style.Foreground(botC).Render("▄")
	}
}

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

func logoBase(color string) lipgloss.Color {
	if color == "accent" {
		return th.Accent
	}
	return th.OK
}

func logoTitle(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(logoBase(color)).Bold(true)
}

var wordmark = []string{"█ ▄ ▀█", "█▀▄ █▄"}

func wordmarkPixels() int {
	n := 0
	for _, row := range wordmark {
		for _, ch := range row {
			top, bot := glyphHalves(ch)
			if top {
				n++
			}
			if bot {
				n++
			}
		}
	}
	return n
}

func paintWordmark(style lipgloss.Style, base, bright lipgloss.Color, frame int) []string {
	if !twinkleAny(frame, wordmarkPixels()) {
		out := make([]string, len(wordmark))
		for i, row := range wordmark {
			out[i] = style.Render(row)
		}
		return out
	}
	out := make([]string, len(wordmark))
	idx := 0
	for i, row := range wordmark {
		var b strings.Builder
		for _, ch := range row {
			top, bot := glyphHalves(ch)
			topLv, botLv := 0, 0
			if top {
				topLv = twinkleLevel(frame, idx)
				idx++
			}
			if bot {
				botLv = twinkleLevel(frame, idx)
				idx++
			}
			b.WriteString(paintCell(ch, topLv, botLv, base, bright, style))
		}
		out[i] = b.String()
	}
	return out
}

func brandRows(cfg config.UI, inner int, big bool) []string {
	return brandRowsLocked(cfg, inner, big, false, 0)
}

func brandRowsLocked(cfg config.UI, inner int, big, lock bool, frame int) []string {
	style := logoTitle(cfg.LogoTitleColor)
	var rows []string
	if big && ansi.StringWidth(wordmark[0]) <= inner {
		rows = append(rows, paintWordmark(style, logoBase(cfg.LogoTitleColor), th.Bright, frame)...)
	} else {
		rows = append(rows, style.Render("k2"))
	}
	for _, t := range taglineRows(tagline, max(inner, 1)) {
		rows = append(rows, dimStyle.Render(t))
	}
	return decorateReadOnly(rows, lock, inner)
}

var mountain = []string{"    ▄█▄    ", "  ▄█████▄  ", "▄█████████▄"}

var mountainMask = []string{"    SSS    ", "  ASSSSSA  ", "BAAAAAAAAAB"}

func mountainColor(kind rune) lipgloss.Color {
	switch kind {
	case 'S':
		return th.Bright
	case 'A':
		return th.Accent
	case 'B':
		return th.Muted
	default:
		return ""
	}
}

func mountainStyle(kind rune) lipgloss.Style {
	if kind == 'B' {
		return dimStyle
	}
	c := mountainColor(kind)
	if c == "" {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(c)
}

func mountainTwinkleN() int {
	n := 0
	for i, row := range mountain {
		mk := []rune(mountainMask[i])
		for j, ch := range []rune(row) {
			base := mountainColor(mk[j])
			if base == "" || base == th.Bright {
				continue
			}
			top, bot := glyphHalves(ch)
			if top {
				n++
			}
			if bot {
				n++
			}
		}
	}
	return n
}

func paintMountainPlain(i int) string {
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

func paintMountain(i int, frame int) string {
	if !twinkleAny(frame, mountainTwinkleN()) {
		return paintMountainPlain(i)
	}
	r, mk := []rune(mountain[i]), []rune(mountainMask[i])
	idx := 0
	for row := 0; row < i; row++ {
		prev := []rune(mountain[row])
		prevMk := []rune(mountainMask[row])
		for j, ch := range prev {
			base := mountainColor(prevMk[j])
			if base == "" || base == th.Bright {
				continue
			}
			top, bot := glyphHalves(ch)
			if top {
				idx++
			}
			if bot {
				idx++
			}
		}
	}
	var b strings.Builder
	for j, ch := range r {
		base := mountainColor(mk[j])
		top, bot := glyphHalves(ch)
		if base == "" || base == th.Bright || (!top && !bot) {
			if ch == ' ' {
				b.WriteRune(' ')
				continue
			}
			b.WriteString(mountainStyle(mk[j]).Render(string(ch)))
			continue
		}
		topLv, botLv := 0, 0
		if top {
			topLv = twinkleLevel(frame, idx)
			idx++
		}
		if bot {
			botLv = twinkleLevel(frame, idx)
			idx++
		}
		b.WriteString(paintCell(ch, topLv, botLv, base, th.Bright, mountainStyle(mk[j])))
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

func mountainRows(cfg config.UI, inner int, lock bool, frame int) []string {
	var rows []string
	for i := range mountain {
		row := paintMountain(i, frame)
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

func logoRows(cfg config.UI, inner, room int, big, lock bool, frame int) []string {
	if cfg.Logo == "mountain" && ansi.StringWidth(mountain[0]) <= inner {
		if rows := mountainRows(cfg, inner, lock, frame); len(rows) <= room {
			return rows
		}
	}
	return brandRowsLocked(cfg, inner, big, lock, frame)
}

func (m model) mark(inner, level int) string {
	if level >= 2 && m.cfg.UI.Logo == "mountain" && ansi.StringWidth(mountain[0]) <= inner {
		return strings.Join(mountainRows(m.cfg.UI, inner, m.readOnly, m.logoFrame()), "\n") + "\n\n"
	}
	return strings.Join(brandRowsLocked(m.cfg.UI, inner, level >= 1, m.readOnly, m.logoFrame()), "\n") + "\n\n"
}

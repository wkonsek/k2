package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const meterBar = 5

const (
	meterMediumMax = meterBar + 1 + 5
	meterValueMax  = 7
)

type meter struct {
	ok     bool
	pct    float64
	approx bool
	tone   tone
	valW   int
	pctW   int
}

func newMeter(used, limit, request string, parse func(string) (float64, bool), warn, crit int) meter {
	u, ok := parse(used)
	if !ok {
		return meter{}
	}
	base, approx := limit, false
	if base == "" {
		base, approx = request, true
	}
	b, err := strconv.ParseFloat(base, 64)
	if base == "" || err != nil || b <= 0 {
		return meter{}
	}
	pct := u / b * 100
	return meter{ok: true, pct: pct, approx: approx, tone: thresholdTone(pct, warn, crit)}
}

func thresholdTone(pct float64, warn, crit int) tone {
	switch {
	case pct >= float64(crit):
		return toneErr
	case pct >= float64(warn):
		return toneWarn
	}
	return toneOK
}

func (me meter) pctText() string {
	s := strconv.Itoa(int(math.Round(me.pct))) + "%"
	if me.approx {
		s = "~" + s
	}
	return s
}

func (me meter) full() int   { return me.valW + 1 + meterBar + 1 + me.pctW }
func (me meter) medium() int { return meterBar + 1 + me.pctW }

func (me meter) bar() string {
	filled := int(math.Round(me.pct / 100 * meterBar))
	filled = max(0, min(meterBar, filled))
	if me.pct > 0 && filled == 0 {
		filled = 1
	}
	return toneStyle(me.tone).Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", meterBar-filled))
}

func (me meter) cell(value string, w int) (string, tone, bool) {
	pct := fmt.Sprintf("%*s", me.pctW, me.pctText())
	switch {
	case w >= me.full():
		return fmt.Sprintf("%-*s ", me.valW, value) + me.bar() + " " + toneStyle(me.tone).Render(pct), 0, true
	case w >= me.medium():
		return me.bar() + " " + toneStyle(me.tone).Render(pct), 0, true
	}
	if me.tone == toneWarn || me.tone == toneErr {
		return value, me.tone, true
	}
	return value, 0, true
}

func lineUp(rows []map[int]meter, values []string, col int) int {
	valW, pctW, any := 0, 0, false
	for i, r := range rows {
		me, ok := r[col]
		if !ok || !me.ok {
			continue
		}
		any = true
		valW = max(valW, ansi.StringWidth(values[i]))
		pctW = max(pctW, len(me.pctText()))
	}
	if !any {
		return 0
	}
	for _, r := range rows {
		if me, ok := r[col]; ok && me.ok {
			me.valW, me.pctW = valW, pctW
			r[col] = me
		}
	}
	return valW + 1 + meterBar + 1 + pctW
}

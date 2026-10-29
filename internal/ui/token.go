package ui

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

type tokenState struct {
	key     string
	ctx     string
	fp      string
	life    kube.TokenLife
	oshift  bool
	seen    time.Time
	loading bool
	at      time.Time
}

type tokenMsg struct {
	key, fp string
	life    kube.TokenLife
	oshift  bool
}

const (
	tokenStep     = 10 * time.Second
	tokenFineStep = time.Second
	tokenFine     = 15 * time.Minute
	tokenWarnLeft = time.Hour
	tokenBarMax   = 12
)

func fileStamp(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s@%d/%d;", p, st.ModTime().UnixNano(), st.Size())
		} else {
			b.WriteString(p + "@-;")
		}
	}
	return b.String()
}

func (m *model) tokenCmd() tea.Cmd {
	if !m.cfg.UI.TokenBar || m.tok.loading {
		return nil
	}
	ctx := infoKey(m.kubeconfig, m.context)
	key := ctx + "\x00" + fileStamp(kube.KubeconfigPaths(m.kubeconfig))
	if key == m.tok.key {
		return nil
	}
	if ctx != m.tok.ctx {

		m.tok = tokenState{}
	}
	prev := m.tok
	m.tok.key, m.tok.ctx, m.tok.loading = key, ctx, true
	k, kc, context := m.k, m.kubeconfig, m.context
	return func() tea.Msg {
		ch := make(chan tokenMsg, 1)
		go func() {
			t := kube.TokenOf(kc, context)
			fp := t.Fingerprint()
			if fp != "" && fp == prev.fp {

				ch <- tokenMsg{key: key, fp: fp, life: prev.life, oshift: prev.oshift}
				return
			}
			ch <- tokenMsg{key: key, fp: fp, life: k.Life(kc, context, t), oshift: t.OpenShift()}
		}()
		select {
		case r := <-ch:
			return r
		case <-time.After(2*kube.InfoTimeout + infoGrace):
			return tokenMsg{key: key}
		}
	}
}

func (m *model) applyToken(msg tokenMsg) {
	m.tok.loading = false
	if msg.key != m.tok.key {
		m.tok.key = ""
		return
	}
	if msg.fp != m.tok.fp {
		m.tok.seen = now()
	}
	m.tok.fp, m.tok.life, m.tok.oshift = msg.fp, msg.life, msg.oshift
	m.tok.at = now()
}

func (m *model) tokenClock() {
	if !m.tok.life.Known {
		return
	}
	t := now()
	step := tokenStep
	if m.tok.life.End.Sub(t) <= tokenFine+tokenStep {
		step = tokenFineStep
	}
	if m.tok.at.IsZero() || t.Sub(m.tok.at) >= step || t.Before(m.tok.at) {
		m.tok.at = t
	}
}

func (m model) tokenRows(inner int) string {
	life := m.tok.life
	if !m.cfg.UI.TokenBar || m.tok.ctx != infoKey(m.kubeconfig, m.context) {
		return ""
	}

	if life.Invalid {
		return m.tokenProblemRow(inner, "invalid")
	}
	if life.Expired && !life.Known {
		return m.tokenProblemRow(inner, "expired")
	}
	if !life.Known {
		return ""
	}
	at := m.tok.at
	if at.IsZero() {
		at = now()
	}
	left := life.End.Sub(at)
	if left <= 0 {
		return m.tokenProblemRow(inner, "expired")
	}
	start := life.Start
	if start.IsZero() || !start.Before(life.End) {
		start = m.tok.seen
	}
	total := life.End.Sub(start)
	frac := 1.0
	if total > 0 {
		frac = math.Min(1, math.Max(0, float64(left)/float64(total)))
	}
	style := okStyle
	switch pct := frac * 100; {
	case pct < float64(m.cfg.UI.TokenCrit) || left < tokenFine:
		style = errStyle
	case pct < float64(m.cfg.UI.TokenWarn) || left < tokenWarnLeft:
		style = warnStyle
	}
	text := tokenLeft(left)
	label := dimStyle.Render(padRight("token", infoPad))
	row := label
	if w := min(tokenBarMax, inner-infoPad-1-len(text)); w >= 4 {
		full, part, empty := tokenBar(frac, w)
		row += style.Render(full+part) + dimStyle.Render(empty) + " "
	}
	row += style.Render(text)
	out := clipLine(row, inner) + "\n"
	if life.Idle > 0 {

		out += clipLine(dimStyle.Render(padRight("", infoPad)+"idle limit "+tokenLeft(life.Idle)), inner) + "\n"
	}
	return out
}

func (m model) tokenProblemRow(inner int, word string) string {
	label := errStyle.Bold(true).Render(word)
	hint := ":login"
	if s := m.cfg.Keys.Login.Short(); s != "" {
		hint = s + " or :login"
	}
	var b strings.Builder
	b.WriteString(clipLine(dimStyle.Render(padRight("token", infoPad))+label, inner) + "\n")
	pad := infoPad
	if pad >= inner {
		pad = 0
	}
	for _, part := range wrapText(hint, max(inner-pad, 1)) {
		b.WriteString(clipLine(strings.Repeat(" ", pad)+dimStyle.Render(part), inner) + "\n")
	}
	return b.String()
}

var tokenEighths = []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"}

func tokenBar(frac float64, w int) (full, part, empty string) {
	cells := frac * float64(w)
	n := int(cells)
	if n >= w {
		return strings.Repeat("█", w), "", ""
	}
	rest := w - n
	if e := int((cells - float64(n)) * 8); e > 0 {
		part, rest = tokenEighths[e-1], rest-1
	}
	return strings.Repeat("█", n), part, strings.Repeat("░", rest)
}

func tokenLeft(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd%dh", int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour))
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	case d >= tokenFine:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}

func (m model) openLogin() (tea.Model, tea.Cmd) {
	if m.context == "" && m.kubeconfig == "" {
		m.status = "select a context first"
		return m, nil
	}
	d := m.newDialog("login", "token", "", "", "").
		withField("", "paste token", "replaces this context's token in the kubeconfig")
	d.input.CharLimit = 16384
	m.dlg = d
	return m, nil
}

func (m model) submitLogin() (tea.Model, tea.Cmd) {
	token := strings.TrimSpace(m.dlg.input.Value())
	m.dlg = nil
	if token == "" {
		m.status = "empty token"
		return m, nil
	}
	if _, err := kube.SetContextToken(m.kubeconfig, m.context, token); err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.tok = tokenState{}
	m.status = "token updated · " + m.context
	return m, m.tokenCmd()
}

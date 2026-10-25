package ui

import (
	"strconv"
	"strings"

	"k2/internal/kustomize"
	"k2/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

var th = theme.Nord()

var (
	titleStyle        = th.Styles().Title
	itemStyle         = th.Styles().Item
	selectedItemStyle = th.Styles().Selected
	dimStyle          = th.Styles().Dim
	keyStyle          = th.Styles().Key
	okStyle           = th.Styles().OK
	warnStyle         = th.Styles().Warn
	errStyle          = th.Styles().Err
)

func useTheme(p theme.Palette) {
	th = p
	st := p.Styles()
	titleStyle, itemStyle, selectedItemStyle = st.Title, st.Item, st.Selected
	dimStyle, keyStyle, okStyle, warnStyle, errStyle = st.Dim, st.Key, st.OK, st.Warn, st.Err
}

type tone int

const (
	toneNormal tone = iota
	toneOK
	toneDone
	toneWarn
	toneErr
	toneMuted
)

func statusTone(s string) tone {
	l := strings.ToLower(strings.TrimSpace(s))
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(l, w) {
				return true
			}
		}
		return false
	}
	switch {
	case l == "" || l == "-":
		return toneNormal
	case has("err", "fail", "crash", "backoff", "pull", "oomkilled", "evicted", "invalid", "lost", "degraded", "rejected", "denied"):
		return toneErr
	case l == "succeeded":

		return toneOK
	case has("pending", "warning", "suspend", "connecting", "idle", "install", "creating", "init", "terminating", "waiting", "progressing", "unknown", "notready",
		"installing", "replacing", "deleting", "requiresapproval", "upgrade", "unschedulable"):
		return toneWarn
	case has("complet", "succeed"):
		return toneDone
	case has("run", "active", "bound", "ready", "available", "healthy", "atlatest", "established"):
		return toneOK
	default:
		return toneNormal
	}
}

func toneStyle(t tone) lipgloss.Style {
	switch t {
	case toneOK:
		return okStyle
	case toneDone:
		return lipgloss.NewStyle().Foreground(th.Accent2)
	case toneWarn:
		return warnStyle
	case toneErr:
		return errStyle
	case toneMuted:
		return dimStyle
	default:
		return itemStyle
	}
}

func statusStyle(s string) lipgloss.Style { return toneStyle(statusTone(s)) }

func colorYAML(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	trim := strings.TrimLeft(line, " ")
	if strings.HasPrefix(trim, "#") {
		return dimStyle.Render(line)
	}
	if strings.HasPrefix(trim, "- ") {
		return itemStyle.Render(indent+"- ") + colorYAML(strings.TrimPrefix(trim, "- "))
	}
	if i := strings.Index(trim, ":"); i > 0 && !strings.Contains(trim[:i], " ") {
		key := trim[:i]
		rest := trim[i+1:]
		out := keyStyle.Render(indent+key) + dimStyle.Render(":")
		if strings.TrimSpace(rest) == "" {
			return out
		}
		return out + yamlValue(rest)
	}
	return itemStyle.Render(indent) + yamlValue(trim)
}

func yamlValue(v string) string {
	t := strings.TrimSpace(v)
	lead := v[:len(v)-len(strings.TrimLeft(v, " "))]
	st := itemStyle
	switch strings.TrimRight(t, ",") {
	case "true", "false", "null", "~":
		st = errStyle
	default:
		if len(t) >= 2 && (t[0] == '"' || t[0] == '\'') {
			st = okStyle
		} else if t != "" && t != "|" && t != ">" {
			if _, err := strconv.ParseFloat(strings.TrimRight(t, ","), 64); err == nil {
				st = lipgloss.NewStyle().Foreground(th.Accent3)
			}
		}
	}
	return itemStyle.Render(lead) + st.Render(t)
}

func colorDiff(line string) string {
	switch {
	case strings.HasPrefix(line, kustomize.DiffHeaderPrefix):

		return titleStyle.Render(line)
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return titleStyle.Render(line)
	case strings.HasPrefix(line, "diff "):
		return dimStyle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return dimStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return okStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return errStyle.Render(line)
	default:
		return itemStyle.Render(line)
	}
}

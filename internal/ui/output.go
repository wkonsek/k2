package ui

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/kustomize"

	tea "github.com/charmbracelet/bubbletea"
)

type runInfo struct {
	set     bool
	code    int
	outcome string
	took    time.Duration
	mode    string
}

func (r runInfo) header() string {
	t := toneOK
	switch r.outcome {
	case "failed":
		t = toneErr
	case "differences":
		t = toneWarn
	}
	return toneStyle(t).Render(toastIcon(t) + " exit " + fmt.Sprint(r.code) + " · " + r.outcome + " · " + elapsed(r.took))
}

func (m *model) showOutput(msg kOutMsg) tea.Cmd {
	var took time.Duration
	if m.load.active() {
		took = now().Sub(m.load.start)
	}
	m.load = load{}
	verb := m.subject
	outcome := kustomize.Outcome(verb, msg.code)
	if verb == "diff" {
		outcome = kustomize.DiffOutcome(msg.text, msg.code)
	}
	mode := "plain"
	switch verb {
	case "apply", "delete", "diff":
		ctx := msg.ctx
		if m.cfg.IsProd(ctx) {
			ctx = skull + " " + ctx
		}
		m.logText = kustomize.Report{Verb: verb, Target: msg.target, Context: ctx, Text: msg.text, Code: msg.code, Took: took}.String()
		mode = "kust"
	default:
		kverb, format := kubectlShape(msg.line)
		text, fverb := msg.text, verb
		switch {
		case kverb == "diff":
			text = kustomize.CollapseDiff(text)
			fverb = "diff"
			outcome = kustomize.DiffOutcome(msg.text, msg.code)
			mode = "diff"
		case format == "yaml":
			mode = "yaml"
		case kverb == "apply" || kverb == "delete" || kverb == "create" || kverb == "replace" || kverb == "patch" || kverb == "label" || kverb == "annotate":
			mode = "kust"
		}
		m.logText = kustomize.Finish(text, msg.code, fverb)
	}
	m.ran = runInfo{set: true, code: msg.code, outcome: outcome, took: took, mode: mode}
	m.logStick = true
	m.logAt = 0
	m.stage = "kout"
	m.status = fmt.Sprintf("exit %d · %s · %s", msg.code, outcome, elapsed(took))
	return m.runToast(verb, msg)
}

func (m *model) runToast(verb string, msg kOutMsg) tea.Cmd {
	c := kustomize.Count(msg.text)
	switch {
	case verb != "apply" && verb != "delete" && verb != "diff":
		return nil
	case verb == "diff":
		switch kustomize.DiffOutcome(msg.text, msg.code) {
		case "done":
			return m.notify(toneOK, "no differences")
		case "failed":
			return m.notify(toneErr, fmt.Sprintf("diff failed · exit %d", msg.code))
		default:
			n := len(kustomize.DiffHeaders(kustomize.CollapseDiff(msg.text)))
			if n == 0 {
				return m.notify(toneWarn, "differences found")
			}
			return m.notify(toneWarn, plural(n, "resource differs", "resources differ"))
		}
	case msg.code != 0:
		return m.notify(toneErr, fmt.Sprintf("%s failed · exit %d", verb, msg.code))
	}
	return m.notify(toneOK, map[string]string{"apply": "applied", "delete": "deleted"}[verb]+" · "+plural(c.Objects(), "object", "objects"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func kubectlShape(line string) (verb, format string) {
	f := strings.Fields(line)
	if len(f) > 0 && f[0] == "kubectl" {
		f = f[1:]
	}
	for i := 0; i < len(f); i++ {
		w := f[i]
		switch {
		case w == "-o" || w == "--output":
			if i+1 < len(f) {
				format = f[i+1]
				i++
			}
		case valueFlags[w]:
			i++
		case strings.HasPrefix(w, "-o="), strings.HasPrefix(w, "--output="):
			format = w[strings.Index(w, "=")+1:]
		case strings.HasPrefix(w, "-o") && len(w) > 2 && !strings.HasPrefix(w, "--"):
			format = w[2:]
		case verb == "" && !strings.HasPrefix(w, "-"):
			verb = w
		}
	}
	return verb, format
}

var valueFlags = map[string]bool{
	"-n": true, "--namespace": true, "-l": true, "--selector": true, "-f": true, "--filename": true,
	"-c": true, "--container": true, "--context": true, "--kubeconfig": true, "-k": true, "--kustomize": true,
	"--field-selector": true, "--sort-by": true,
}

func colorOut(line, mode string) string {
	if strings.Contains(line, "\x1b[") {
		return line
	}
	switch {
	case strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "│") || strings.HasPrefix(line, "╰"):
		return colorBox(line)
	case strings.HasPrefix(line, "── exit "):
		return colorExit(line)
	}
	switch mode {
	case "kust":
		return colorResult(line)
	case "diff":
		return colorDiff(line)
	case "yaml":
		return colorYAML(line)
	}
	if strings.HasPrefix(line, kustomize.DiffHeaderPrefix) {
		return colorDiff(line)
	}

	switch kustomize.Classify(line) {
	case kustomize.Error:
		return errStyle.Render(line)
	case kustomize.Warning:
		return warnStyle.Render(line)
	}
	return itemStyle.Render(line)
}

func classTone(c kustomize.Class) tone {
	switch c {
	case kustomize.Created:
		return toneOK
	case kustomize.Configured, kustomize.ServerSide, kustomize.Warning:
		return toneWarn
	case kustomize.Deleted, kustomize.Pruned:
		return toneDone
	case kustomize.Error:
		return toneErr
	}
	return toneNormal
}

func colorResult(line string) string {
	c := kustomize.Classify(line)
	switch {
	case c == kustomize.Other:
		if strings.HasPrefix(line, kustomize.DiffHeaderPrefix) || strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			return colorDiff(line)
		}
		return itemStyle.Render(line)
	case c == kustomize.Unchanged:
		return dimStyle.Render(kustomize.Icon(c) + " " + line)
	}
	return toneStyle(classTone(c)).Render(kustomize.Icon(c) + " " + line)
}

func colorBox(line string) string {
	r := []rune(line)
	if len(r) < 2 {
		return dimStyle.Render(line)
	}
	first, last := string(r[0]), string(r[len(r)-1])
	inner := string(r[1 : len(r)-1])
	switch first {
	case "╭":

		title := strings.TrimRight(strings.TrimRight(strings.TrimPrefix(inner, "─ "), "─"), " ")
		fill := strings.TrimPrefix(inner, "─ "+title)
		ts := titleStyle
		if strings.Contains(title, skull) {
			ts = ts.Foreground(th.Danger)
		}
		return dimStyle.Render(first+"─ ") + ts.Render(title) + dimStyle.Render(fill+last)
	case "╰":
		return dimStyle.Render(line)
	}
	var b strings.Builder
	b.WriteString(dimStyle.Render(first))
	rest := inner
	for rest != "" {
		i := strings.Index(rest, "   ")
		seg, gap := rest, ""
		if i >= 0 {
			seg = rest[:i]
			j := i
			for j < len(rest) && rest[j] == ' ' {
				j++
			}
			gap, rest = rest[i:j], rest[j:]
		} else {
			rest = ""
		}
		b.WriteString(colorSummaryPart(seg) + gap)
	}
	b.WriteString(dimStyle.Render(last))
	return b.String()
}

var summaryIcons = map[string]kustomize.Class{
	"+": kustomize.Created, "~": kustomize.Configured, "·": kustomize.Unchanged,
	"−": kustomize.Deleted, "!": kustomize.Warning, "✗": kustomize.Error,
}

func colorSummaryPart(seg string) string {
	t := strings.TrimLeft(seg, " ")
	lead := seg[:len(seg)-len(t)]
	if f := strings.Fields(t); len(f) > 0 {
		if c, ok := summaryIcons[f[0]]; ok {
			if c == kustomize.Unchanged {
				return lead + dimStyle.Render(t)
			}
			return lead + toneStyle(classTone(c)).Render(t)
		}
	}
	if strings.Contains(t, "differ") {
		return lead + warnStyle.Render(t)
	}
	return lead + itemStyle.Render(t)
}

func colorExit(line string) string {
	t := toneOK
	switch {
	case strings.Contains(line, "· failed"):
		t = toneErr
	case strings.Contains(line, "· differences"):
		t = toneWarn
	}
	body := strings.TrimSuffix(strings.TrimPrefix(line, "── "), " ──")
	return dimStyle.Render("── ") + toneStyle(t).Render(body) + dimStyle.Render(" ──")
}

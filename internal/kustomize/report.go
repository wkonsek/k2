package kustomize

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"k2/internal/shell"
)

type Class int

const (
	Other Class = iota
	Created
	Configured
	Unchanged
	ServerSide
	Deleted
	Pruned
	Warning
	Error
)

var classWords = map[string]Class{
	"created": Created, "configured": Configured, "unchanged": Unchanged,
	"serverside-applied": ServerSide, "deleted": Deleted, "pruned": Pruned,
}

var names = map[Class]string{
	Created: "created", Configured: "configured", Unchanged: "unchanged", ServerSide: "serverside-applied",
	Deleted: "deleted", Pruned: "pruned", Warning: "warnings", Error: "errors",
}

var singular = map[Class]string{Warning: "warning", Error: "error"}

func countWord(c Class, n int) string {
	if w, ok := singular[c]; ok && n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %s", n, names[c])
}

var icons = map[Class]string{
	Created: "+", Configured: "~", Unchanged: "·", ServerSide: "~",
	Deleted: "−", Pruned: "−", Warning: "!", Error: "✗",
}

func Icon(c Class) string { return icons[c] }

var resultSuffix = regexp.MustCompile(`(\s+from\s+\S+\s+namespace)?(\s+\((server )?dry run\))?$`)

func Classify(line string) Class {
	t := strings.TrimSpace(line)
	t = strings.TrimSpace(strings.TrimPrefix(t, "# "))
	l := strings.ToLower(t)
	switch {
	case t == "":
		return Other
	case strings.HasPrefix(l, "error"), strings.Contains(l, "error from server"), strings.HasPrefix(l, "unable to"),
		strings.HasPrefix(l, "the server"), strings.HasPrefix(l, "failed"), strings.Contains(l, " is invalid:"):
		return Error
	case strings.HasPrefix(l, "warning:"):
		return Warning
	}
	f := strings.Fields(resultSuffix.ReplaceAllString(l, ""))
	if len(f) >= 2 {
		if c, ok := classWords[f[len(f)-1]]; ok {
			return c
		}
	}
	return Other
}

func IsObject(c Class) bool { return c >= Created && c <= Pruned }

func Settles(line string) bool {
	c := Classify(line)
	return IsObject(c) || c == Error && strings.HasSuffix(strings.TrimSpace(strings.ToLower(line)), " not found")
}

type Counts map[Class]int

func Count(text string) Counts {
	c := Counts{}
	for _, line := range strings.Split(text, "\n") {
		if k := Classify(line); k != Other {
			c[k]++
		}
	}
	return c
}

func (c Counts) Objects() int {
	n := 0
	for k, v := range c {
		if IsObject(k) {
			n += v
		}
	}
	return n
}

func (c Counts) Parts() []string {
	var out []string
	for k := Created; k <= Error; k++ {
		if n := c[k]; n > 0 {
			out = append(out, icons[k]+" "+countWord(k, n))
		}
	}
	return out
}

func Build(r shell.Runner, dir string) (total int, ok bool) {
	res := r.Run(shell.Request{Name: "kubectl", Args: []string{"kustomize", dir}})
	if res.Err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if strings.HasPrefix(line, "kind:") {
			total++
		}
	}
	return total, total > 0
}

func RunStream(r shell.Runner, verb, target, kc, ctx string, onLine func(string)) (string, int) {
	flag := "-k"
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		flag = "-f"
	}
	req := shell.Request{Name: "kubectl", Args: []string{verb, flag, target, "--kubeconfig", kc, "--context", ctx}, OnLine: onLine}
	if verb == "diff" {
		req.TTY = true
	}
	return run(r, req, verb)
}

type Report struct {
	Verb    string
	Target  string
	Context string
	Text    string
	Code    int
	Took    time.Duration
}

func (r Report) String() string {
	text := strings.TrimRight(r.Text, "\n")
	var lines []string
	head := ""
	failed := Outcome(r.Verb, r.Code) == "failed"
	c := Count(text)
	if r.Verb == "diff" {
		text = CollapseDiff(text)
		n := len(DiffHeaders(text))
		switch {
		case failed:

			lines = append(lines, failLine(r.Verb, c[Error]))
			if n > 0 {
				lines = append(lines, fmt.Sprintf("%s before it failed", plural(n, "resource differs", "resources differ")))
			}
		case n == 1:
			lines = append(lines, "1 resource differs")
		case n > 1:
			lines = append(lines, fmt.Sprintf("%d resources differ", n))
		case r.Code == 1:
			lines = append(lines, "differences found")
		default:
			lines = append(lines, "no differences")
		}
		var rest []string
		for _, k := range []Class{Warning, Error} {
			if c[k] > 0 {
				rest = append(rest, icons[k]+" "+countWord(k, c[k]))
			}
		}
		if len(rest) > 0 {
			lines = append(lines, strings.Join(rest, "   "))
		}
	} else {
		if failed {
			lines = append(lines, failLine(r.Verb, c[Error]))
		}
		lines = append(lines, wrapParts(c.Parts(), 44)...)
		done := map[string]string{"apply": "applied", "delete": "deleted"}[r.Verb]
		switch {
		case failed && c.Objects() == 0:
			lines = append(lines, "nothing "+or(done, "done"))
		case failed:
			lines = append(lines, plural(c.Objects(), "object", "objects")+" "+or(done, "done")+" before it failed")
		default:
			lines = append(lines, plural(c.Objects(), "object", "objects"))
		}
	}
	lines[len(lines)-1] += " · " + took(r.Took)
	title := r.Verb
	if r.Target != "" {
		title += " · " + r.Target
	}
	if r.Context != "" {
		title += " · " + r.Context
	}
	if text != "no differences" {
		head = text + "\n\n"
	}
	return head + Box(title, lines) + fmt.Sprintf("\n\n── exit %d · %s ──", r.Code, Outcome(r.Verb, r.Code))
}

func failLine(verb string, errs int) string {
	where := "see the output above"
	switch {
	case errs == 1:
		where = "see error above"
	case errs > 1:
		where = "see errors above"
	}
	return icons[Error] + " " + verb + " failed — " + where
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func took(d time.Duration) string {
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d >= 10*time.Second:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func wrapParts(parts []string, width int) []string {
	var out []string
	cur := ""
	for _, p := range parts {
		switch {
		case cur == "":
			cur = p
		case len([]rune(cur))+3+len([]rune(p)) <= width:
			cur += "   " + p
		default:
			out = append(out, cur)
			cur = p
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func Box(title string, lines []string) string {
	w := len([]rune(title)) + 4
	for _, l := range lines {
		if n := len([]rune(l)) + 2; n > w {
			w = n
		}
	}
	w = max(w, 32)
	var b strings.Builder
	b.WriteString("╭─ " + title + " " + strings.Repeat("─", w-len([]rune(title))-3) + "╮")
	for _, l := range lines {
		b.WriteString("\n│ " + l + strings.Repeat(" ", w-len([]rune(l))-2) + " │")
	}
	b.WriteString("\n╰" + strings.Repeat("─", w) + "╯")
	return b.String()
}

var diffFile = regexp.MustCompile(`/(?:LIVE|MERGED)-[^/\s]+/(\S+)`)

var version = regexp.MustCompile(`^v\d+((alpha|beta)\d+)?$`)

const DiffHeaderPrefix = "▸ "

func CollapseDiff(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		name := ""
		switch {
		case strings.HasPrefix(l, "diff ") && diffFile.MatchString(l):
			name = diffFile.FindStringSubmatch(l)[1]

			for j := 0; j < 2 && i+1 < len(lines) && (strings.HasPrefix(lines[i+1], "--- ") || strings.HasPrefix(lines[i+1], "+++ ")); j++ {
				i++
			}
		case strings.HasPrefix(l, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ") && diffFile.MatchString(lines[i+1]):
			name = diffFile.FindStringSubmatch(lines[i+1])[1]
			i++
		}
		if name == "" {
			out = append(out, l)
			continue
		}
		out = append(out, DiffHeaderPrefix+ResourceName(name))
	}
	return strings.Join(out, "\n")
}

func DiffHeaders(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, DiffHeaderPrefix) {
			out = append(out, strings.TrimPrefix(l, DiffHeaderPrefix))
		}
	}
	return out
}

func ResourceName(file string) string {
	parts := strings.Split(file, ".")

	for i := 0; i+3 < len(parts); i++ {
		if !version.MatchString(parts[i]) || parts[i+1] == "" || parts[i+1][0] < 'A' || parts[i+1][0] > 'Z' {
			continue
		}
		group, ver, kind, ns := strings.Join(parts[:i], "."), parts[i], parts[i+1], parts[i+2]
		name := strings.Join(parts[i+3:], ".")
		if name == "" {
			break
		}
		ref := name
		if ns != "" {
			ref = ns + "/" + name
		}
		api := ver
		if group != "" {
			api = group + "/" + ver
		}
		return kind + " " + ref + "  (" + api + ")"
	}
	return file
}

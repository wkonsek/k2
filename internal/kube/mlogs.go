package kube

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"
)

type LogQuery struct {
	Selector  string
	Regex     *regexp.Regexp
	Container string
	All       bool
	Since     time.Duration
	Tail      int
}

func (q LogQuery) String() string {
	s := q.Selector
	if s == "" && q.Regex != nil {
		s = "/" + q.Regex.String() + "/"
	}
	if q.Container != "" {
		s += " -c " + q.Container
	}
	return s
}

func ParseLogQuery(args string, allDefault bool) (LogQuery, error) {
	q := LogQuery{All: allDefault, Tail: -1}
	f := strings.Fields(args)
	var target []string
	for i := 0; i < len(f); i++ {
		a := f[i]
		val := func(prefix string) (string, bool) {
			if v, ok := strings.CutPrefix(a, prefix+"="); ok {
				return v, true
			}
			if a == prefix && i+1 < len(f) {
				i++
				return f[i], true
			}
			return "", false
		}
		if v, ok := val("--since"); ok {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				return q, fmt.Errorf("--since %q: use a duration such as 5m or 1h", v)
			}
			q.Since = d
			continue
		}
		if v, ok := val("--tail"); ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return q, fmt.Errorf("--tail %q: use a number of lines", v)
			}
			q.Tail = n
			continue
		}
		if v, ok := val("--container"); ok {
			q.Container, q.All = v, false
			continue
		}
		if v, ok := val("-c"); ok {
			q.Container, q.All = v, false
			continue
		}
		if v, ok := strings.CutPrefix(a, "--all="); ok {
			q.All = v == "true"
			continue
		}
		if a == "--all" || a == "-a" {
			q.All = true
			continue
		}
		target = append(target, a)
	}
	t := strings.Join(target, " ")
	switch {
	case t == "":
		return q, fmt.Errorf("logs: name a selector (app=web) or a pod name pattern")
	case strings.ContainsAny(t, "=") || strings.Contains(t, " in (") || strings.Contains(t, " notin ("):
		q.Selector = t
	default:
		re, err := regexp.Compile(t)
		if err != nil {
			return q, fmt.Errorf("logs: %q is no valid pattern: %v", t, err)
		}
		q.Regex = re
	}
	return q, nil
}

func SelectorOf(o Object) string {
	var parts []string
	if BaseKind(o.Str("kind")) == "services" || o.Str("kind") == "Service" {
		for k, v := range labelsOf(o.Get("spec", "selector")) {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		return strings.Join(parts, ",")
	}
	for k, v := range labelsOf(o.Get("spec", "selector", "matchLabels")) {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	for _, e := range o.List("spec", "selector", "matchExpressions") {
		key, op := field(e, "key"), strings.ToLower(field(e, "operator"))
		var vals []string
		for _, v := range listAt(e, "values") {
			if s, ok := v.(string); ok {
				vals = append(vals, s)
			}
		}
		switch op {
		case "in", "notin":
			parts = append(parts, key+" "+op+" ("+strings.Join(vals, ",")+")")
		case "exists":
			parts = append(parts, key)
		case "doesnotexist":
			parts = append(parts, "!"+key)
		}
	}
	return strings.Join(parts, ",")
}

type LogLine struct {
	Namespace      string
	Pod, Container string
	At             time.Time
	Text           string
}

func (l LogLine) Source() string {
	if l.Namespace != "" {
		return l.Namespace + "/" + l.Pod + "/" + l.Container
	}
	return l.Pod + "/" + l.Container
}

func MultiLogsArgs(pod, namespace, kubeconfig, context string, q LogQuery, since time.Time, tail int) []string {
	args := []string{"logs", pod, "-n", namespace, "--prefix", "--timestamps", "--kubeconfig", kubeconfig, "--context", context}
	switch {
	case q.Container != "":
		args = append(args, "-c", q.Container)
	case q.All:
		args = append(args, "--all-containers")
	}
	switch {
	case !since.IsZero():
		args = append(args, "--since-time="+since.UTC().Format(time.RFC3339))
	case q.Since > 0:
		args = append(args, "--since="+q.Since.String())
	default:
		args = append(args, "--tail="+strconv.Itoa(tail))
	}
	return args
}

func ParseMultiLogs(out, pod string) []LogLine {
	var lines []LogLine
	var last time.Time
	for _, raw := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if raw == "" {
			continue
		}
		l := LogLine{Pod: pod, Container: "?", At: last, Text: raw}
		if strings.HasPrefix(raw, "[") {
			if end := strings.Index(raw, "] "); end > 0 {
				ref := raw[1:end]
				rest := raw[end+2:]
				parts := strings.Split(ref, "/")
				if len(parts) == 3 && parts[0] == "pod" {
					l.Pod, l.Container = parts[1], parts[2]
				}
				l.Text = rest
			}
		}
		if ts, text, ok := strings.Cut(l.Text, " "); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				l.At, l.Text = t, text
			}
		} else if t, err := time.Parse(time.RFC3339Nano, l.Text); err == nil {
			l.At, l.Text = t, ""
		}
		last = l.At
		lines = append(lines, l)
	}
	return lines
}

type LogCursor map[string]time.Time

func (c LogCursor) Fresh(lines []LogLine) ([]LogLine, LogCursor) {
	next := LogCursor{}
	for k, v := range c {
		next[k] = v
	}
	var out []LogLine
	for _, l := range lines {
		key := l.Source()
		if seen, ok := next[key]; ok && !l.At.After(seen) {
			continue
		}
		out = append(out, l)
	}
	for _, l := range out {
		if l.At.After(next[l.Source()]) {
			next[l.Source()] = l.At
		}
	}
	return out, next
}

func (c LogCursor) Since(pod string) time.Time {
	var t time.Time
	for k, v := range c {
		if strings.HasPrefix(k, pod+"/") && (t.IsZero() || v.Before(t)) {
			t = v
		}
	}
	return t
}

func SortLines(lines []LogLine) {
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At.Before(lines[j].At) })
}

func MatchPods(objs []Object, q LogQuery) []Object {
	var out []Object
	for _, o := range objs {
		if q.Regex != nil && !q.Regex.MatchString(o.Metadata.Name) {
			continue
		}
		if o.Status.Phase == "Pending" && len(o.Status.ContainerStatuses) == 0 {
			continue
		}
		out = append(out, o)
	}
	return out
}

func (c Client) MultiLogs(pod, namespace, kubeconfig, context string, q LogQuery, since time.Time, tail int) ([]LogLine, error) {
	out, err := shell.Combined(c.Run, "kubectl", MultiLogsArgs(pod, namespace, kubeconfig, context, q, since, tail)...)
	if err != nil {
		return nil, cmdError(out, err)
	}
	return ParseMultiLogs(string(out), pod), nil
}

package forward

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"k2/internal/shell"
)

func Up(f Forward) bool { return f.Reason == "" && Alive(f.PID) }

func Display(f Forward) string {
	if f.Via != "" && KindOr(f.Kind) != "pod" {
		return Target(f) + " → pod/" + f.Via
	}
	return Target(f)
}

var klogLine = regexp.MustCompile(`^([IWEF])\d{4} \d\d:\d\d:\d\d\.\d+\s+\d+ [^\]]*\] ?(.*)$`)

var viaURL = regexp.MustCompile(`/namespaces/[^/\s]+/pods/([^/\s?]+)/portforward`)

var notFound = regexp.MustCompile(`(?i)\bpods? .*not ?found|\(notfound\)`)

func Signal(line string) string {
	msg := strings.TrimSpace(line)
	if m := klogLine.FindStringSubmatch(msg); m != nil {
		if m[1] == "I" {
			return ""
		}
		msg = m[2]
	}
	low := strings.ToLower(msg)
	switch {
	case low == "":
		return ""
	case strings.Contains(low, "lost connection to pod"):
		return "lost connection to pod"
	case notFound.MatchString(msg):
		return "pod gone"
	case strings.Contains(low, "connection refused"):
		return "remote port refused"
	case strings.Contains(low, "unable to do port forwarding"):
		return "port forwarding failed"
	case strings.Contains(low, "error upgrading connection"):
		return "connection upgrade failed"
	case strings.Contains(low, "an error occurred forwarding") || strings.Contains(low, "error forwarding"):
		return "forward error"
	}
	return ""
}

const scanMax = 256 << 10

type Scan struct {
	Offset int64
	Via    string
	Reason string
	Line   string
}

func ScanLog(path string, offset int64) Scan {
	s := Scan{Offset: offset}
	if path == "" {
		return s
	}
	fh, err := os.Open(path)
	if err != nil {
		return s
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return s
	}
	size := st.Size()
	if size < offset {
		offset = 0
	}
	start := max(offset, size-scanMax)
	if _, err := fh.Seek(start, io.SeekStart); err != nil {
		return s
	}
	b, err := io.ReadAll(io.LimitReader(fh, size-start))
	if err != nil {
		return s
	}
	end := strings.LastIndexByte(string(b), '\n')
	if end < 0 {
		return Scan{Offset: start}
	}
	s.Offset = start + int64(end) + 1
	for _, line := range strings.Split(string(b[:end]), "\n") {
		if m := viaURL.FindStringSubmatch(line); m != nil {
			s.Via = m[1]
		}
		if s.Reason == "" {
			if r := Signal(line); r != "" {
				s.Reason, s.Line = r, strings.TrimSpace(line)
			}
		}
	}
	return s
}

func ExitReason(path string) (string, string) {
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if r := Signal(l); r != "" {
			return r, l
		}
		if last == "" && l != "" && !isInfo(l) {
			last = l
		}
	}
	if last != "" {
		return "kubectl exited: " + shortLine(last), last
	}
	return "kubectl exited", ""
}

func isInfo(l string) bool {
	m := klogLine.FindStringSubmatch(l)
	return m != nil && m[1] == "I"
}

func shortLine(l string) string {
	if m := klogLine.FindStringSubmatch(l); m != nil {
		l = m[2]
	}
	l = shell.OneLine(l)
	if r := []rune(l); len(r) > 80 {
		l = string(r[:79]) + "…"
	}
	return l
}

func startError(log string) string {
	var keep []string
	for _, l := range strings.Split(log, "\n") {
		if l = strings.TrimSpace(l); l != "" && !isInfo(l) {
			if m := klogLine.FindStringSubmatch(l); m != nil {
				l = m[2]
			}
			keep = append(keep, l)
		}
	}
	msg := strings.Join(keep, "\n")
	if msg == "" {
		msg = "kubectl exited"
	}
	if len(msg) > 220 {
		msg = msg[len(msg)-220:]
	}
	return msg
}

type probeObj struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name              string `json:"name"`
		DeletionTimestamp string `json:"deletionTimestamp"`
	} `json:"metadata"`
	Status struct {
		Phase         string `json:"phase"`
		ReadyReplicas int    `json:"readyReplicas"`
	} `json:"status"`
	Items []probeObj `json:"items"`
}

var probeKinds = map[string]string{"deploy": "Deployment", "sts": "StatefulSet", "rs": "ReplicaSet"}

func probeRef(f Forward) string {
	kind := KindOr(f.Kind)
	switch {
	case kind == "pod":
		return "pod/" + f.Pod
	case f.Via != "":
		return "pod/" + f.Via
	case probeKinds[kind] != "":
		return kind + "/" + f.Pod
	}
	return ""
}

const ProbeTimeout = "5s"

func Probe(r shell.Runner, fs []Forward) map[int]string {
	type group struct {
		kc, ctx, ns string
		refs        []string
		fwds        []Forward
	}
	groups := map[string]*group{}
	var order []string
	for _, f := range fs {
		ref := probeRef(f)
		if ref == "" || !Up(f) {
			continue
		}
		key := f.Kubeconfig + "\x00" + f.Context + "\x00" + nsOr(f.Namespace)
		g := groups[key]
		if g == nil {
			g = &group{kc: f.Kubeconfig, ctx: f.Context, ns: nsOr(f.Namespace)}
			groups[key] = g
			order = append(order, key)
		}
		g.refs = append(g.refs, ref)
		g.fwds = append(g.fwds, f)
	}
	dead := map[int]string{}
	for _, key := range order {
		g := groups[key]
		refs := uniq(g.refs)
		args := append([]string{"get"}, refs...)
		args = append(args, "-n", g.ns, "--kubeconfig", g.kc, "--context", g.ctx,
			"--ignore-not-found", "--request-timeout="+ProbeTimeout, "-o", "json")
		out, err := shell.Output(r, "kubectl", args...)
		if err != nil {
			continue
		}
		found := map[string]probeObj{}
		if s := strings.TrimSpace(string(out)); s != "" {
			var top probeObj
			if json.Unmarshal([]byte(s), &top) != nil {
				continue
			}
			list := top.Items
			if top.Kind != "List" {
				list = []probeObj{top}
			}
			for _, o := range list {
				found[o.Kind+"/"+o.Metadata.Name] = o
			}
		}
		for _, f := range g.fwds {
			if why := judge(probeRef(f), found); why != "" {
				dead[f.PID] = why
			}
		}
	}
	return dead
}

func judge(ref string, found map[string]probeObj) string {
	kind, name, _ := strings.Cut(ref, "/")
	if kind == "pod" {
		o, ok := found["Pod/"+name]
		switch {
		case !ok:
			return "pod gone"
		case o.Metadata.DeletionTimestamp != "":
			return "pod terminating"
		case o.Status.Phase == "Succeeded" || o.Status.Phase == "Failed":
			return "pod " + o.Status.Phase
		}
		return ""
	}
	o, ok := found[probeKinds[kind]+"/"+name]
	switch {
	case !ok:
		return ref + " gone"
	case o.Status.ReadyReplicas == 0:
		return "no ready pods"
	}
	return ""
}

func uniq(ss []string) []string {
	m := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

var (
	deployRest = regexp.MustCompile(`^[a-z0-9]{6,10}-[a-z0-9]{5}$`)
	stsRest    = regexp.MustCompile(`^[0-9]+$`)
)

var guessKinds = []struct {
	api, short string
	rest       *regexp.Regexp
}{
	{"Deployment", "deploy", deployRest},
	{"StatefulSet", "sts", stsRest},
}

func GuessOwner(r shell.Runner, kc, ctx, ns, pod string) (string, error) {
	out, err := shell.Output(r, "kubectl", "get", "deployments,statefulsets,pods", "-n", ns,
		"--kubeconfig", kc, "--context", ctx, "--request-timeout=10s",
		"-o", `jsonpath={range .items[*]}{.kind}/{.metadata.name}{"\n"}{end}`)
	if err != nil {
		return "", fmt.Errorf("pod %s not found; no owner recorded, and the lookup failed: %s", pod, shell.FailText([]byte(err.Error()), nil))
	}
	objs := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, n, ok := strings.Cut(strings.TrimSpace(l), "/"); ok {
			objs[k] = append(objs[k], n)
		}
	}
	var hits []string
	for _, gk := range guessKinds {
		for _, w := range objs[gk.api] {
			if !named(pod, w, gk.rest) {
				continue
			}
			for _, p := range objs["Pod"] {
				if p != pod && named(p, w, gk.rest) {
					hits = append(hits, gk.short+"/"+w)
					break
				}
			}
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", fmt.Errorf("pod %s not found; no owner to fall back to (none recorded, and no deployment or statefulset in %s has pods named like it)", pod, ns)
	}
	sort.Strings(hits)
	return "", fmt.Errorf("pod %s not found; no owner to fall back to (none recorded, and %s all match; start the forward again from a current pod)", pod, strings.Join(hits, ", "))
}

func named(pod, w string, rest *regexp.Regexp) bool {
	tail, ok := strings.CutPrefix(pod, w+"-")
	return ok && rest.MatchString(tail)
}

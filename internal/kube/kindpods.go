package kube

import (
	"strconv"
	"strings"
)

type KindPods struct {
	Running, Total int
	OK             bool
}

func KubectlPodCountArgs(kubeconfig, context string) []string {
	return []string{"get", "pods", "-A", "--no-headers", "--kubeconfig", kubeconfig, "--context", context}
}

func ParsePodCounts(out string) KindPods {
	var p KindPods
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		p.Total++
		if podRowReady(fields) {
			p.Running++
		}
	}
	p.OK = true
	return p
}

func (p KindPods) Label() string {
	if !p.OK {
		return "—"
	}
	if p.Running == p.Total {
		return strconv.Itoa(p.Total)
	}
	return strconv.Itoa(p.Running) + "/" + strconv.Itoa(p.Total)
}

func podRowReady(fields []string) bool {
	ready, status := podRowReadyFields(fields)
	if a, b, ok := parseReadyFrac(ready); ok {
		return a == b && b > 0
	}
	return strings.EqualFold(status, "Running")
}

func podRowReadyFields(fields []string) (ready, status string) {
	if len(fields) >= 3 && parseReadyOK(fields[1]) {
		return fields[1], fields[2]
	}
	if len(fields) >= 4 && parseReadyOK(fields[2]) {
		return fields[2], fields[3]
	}
	if len(fields) >= 3 {
		return "", fields[2]
	}
	return "", ""
}

func parseReadyOK(s string) bool {
	_, _, ok := parseReadyFrac(s)
	return ok
}

func parseReadyFrac(s string) (int, int, bool) {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return 0, 0, false
	}
	left, err1 := strconv.Atoi(a)
	right, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || left < 0 || right < 0 {
		return 0, 0, false
	}
	return left, right, true
}

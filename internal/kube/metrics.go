package kube

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"k2/internal/shell"
)

type Usage struct {
	CPU, Mem string
}

func TopPodsArgs(kubeconfig, context, namespace string) []string {
	args := []string{"top", "pods", "--no-headers", "--request-timeout=" + InfoTimeout.String()}
	if namespace == "" {
		args = append(args, "-A")
	} else {
		args = append(args, "-n", namespace)
	}
	return append(args, targetArgs(kubeconfig, context)...)
}

func (c Client) TopPods(kubeconfig, context, namespace string) (map[string]Usage, error) {
	out, err := shell.Combined(c.Run, "kubectl", TopPodsArgs(kubeconfig, context, namespace)...)
	if err != nil {
		return nil, cmdError(out, err)
	}
	return ParseTop(string(out), namespace), nil
}

func ParseTop(out, namespace string) map[string]Usage {
	usage := map[string]Usage{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case namespace == "" && len(f) == 4:
			if _, ok := ParseCPU(f[2]); ok {
				usage[f[0]+"/"+f[1]] = Usage{CPU: f[2], Mem: f[3]}
			}
		case namespace != "" && len(f) == 3:
			if _, ok := ParseCPU(f[1]); ok {
				usage[namespace+"/"+f[0]] = Usage{CPU: f[1], Mem: f[2]}
			}
		}
	}
	return usage
}

func ParseCPU(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	scale := 1000.0
	switch {
	case strings.HasSuffix(s, "m"):
		s, scale = strings.TrimSuffix(s, "m"), 1
	case strings.HasSuffix(s, "u"):
		s, scale = strings.TrimSuffix(s, "u"), 1e-3
	case strings.HasSuffix(s, "n"):
		s, scale = strings.TrimSuffix(s, "n"), 1e-6
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v * scale, true
}

var memUnits = []struct {
	suffix string
	factor float64
}{
	{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50},
	{"k", 1e3}, {"K", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15},
}

func ParseMem(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	factor := 1.0
	for _, u := range memUnits {
		if strings.HasSuffix(s, u.suffix) {
			s, factor = strings.TrimSuffix(s, u.suffix), u.factor
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v * factor, true
}

func podLimits(o Object) (cpu, mem string) { return podResources(o, "limits") }

func podRequests(o Object) (cpu, mem string) { return podResources(o, "requests") }

func podResources(o Object, what string) (cpu, mem string) {
	var cpuSum, memSum float64
	cpuAll, memAll, n := true, true, 0
	for _, c := range o.List("spec", "containers") {
		n++
		cm, _ := c.(map[string]any)
		res, _ := cm["resources"].(map[string]any)
		lim, _ := res[what].(map[string]any)
		if v, ok := quantity(lim["cpu"], ParseCPU); ok {
			cpuSum += v
		} else {
			cpuAll = false
		}
		if v, ok := quantity(lim["memory"], ParseMem); ok {
			memSum += v
		} else {
			memAll = false
		}
	}
	if n == 0 {
		return "", ""
	}
	if cpuAll && cpuSum > 0 {
		cpu = strconv.FormatFloat(cpuSum, 'f', -1, 64)
	}
	if memAll && memSum > 0 {
		mem = strconv.FormatFloat(memSum, 'f', -1, 64)
	}
	return cpu, mem
}

func quantity(v any, parse func(string) (float64, bool)) (float64, bool) {
	switch q := v.(type) {
	case string:
		return parse(q)
	case float64:
		return parse(fmt.Sprint(q))
	}
	return 0, false
}

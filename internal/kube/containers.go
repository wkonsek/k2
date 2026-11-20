package kube

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"
)

var ContainerHeaders = []string{"NAME", "PF", "IMAGE", "READY", "STATE", "INIT", "RESTARTS", "PROBES(L:R)", "CPU", "MEM",
	"CPU/R:L", "MEM/R:L", "%CPU/R", "%CPU/L", "%MEM/R", "%MEM/L", "PORTS", "AGE"}

const containerStateCol = 4

func TopContainersArgs(kubeconfig, context, namespace, pod string) []string {
	args := []string{"top", "pod", pod, "--containers", "--no-headers", "--request-timeout=" + InfoTimeout.String()}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return append(args, targetArgs(kubeconfig, context)...)
}

func (c Client) TopContainers(kubeconfig, context, namespace, pod string) (map[string]Usage, error) {
	out, err := shell.Combined(c.Run, "kubectl", TopContainersArgs(kubeconfig, context, namespace, pod)...)
	if err != nil {
		return nil, cmdError(out, err)
	}
	return ParseTopContainers(string(out)), nil
}

func ParseTopContainers(out string) map[string]Usage {
	usage := map[string]Usage{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 4 {
			continue
		}
		if _, ok := ParseCPU(f[2]); !ok {
			continue
		}
		usage[f[1]] = Usage{CPU: f[2], Mem: f[3]}
	}
	return usage
}

func (o Object) containerStatuses() map[string]any {
	out := map[string]any{}
	for _, key := range []string{"containerStatuses", "initContainerStatuses"} {
		for _, s := range o.List("status", key) {
			out[field(s, "name")] = s
		}
	}
	return out
}

func ContainerTable(o Object, usage map[string]Usage, metrics bool) Table {
	t := Table{Headers: append([]string(nil), ContainerHeaders...), ColorCol: containerStateCol}
	statuses := o.containerStatuses()
	ns, pod := o.Metadata.Namespace, o.Metadata.Name
	add := func(c any, init bool) {
		name := field(c, "name")
		st := statuses[name]
		sidecar := init && field(c, "restartPolicy") == "Always"
		label := name
		switch {
		case sidecar:
			label += " (sidecar)"
		case init:
			label += " (init)"
		}
		state, started := containerState(st)
		restarts := Object{Raw: map[string]any{"s": st}}.IntOr(0, "s", "restartCount")
		rest := strconv.Itoa(restarts)
		if restarts > 0 {
			if at, err := time.Parse(time.RFC3339, field(st, "lastState", "terminated", "finishedAt")); err == nil {
				rest += " (" + Age(at) + " ago)"
			}
		}
		born := started
		if born.IsZero() {
			born = o.Metadata.CreationTimestamp
		}
		cpuReq, cpuLim := resourceMilli(c, "requests"), resourceMilli(c, "limits")
		memReq, memLim := resourceMebi(c, "requests"), resourceMebi(c, "limits")
		u, found := usage[name]
		cpu, mem := "n/a", "n/a"
		var cpuUse, memUse float64
		haveUse := false
		if metrics {
			cpu, mem = "-", "-"
			if found {
				cpu, mem = u.CPU, u.Mem
				a, ok1 := ParseCPU(u.CPU)
				b, ok2 := ParseMem(u.Mem)
				cpuUse, memUse, haveUse = a, b/(1<<20), ok1 && ok2
			}
		}
		pct := func(use float64, base int) string {
			if !haveUse || base <= 0 {
				return "n/a"
			}
			return strconv.Itoa(int(use*100/float64(base) + 0.5))
		}
		ready := "false"
		if field(st, "ready") == "true" {
			ready = "true"
		}
		isInit := "false"
		if init {
			isInit = "true"
		}
		ports, nums := containerPorts(c)
		t.Rows = append(t.Rows, []string{label, "", field(c, "image"), ready, state, isInit, rest, probes(c), cpu, mem,
			fmt.Sprintf("%d:%d", cpuReq, cpuLim), fmt.Sprintf("%d:%d", memReq, memLim),
			pct(cpuUse, cpuReq), pct(cpuUse, cpuLim), pct(memUse, memReq), pct(memUse, memLim), ports, Age(born)})
		t.Refs = append(t.Refs, [2]string{ns, name})
		t.Born = append(t.Born, born)
		t.Extra = append(t.Extra, map[string]string{"container": name, "pod": pod, "ports": nums, "init": isInit,
			"sidecar": strconv.FormatBool(sidecar)})
	}
	for _, c := range o.List("spec", "initContainers") {
		add(c, true)
	}
	for _, c := range o.List("spec", "containers") {
		add(c, false)
	}
	return t
}

func containerState(st any) (string, time.Time) {
	if st == nil {
		return "Pending", time.Time{}
	}
	holder := Object{Raw: map[string]any{"s": st}}
	m, _ := holder.Get("s", "state").(map[string]any)
	switch {
	case m["running"] != nil:
		at, _ := time.Parse(time.RFC3339, field(m["running"], "startedAt"))
		return "Running", at
	case m["waiting"] != nil:
		return orElse(field(m["waiting"], "reason"), "Waiting"), time.Time{}
	case m["terminated"] != nil:
		at, _ := time.Parse(time.RFC3339, field(m["terminated"], "startedAt"))
		return orElse(field(m["terminated"], "reason"), "Terminated"), at
	}
	return "Pending", time.Time{}
}

func orElse(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func probes(c any) string {
	on := func(key string) string {
		if m, ok := c.(map[string]any); ok && m[key] != nil {
			return "on"
		}
		return "off"
	}
	return on("livenessProbe") + ":" + on("readinessProbe")
}

func containerPorts(c any) (label, nums string) {
	m, _ := c.(map[string]any)
	list, _ := m["ports"].([]any)
	var labels, numbers []string
	for _, p := range list {
		num := field(p, "containerPort")
		if num == "" {
			continue
		}
		numbers = append(numbers, num)
		if name := field(p, "name"); name != "" {
			labels = append(labels, name+":"+num)
		} else {
			labels = append(labels, num)
		}
	}
	return strings.Join(labels, ","), strings.Join(numbers, ",")
}

func resourceMilli(c any, what string) int {
	v, _ := ParseCPU(field(c, "resources", what, "cpu"))
	return int(v + 0.5)
}

func resourceMebi(c any, what string) int {
	v, _ := ParseMem(field(c, "resources", what, "memory"))
	return int(v/(1<<20) + 0.5)
}

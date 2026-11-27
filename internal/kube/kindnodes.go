package kube

import (
	"strconv"
	"strings"
)

type KindNodes struct {
	Planes, Workers, Running, Total int
	Version, Uptime                 string
	Containers                      []string
}

func DockerKindNodesArgs() []string {
	return []string{"ps", "-a",
		"--filter", "label=io.x-k8s.kind.cluster",
		"--format", "{{.Label \"io.x-k8s.kind.cluster\"}}\t{{.Label \"io.x-k8s.kind.role\"}}\t{{.Image}}\t{{.State}}\t{{.Status}}\t{{.ID}}",
	}
}

func DockerControlPlaneLogsArgs(cluster string) []string {
	return []string{"logs", "--tail", "200", "-f", cluster + "-control-plane"}
}

func ParseKindNodes(out string) map[string]KindNodes {
	got := map[string]KindNodes{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 1 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		n := got[name]
		n.Total++
		role, image, state, status, id := "", "", "", "", ""
		if len(fields) > 1 {
			role = strings.ToLower(strings.TrimSpace(fields[1]))
		}
		if len(fields) > 2 {
			image = strings.TrimSpace(fields[2])
		}
		if len(fields) > 3 {
			state = strings.ToLower(strings.TrimSpace(fields[3]))
		}
		if len(fields) > 4 {
			status = strings.TrimSpace(fields[4])
		}
		if len(fields) > 5 {
			id = strings.TrimSpace(fields[5])
		}
		isCP := strings.Contains(role, "control")
		switch {
		case isCP:
			n.Planes++
		case strings.Contains(role, "worker"):
			n.Workers++
		}
		if state == "running" {
			n.Running++
			if id != "" {
				n.Containers = append(n.Containers, id)
			}
			if up := KindUptime(status); up != "" && (n.Uptime == "" || isCP) {
				n.Uptime = up
			}
		}
		if n.Version == "" {
			n.Version = KindImageVersion(image)
		}
		got[name] = n
	}
	return got
}

func KindImageVersion(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if i := strings.LastIndex(image, "@"); i >= 0 {
		image = image[:i]
	}
	i := strings.LastIndex(image, ":")
	if i < 0 || i == len(image)-1 {
		return ""
	}
	tag := image[i+1:]
	if strings.Contains(tag, "/") {
		return ""
	}
	if tag[0] == 'v' || (tag[0] >= '0' && tag[0] <= '9') {
		return tag
	}
	return ""
}

func KindUptime(status string) string {
	s := strings.TrimSpace(status)
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "up ") {
		return ""
	}
	rest := strings.TrimSpace(s[3:])
	if i := strings.Index(rest, "("); i >= 0 {
		rest = strings.TrimSpace(rest[:i])
	}
	rest = strings.ToLower(rest)
	switch {
	case strings.HasPrefix(rest, "about an hour") || rest == "an hour":
		return "up 1h"
	case strings.HasPrefix(rest, "about a minute"):
		return "up 1m"
	case strings.Contains(rest, "less than a second"):
		return "up 0s"
	}
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return ""
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n < 0 {
		return ""
	}
	unit := fields[1]
	switch {
	case strings.HasPrefix(unit, "sec"):
		return "up " + strconv.Itoa(n) + "s"
	case strings.HasPrefix(unit, "min"):
		return "up " + strconv.Itoa(n) + "m"
	case strings.HasPrefix(unit, "hour"):
		return "up " + strconv.Itoa(n) + "h"
	case strings.HasPrefix(unit, "day"):
		return "up " + strconv.Itoa(n) + "d"
	case strings.HasPrefix(unit, "week"):
		return "up " + strconv.Itoa(n) + "w"
	case strings.HasPrefix(unit, "month"):
		return "up " + strconv.Itoa(n) + "mo"
	}
	return ""
}

func (n KindNodes) Summary() string {
	var parts []string
	if n.Planes > 0 || n.Workers > 0 || n.Total > 0 {
		cp, wr := n.Planes, n.Workers
		if cp == 0 && wr == 0 && n.Total > 0 {
			cp = n.Total
		}
		s := strconv.Itoa(cp) + " cp"
		if wr == 1 {
			s += " · 1 worker"
		} else {
			s += " · " + strconv.Itoa(wr) + " workers"
		}
		parts = append(parts, s)
	}
	if n.Version != "" {
		parts = append(parts, n.Version)
	}
	if n.Total > 0 && n.Running != n.Total {
		parts = append(parts, strconv.Itoa(n.Running)+"/"+strconv.Itoa(n.Total))
	}
	return strings.Join(parts, " · ")
}

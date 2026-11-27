package kube

import (
	"strconv"
	"strings"
)

type KindNodes struct {
	Planes, Workers, Running, Total int
	Version                         string
}

func DockerKindNodesArgs() []string {
	return []string{"ps", "-a",
		"--filter", "label=io.x-k8s.kind.cluster",
		"--format", "{{.Label \"io.x-k8s.kind.cluster\"}}\t{{.Label \"io.x-k8s.kind.role\"}}\t{{.Image}}\t{{.State}}\t{{.Status}}",
	}
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
		role, image, state := "", "", ""
		if len(fields) > 1 {
			role = strings.ToLower(strings.TrimSpace(fields[1]))
		}
		if len(fields) > 2 {
			image = strings.TrimSpace(fields[2])
		}
		if len(fields) > 3 {
			state = strings.ToLower(strings.TrimSpace(fields[3]))
		}
		switch {
		case strings.Contains(role, "control"):
			n.Planes++
		case strings.Contains(role, "worker"):
			n.Workers++
		}
		if state == "running" {
			n.Running++
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

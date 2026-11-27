package kube

import (
	"strconv"
	"strings"
	"time"
)

type KindNodes struct {
	Planes, Workers, Running, Total int
	Version, Uptime                 string
	StartedAt                       time.Time
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
	return ParseKindNodesAt(out, time.Now())
}

func ParseKindNodesAt(out string, at time.Time) map[string]KindNodes {
	if at.IsZero() {
		at = time.Now()
	}
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
			if d, ok := ParseKindUptime(status); ok && (n.Uptime == "" || isCP) {
				n.Uptime = FormatKindUptime(d)
				n.StartedAt = at.Add(-d)
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
	d, ok := ParseKindUptime(status)
	if !ok {
		return ""
	}
	return FormatKindUptime(d)
}

func ParseKindUptime(status string) (time.Duration, bool) {
	s := strings.TrimSpace(status)
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "up ") {
		return 0, false
	}
	rest := strings.TrimSpace(s[3:])
	if i := strings.Index(rest, "("); i >= 0 {
		rest = strings.TrimSpace(rest[:i])
	}
	rest = strings.ToLower(rest)
	switch {
	case strings.HasPrefix(rest, "about an hour") || rest == "an hour":
		return time.Hour, true
	case strings.HasPrefix(rest, "about a minute"):
		return time.Minute, true
	case strings.Contains(rest, "less than a second"):
		return 0, true
	}
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n < 0 {
		return 0, false
	}
	unit := fields[1]
	switch {
	case strings.HasPrefix(unit, "sec"):
		return time.Duration(n) * time.Second, true
	case strings.HasPrefix(unit, "min"):
		return time.Duration(n) * time.Minute, true
	case strings.HasPrefix(unit, "hour"):
		return time.Duration(n) * time.Hour, true
	case strings.HasPrefix(unit, "day"):
		return time.Duration(n) * 24 * time.Hour, true
	case strings.HasPrefix(unit, "week"):
		return time.Duration(n) * 7 * 24 * time.Hour, true
	case strings.HasPrefix(unit, "month"):
		return time.Duration(n) * 30 * 24 * time.Hour, true
	}
	return 0, false
}

func FormatKindUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "up 0s"
	case d < time.Minute:
		return "up " + strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return "up " + strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return "up " + strconv.Itoa(int(d.Hours())) + "h"
	case d < 14*24*time.Hour:
		return "up " + strconv.Itoa(int(d.Hours())/24) + "d"
	case d < 60*24*time.Hour:
		return "up " + strconv.Itoa(int(d.Hours())/24/7) + "w"
	default:
		return "up " + strconv.Itoa(int(d.Hours())/24/30) + "mo"
	}
}

func kindUptimeUnit(d time.Duration) time.Duration {
	switch {
	case d >= time.Hour:
		return time.Hour
	case d >= time.Minute:
		return time.Minute
	default:
		return time.Second
	}
}

func KeepKindStartedAt(old, derived, at time.Time) time.Time {
	if derived.IsZero() {
		return time.Time{}
	}
	if old.IsZero() {
		return derived
	}
	age := at.Sub(derived)
	if age < 0 {
		age = 0
	}
	unit := kindUptimeUnit(age)
	cur := at.Sub(old)
	if cur >= age && cur < age+unit {
		return old
	}
	return derived
}

func KeepKindStartedAts(prev, next map[string]KindNodes, at time.Time) {
	if next == nil {
		return
	}
	for name, n := range next {
		var old time.Time
		if prev != nil {
			old = prev[name].StartedAt
		}
		n.StartedAt = KeepKindStartedAt(old, n.StartedAt, at)
		next[name] = n
	}
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

package kube

import (
	"strconv"
	"strings"
)

type DockerStat struct {
	ID, Name          string
	CPU, MemPct       float64
	MemUsed, MemLimit int64
}

type KindUsage struct {
	CPU     float64
	MemUsed int64
	MemPct  float64
	OK      bool
}

func DockerStatsArgs(ids []string) []string {
	args := []string{"stats", "--no-stream", "--format", "{{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}"}
	return append(args, ids...)
}

func ParseDockerStats(out string) []DockerStat {
	var rows []DockerStat
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		low := strings.ToLower(strings.TrimSpace(f[0]))
		if low == "container" || low == "id" || strings.HasPrefix(low, "container") {
			continue
		}
		s := DockerStat{ID: strings.TrimSpace(f[0]), Name: strings.TrimSpace(f[1])}
		s.CPU, _ = ParseDockerCPU(f[2])
		if len(f) > 3 {
			s.MemUsed, s.MemLimit = ParseDockerMemUsage(f[3])
		}
		if len(f) > 4 {
			s.MemPct, _ = ParseDockerCPU(f[4])
		} else if s.MemLimit > 0 {
			s.MemPct = float64(s.MemUsed) / float64(s.MemLimit) * 100
		}
		rows = append(rows, s)
	}
	return rows
}

func ParseDockerCPU(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" || s == "--" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func ParseDockerMemUsage(s string) (used, limit int64) {
	s = strings.TrimSpace(s)
	a, b, ok := strings.Cut(s, "/")
	used, _ = ParseDockerBytes(a)
	if ok {
		limit, _ = ParseDockerBytes(b)
	}
	return used, limit
}

func ParseDockerBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0, false
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.ToLower(strings.TrimSpace(s[i:]))
	var mul float64
	switch unit {
	case "", "b":
		mul = 1
	case "kb", "k":
		mul = 1000
	case "kib", "ki":
		mul = 1024
	case "mb", "m":
		mul = 1000 * 1000
	case "mib", "mi":
		mul = 1024 * 1024
	case "gb", "g":
		mul = 1000 * 1000 * 1000
	case "gib", "gi":
		mul = 1024 * 1024 * 1024
	case "tb", "t":
		mul = 1000 * 1000 * 1000 * 1000
	case "tib", "ti":
		mul = 1024 * 1024 * 1024 * 1024
	default:
		return 0, false
	}
	return int64(v*mul + 0.5), true
}

func FormatCPU(p float64) string {
	if p < 10 {
		return strconv.FormatFloat(p, 'f', 1, 64) + "%"
	}
	return strconv.FormatFloat(p, 'f', 0, 64) + "%"
}

func FormatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	type unit struct {
		name string
		size int64
	}
	for _, u := range []unit{{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}} {
		if n >= u.size {
			v := float64(n) / float64(u.size)
			if v >= 10 {
				return strconv.FormatFloat(v, 'f', 0, 64) + " " + u.name
			}
			return strconv.FormatFloat(v, 'f', 1, 64) + " " + u.name
		}
	}
	return strconv.FormatInt(n, 10) + " B"
}

func ContainerIDMatch(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	if have == want {
		return true
	}
	if len(have) >= 12 && len(want) >= 12 {
		n := 12
		if len(have) < n {
			n = len(have)
		}
		if len(want) < n {
			n = len(want)
		}
		return have[:n] == want[:n]
	}
	return strings.HasPrefix(have, want) || strings.HasPrefix(want, have)
}

func SumDockerStats(stats []DockerStat) KindUsage {
	var u KindUsage
	var pctSum float64
	var pctN int
	for _, s := range stats {
		u.CPU += s.CPU
		u.MemUsed += s.MemUsed
		if s.MemPct > 0 || s.MemLimit > 0 {
			pctSum += s.MemPct
			pctN++
		}
	}
	if pctN > 0 {
		u.MemPct = pctSum / float64(pctN)
	}
	u.OK = len(stats) > 0
	return u
}

func UsageByCluster(stats []DockerStat, nodes map[string]KindNodes) map[string]KindUsage {
	idOf := func(id string) string {
		for cluster, n := range nodes {
			for _, have := range n.Containers {
				if ContainerIDMatch(have, id) {
					return cluster
				}
			}
		}
		return ""
	}
	groups := map[string][]DockerStat{}
	for _, s := range stats {
		c := idOf(s.ID)
		if c == "" {
			c = idOf(s.Name)
		}
		if c == "" {
			continue
		}
		groups[c] = append(groups[c], s)
	}
	out := map[string]KindUsage{}
	for c, rows := range groups {
		out[c] = SumDockerStats(rows)
	}
	return out
}

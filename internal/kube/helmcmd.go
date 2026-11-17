package kube

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k2/internal/shell"
)

type HelmChart struct {
	Name, Version, AppVersion, Description string
}

func ParseHelmSearch(out string) []HelmChart {
	var rows []HelmChart
	for i, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if i == 0 && strings.EqualFold(f[0], "NAME") {
			continue
		}
		low := strings.ToLower(f[0])
		if strings.HasPrefix(low, "warning:") || strings.HasPrefix(low, "error:") {
			continue
		}
		c := HelmChart{Name: f[0], Version: f[1]}
		if len(f) > 2 {
			c.AppVersion = f[2]
		}
		if len(f) > 3 {
			c.Description = strings.Join(f[3:], " ")
		}
		rows = append(rows, c)
	}
	return rows
}

func ChartNames(charts []HelmChart) []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range charts {
		if c.Name == "" || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}

func ChartVersions(charts []HelmChart, name string) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range charts {
		if name != "" && c.Name != name {
			continue
		}
		if c.Version == "" || seen[c.Version] {
			continue
		}
		seen[c.Version] = true
		out = append(out, c.Version)
	}
	return out
}

func ManifestKindCounts(manifest string) string {
	counts := map[string]int{}
	var order []string
	for _, doc := range strings.Split(manifest, "\n---") {
		kind := manifestKind(doc)
		if kind == "" {
			continue
		}
		if counts[kind] == 0 {
			order = append(order, kind)
		}
		counts[kind]++
	}
	if len(order) == 0 {
		return "no resources"
	}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		parts = append(parts, k+" ×"+strconv.Itoa(counts[k]))
	}
	return strings.Join(parts, " · ")
}

func manifestKind(doc string) string {
	for _, l := range strings.Split(doc, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "kind:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "kind:")), `"'`)
		}
	}
	return ""
}

func HelmTemplateArgs(name, chart, version, namespace, values, kubeconfig, context string) []string {
	args := []string{"template", name, chart}
	args = append(args, HelmArgs(kubeconfig, context, namespace)...)
	if version != "" {
		args = append(args, "--version", version)
	}
	if values != "" {
		args = append(args, "-f", values)
	}
	return args
}

func HelmApplyArgs(upgrade bool, name, chart, version, namespace string, createNS bool, values, kubeconfig, context string) []string {
	verb := "install"
	if upgrade {
		verb = "upgrade"
	}
	args := []string{verb, name, chart}
	args = append(args, HelmArgs(kubeconfig, context, namespace)...)
	if version != "" {
		args = append(args, "--version", version)
	}
	if values != "" {
		args = append(args, "-f", values)
	}
	if createNS && !upgrade {
		args = append(args, "--create-namespace")
	}
	return args
}

func HelmSearchArgs(query string, versions bool) []string {
	args := []string{"search", "repo"}
	if query != "" {
		args = append(args, query)
	}
	if versions {
		args = append(args, "--versions")
	}
	return args
}

func HelmShowValuesArgs(chart, version string) []string {
	args := []string{"show", "values", chart}
	if version != "" {
		args = append(args, "--version", version)
	}
	return args
}

func ChartAt(chart, version string) string {
	if version == "" {
		return chart
	}
	return chart + "@" + version
}

func UpgradeDiff(current HelmRelease, manifest, chart, version string) string {
	next := current
	next.Revision = current.Revision + 1
	next.Manifest = manifest
	if chart != "" {
		if version != "" {
			next.Chart = chartName(chart) + "-" + version
		} else {
			next.Chart = chart
		}
	}
	return RevisionDiff(current, next)
}

func chartName(chart string) string {
	if i := strings.LastIndex(chart, "/"); i >= 0 {
		return chart[i+1:]
	}
	return chart
}

func (c Client) HelmOut(bin string, args ...string) (string, error) {
	out, err := shell.Combined(c.Run, bin, args...)
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("%s", text)
	}
	return string(out), nil
}

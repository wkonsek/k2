package kube

import (
	"sort"
	"strings"

	"k2/internal/shell"
)

type APIResource struct {
	Name       string
	Short      []string
	Group      string
	Version    string
	Namespaced bool
	Kind       string
}

func (a APIResource) Qualified() string {
	if a.Group == "" {
		return a.Name
	}
	return a.Name + "." + a.Group
}

func (a APIResource) GVR() string {
	if a.Group == "" {
		return a.Version + "/" + a.Name
	}
	return a.Group + "/" + a.Version + "/" + a.Name
}

func (a APIResource) matches(q string) bool {
	if q == a.Name || q == strings.ToLower(a.Kind) {
		return true
	}
	for _, s := range a.Short {
		if q == s {
			return true
		}
	}
	return false
}

func (c Client) APIList(kubeconfig, context string) ([]APIResource, error) {
	out, err := shell.Output(c.Run, "kubectl", "api-resources", "--no-headers", "--verbs=get", "--kubeconfig", kubeconfig, "--context", context)
	list := ParseAPIList(string(out))
	if err != nil && len(list) == 0 {
		return nil, err
	}
	return list, nil
}

func ParseAPIList(out string) []APIResource {
	var list []APIResource
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.ToLower(line))
		nsIdx := -1
		for i := len(f) - 1; i >= 2; i-- {
			if f[i] == "true" || f[i] == "false" {
				nsIdx = i
				break
			}
		}
		if nsIdx < 2 {
			continue
		}
		a := APIResource{Name: f[0], Namespaced: f[nsIdx] == "true"}
		if nsIdx+1 < len(f) {
			a.Kind = strings.Fields(line)[nsIdx+1]
		}
		a.Version = f[nsIdx-1]
		if g, v, ok := strings.Cut(a.Version, "/"); ok {
			a.Group, a.Version = g, v
		}
		for _, s := range strings.Split(strings.Join(f[1:nsIdx-1], ","), ",") {
			if s != "" {
				a.Short = append(a.Short, s)
			}
		}
		list = append(list, a)
	}
	return list
}

func ResourceNames(list []APIResource) []string {
	seen := map[string]bool{}
	var names []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			names = append(names, s)
		}
	}
	for _, a := range list {
		add(a.Name)
		for _, s := range a.Short {
			add(s)
		}
		if a.Group != "" {
			add(a.Qualified())
			add(a.GVR())
		}
	}
	sort.Strings(names)
	return names
}

func IsVersion(s string) bool {
	if len(s) < 2 || s[0] != 'v' || s[1] < '0' || s[1] > '9' {
		return false
	}
	rest := strings.TrimLeft(s[1:], "0123456789")
	for _, tag := range []string{"alpha", "beta"} {
		if t, ok := strings.CutPrefix(rest, tag); ok {
			return t != "" && strings.TrimLeft(t, "0123456789") == ""
		}
	}
	return rest == ""
}

func SplitKind(kind string) (resource, version, group string) {
	k := strings.ToLower(strings.TrimSpace(kind))
	if strings.Contains(k, "/") {
		p := strings.Split(k, "/")
		switch {
		case len(p) == 3:
			return p[2], p[1], p[0]
		case len(p) == 2 && IsVersion(p[0]):
			return p[1], p[0], ""
		case len(p) == 2:
			return p[1], "", p[0]
		}
		return "", "", ""
	}
	resource, rest, _ := strings.Cut(k, ".")
	if v, g, ok := strings.Cut(rest, "."); ok && IsVersion(v) {
		return resource, v, g
	}
	return resource, "", rest
}

func kubectlKind(resource, version, group string) string {
	switch {
	case group == "":
		return resource
	case version == "":
		return resource + "." + group
	}
	return resource + "." + version + "." + group
}

func groupRank(g string) int {
	switch {
	case g == "":
		return 0
	case strings.HasSuffix(g, ".k8s.io"):
		return 1
	}
	return 2
}

func Resolve(q string, list []APIResource) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	qualified := strings.ContainsAny(q, "./")
	res, ver, grp := q, "", ""
	if qualified {
		res, ver, grp = SplitKind(q)
		if res == "" {
			return nil
		}
		if len(list) == 0 {
			return []string{kubectlKind(res, ver, grp)}
		}
	}
	var hits []APIResource
	seen := map[string]bool{}
	for _, a := range list {
		if !a.matches(res) || (qualified && a.Group != grp) {
			continue
		}
		if seen[a.Qualified()] {
			continue
		}
		seen[a.Qualified()] = true
		hits = append(hits, a)
	}
	switch {
	case len(hits) == 0:
		return nil
	case qualified:
		return []string{kubectlKind(hits[0].Name, ver, grp)}
	case len(hits) == 1:
		return []string{q}
	}
	for _, a := range hits {
		if a.Group == "" {
			return []string{q}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return groupRank(hits[i].Group) < groupRank(hits[j].Group) })
	out := make([]string, len(hits))
	for i, a := range hits {
		out[i] = a.Qualified()
	}
	return out
}

package kube

import (
	"sort"
	"strings"
)

type groupSpec struct {
	group, plural, kind string
	spec                spec
}

func strs(o Object, path ...string) []string {
	var out []string
	for _, v := range o.List(path...) {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func joinUniq(ss []string) string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}

func gatewayAddress(o Object) string {
	var out []string
	for _, a := range o.List("status", "addresses") {
		out = append(out, field(a, "value"))
	}
	return joinUniq(out)
}

func istioSelector(o Object) string {
	sel, _ := o.Get("spec", "selector").(map[string]any)
	var out []string
	for k, v := range sel {
		s, _ := v.(string)
		out = append(out, k+"="+s)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func istioHosts(o Object) string {
	var out []string
	for _, s := range o.List("spec", "servers") {
		out = append(out, strs(Object{Raw: map[string]any{"s": s}}, "s", "hosts")...)
	}
	return joinUniq(out)
}

func routeParents(o Object) string {
	var out []string
	for _, p := range o.List("spec", "parentRefs") {
		n := field(p, "name")
		if ns := field(p, "namespace"); ns != "" {
			n = ns + "/" + n
		}
		out = append(out, n)
	}
	return joinUniq(out)
}

func cond(typ string) func(Object) string {
	return func(o Object) string { return o.condition(typ) }
}

var groupSpecs = []groupSpec{
	{"gateway.networking.k8s.io", "gateways", "Gateway", spec{cols: []column{
		{"CLASS", func(o Object) string { return o.Str("spec", "gatewayClassName") }},
		{"ADDRESS", gatewayAddress},
		{"PROGRAMMED", cond("Programmed")},
	}}},
	{"gateway.networking.k8s.io", "gatewayclasses", "GatewayClass", spec{cols: []column{
		{"CONTROLLER", func(o Object) string { return o.Str("spec", "controllerName") }},
		{"ACCEPTED", cond("Accepted")},
	}}},
	{"gateway.networking.k8s.io", "httproutes", "HTTPRoute", spec{cols: []column{
		{"HOSTNAMES", func(o Object) string { return joinUniq(strs(o, "spec", "hostnames")) }},
		{"PARENTS", routeParents},
	}}},
	{"networking.istio.io", "gateways", "Gateway", spec{cols: []column{
		{"SELECTOR", istioSelector},
		{"HOSTS", istioHosts},
	}}},
	{"networking.istio.io", "virtualservices", "VirtualService", spec{cols: []column{
		{"GATEWAYS", func(o Object) string { return joinUniq(strs(o, "spec", "gateways")) }},
		{"HOSTS", func(o Object) string { return joinUniq(strs(o, "spec", "hosts")) }},
	}}},
}

func apiGroup(apiVersion string) string {
	if g, _, ok := strings.Cut(apiVersion, "/"); ok {
		return g
	}
	return ""
}

func HasTableSpec(kind string) bool {
	base := BaseKind(kind)
	if _, ok := specs[base]; ok {
		return true
	}
	switch base {
	case "pods", "events", "secrets", "secret":
		return true
	}
	res, _, grp := SplitKind(kind)
	if res == "" {
		res = base
	}
	for _, s := range groupSpecs {
		if s.plural == res && (grp == "" || s.group == grp || strings.HasSuffix(kind, "."+s.group) || strings.Contains(kind, s.group)) {
			return true
		}
		if strings.EqualFold(s.kind, res) {
			return true
		}
	}
	return false
}

func specFor(kind string, objs []Object) (spec, bool) {
	if len(objs) > 0 {
		k, g := objs[0].Str("kind"), apiGroup(objs[0].Str("apiVersion"))
		for _, s := range groupSpecs {
			if s.kind == k && s.group == g {
				return s.spec, true
			}
		}
		return spec{}, false
	}
	res, _, grp := SplitKind(kind)
	var hits []groupSpec
	for _, s := range groupSpecs {
		if (grp == "" || s.group == grp) && (res == s.plural || res == strings.ToLower(s.kind)) {
			hits = append(hits, s)
		}
	}
	if len(hits) != 1 {
		return spec{}, false
	}
	return hits[0].spec, true
}

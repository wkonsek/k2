package kube

import (
	"encoding/json"
	"fmt"
	"strings"

	"k2/internal/shell"
)

func init() {
	specs["subscriptions"] = spec{cols: []column{
		{"PACKAGE", func(o Object) string { return o.Str("spec", "name") }},
		{"CHANNEL", func(o Object) string { return o.Str("spec", "channel") }},
		{"SOURCE", func(o Object) string { return o.Str("spec", "source") }},
		{"INSTALLED CSV", func(o Object) string { return o.Str("status", "installedCSV") }},
		{"CURRENT CSV", func(o Object) string { return o.Str("status", "currentCSV") }},
		{"STATE", func(o Object) string { return o.Str("status", "state") }},
	}, status: "STATE", extra: func(o Object) map[string]string {
		return map[string]string{"csv": or(o.Str("status", "installedCSV"), o.Str("status", "currentCSV"))}
	}}
	specs["clusterserviceversions"] = spec{cols: []column{
		{"DISPLAY", func(o Object) string { return o.Str("spec", "displayName") }},
		{"VERSION", func(o Object) string { return o.Str("spec", "version") }},
		{"REPLACES", func(o Object) string { return o.Str("spec", "replaces") }},
		{"PHASE", func(o Object) string { return o.Str("status", "phase") }},
	}, status: "PHASE"}
	specs["installplans"] = spec{cols: []column{
		{"CSV", func(o Object) string { return joinList(o.List("spec", "clusterServiceVersionNames")) }},
		{"APPROVAL", func(o Object) string { return o.Str("spec", "approval") }},
		{"APPROVED", func(o Object) string { return or(o.Str("spec", "approved"), "false") }},
		{"PHASE", func(o Object) string { return o.Str("status", "phase") }},
	}, status: "PHASE", extra: func(o Object) map[string]string {
		return map[string]string{"approval": o.Str("spec", "approval"), "approved": or(o.Str("spec", "approved"), "false")}
	}}
	specs["catalogsources"] = spec{cols: []column{
		{"DISPLAY", func(o Object) string { return o.Str("spec", "displayName") }},
		{"TYPE", func(o Object) string { return o.Str("spec", "sourceType") }},
		{"PUBLISHER", func(o Object) string { return o.Str("spec", "publisher") }},
		{"CONNECTION", func(o Object) string { return o.Str("status", "connectionState", "lastObservedState") }},
	}, status: "CONNECTION"}
	specs["operatorgroups"] = spec{cols: []column{
		{"TARGET NAMESPACES", func(o Object) string {
			if ns := joinList(o.List("spec", "targetNamespaces")); ns != "" {
				return ns
			}
			if o.Get("spec", "selector") != nil {
				return "(selector)"
			}
			return "(all)"
		}},
		{"UPGRADE", func(o Object) string { return or(o.Str("spec", "upgradeStrategy", "name"), "Default") }},
	}}
	specs["customresourcedefinitions"] = spec{cols: []column{
		{"GROUP", func(o Object) string { return o.Str("spec", "group") }},
		{"KIND", func(o Object) string { return o.Str("spec", "names", "kind") }},
		{"VERSIONS", crdVersions},
		{"SCOPE", func(o Object) string { return o.Str("spec", "scope") }},
	}, extra: func(o Object) map[string]string {
		return map[string]string{"resource": o.Str("spec", "names", "plural") + "." + o.Str("spec", "group"), "scope": o.Str("spec", "scope")}
	}}
	specs["routes"] = spec{cols: []column{
		{"HOST", RouteURL},
		{"PATH", func(o Object) string { return o.Str("spec", "path") }},
		{"SERVICE", func(o Object) string { return o.Str("spec", "to", "name") }},
		{"PORT", func(o Object) string { return o.Str("spec", "port", "targetPort") }},
		{"TLS", func(o Object) string {
			t := o.Str("spec", "tls", "termination")
			if p := o.Str("spec", "tls", "insecureEdgeTerminationPolicy"); t != "" && p != "" {
				t += "/" + p
			}
			return t
		}},
	}, extra: func(o Object) map[string]string { return map[string]string{"url": RouteURL(o)} }}
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func joinList(l []any) string {
	var out []string
	for _, v := range l {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}

func crdVersions(o Object) string {
	var out []string
	for _, v := range o.List("spec", "versions") {
		if field(v, "served") == "false" {
			continue
		}
		name := field(v, "name")
		if field(v, "storage") == "true" {
			name += "*"
		}
		out = append(out, name)
	}
	return strings.Join(out, ",")
}

func RouteURL(o Object) string {
	host := o.Str("spec", "host")
	if host == "" {
		host = o.Str("status", "ingress", "0", "host")
	}
	if host == "" {
		return ""
	}
	scheme := "http://"
	if o.Get("spec", "tls") != nil {
		scheme = "https://"
	}
	return scheme + host + o.Str("spec", "path")
}

func (c Client) Object(kubeconfig, context, kind, namespace, name string) (Object, error) {
	args := []string{"get", kind, name, "-o", "json", "--kubeconfig", kubeconfig, "--context", context}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil {
		return Object{}, fmt.Errorf("%s", shell.FailText(out, err))
	}
	var o Object
	if json.Unmarshal(out, &o) != nil || json.Unmarshal(out, &o.Raw) != nil {
		return Object{}, fmt.Errorf("bad kubectl json")
	}
	return o, nil
}

func CSVSummary(o Object) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	line("%s  %s %s", o.Metadata.Name, o.Str("spec", "displayName"), o.Str("spec", "version"))
	line("phase: %s", or(o.Str("status", "phase"), "-"))
	if r := o.Str("status", "reason"); r != "" {
		line("reason: %s", r)
	}
	if msg := o.Str("status", "message"); msg != "" {
		line("message: %s", msg)
	}
	if r := o.Str("spec", "replaces"); r != "" {
		line("replaces: %s", r)
	}
	var modes []string
	for _, im := range o.List("spec", "installModes") {
		if field(im, "supported") == "true" {
			modes = append(modes, field(im, "type"))
		}
	}
	if len(modes) > 0 {
		line("install modes: %s", strings.Join(modes, ", "))
	}
	section := func(title string, items []string) {
		line("")
		line("%s (%d)", title, len(items))
		if len(items) == 0 {
			line("  none")
		}
		for _, it := range items {
			line("  %s", it)
		}
	}
	crds := func(path ...string) []string {
		var out []string
		for _, c := range o.List(path...) {
			out = append(out, fmt.Sprintf("%s  (%s %s)", field(c, "name"), field(c, "kind"), field(c, "version")))
		}
		return out
	}
	section("owned CRDs", crds("spec", "customresourcedefinitions", "owned"))
	if req := crds("spec", "customresourcedefinitions", "required"); len(req) > 0 {
		section("required CRDs", req)
	}
	var apis []string
	for _, a := range o.List("spec", "apiservicedefinitions", "owned") {
		apis = append(apis, fmt.Sprintf("%s.%s  (%s %s)", field(a, "version"), field(a, "group"), field(a, "kind"), field(a, "deploymentName")))
	}
	if len(apis) > 0 {
		section("owned API services", apis)
	}
	var deps []string
	for _, d := range o.List("spec", "install", "spec", "deployments") {
		deps = append(deps, field(d, "name"))
	}
	section("deployments", deps)
	var conds []string
	for _, c := range o.List("status", "conditions") {
		conds = append(conds, strings.TrimSpace(fmt.Sprintf("%s  %s  %s  %s", field(c, "lastTransitionTime"), field(c, "phase"), field(c, "reason"), field(c, "message"))))
	}
	if len(conds) > 0 {
		section("conditions", conds)
	}
	line("")
	line("yaml: kubectl get csv %s -n %s -o yaml  (or e to edit)", o.Metadata.Name, o.Metadata.Namespace)
	return b.String()
}

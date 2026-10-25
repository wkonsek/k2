package kube

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"
)

type Node struct {
	Kind      string
	Namespace string
	Name      string
	Pod       string
	Init      bool
	Status    string
	Ready     string
	Restarts  int
	Born      time.Time
	Children  []*Node
}

func (n *Node) ID() string {
	if n.Kind == "container" {
		return "container/" + n.Namespace + "/" + n.Pod + "/" + n.Name
	}
	return n.Kind + "/" + n.Namespace + "/" + n.Name
}

func (n *Node) Label() string {
	if l, ok := xrayLabels[n.Kind]; ok {
		return l
	}
	return n.Kind
}

var xrayLabels = map[string]string{
	"deploy": "Deployment", "rs": "ReplicaSet", "pod": "Pod", "svc": "Service", "ing": "Ingress",
	"sts": "StatefulSet", "ds": "DaemonSet", "job": "Job", "cronjob": "CronJob",
	"cm": "ConfigMap", "secret": "Secret", "pvc": "PVC", "container": "container",
}

var xrayKinds = map[string]string{
	"deployments": "deploy", "statefulsets": "sts", "daemonsets": "ds", "jobs": "job",
	"cronjobs": "cronjob", "pods": "pod", "replicasets": "rs", "services": "svc",
	"svc": "svc", "service": "svc",
}

func XrayKind(kind string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(kind))
	if b, ok := Builtin[k]; ok {
		k = b
	}
	t, ok := xrayKinds[BaseKind(k)]
	if !ok {
		t, ok = xrayKinds[k]
	}
	return t, ok
}

var XrayKindNames = []string{"deployments", "statefulsets", "daemonsets", "replicasets", "jobs", "cronjobs", "pods", "services"}

const xrayList = "pods,replicasets,deployments,statefulsets,daemonsets,jobs,cronjobs,services,ingresses"

func (c Client) XrayObjects(kubeconfig, context, namespace string) ([]Object, error) {
	args := []string{"get", xrayList, "-o", "json", "--kubeconfig", kubeconfig, "--context", context}
	if namespace == "" {
		args = append(args, "-A")
	} else {
		args = append(args, "-n", namespace)
	}
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil {
		return nil, cmdError(out, err)
	}
	return ParseObjects(out)
}

func kindOf(o Object) string {
	switch o.Str("kind") {
	case "Deployment":
		return "deploy"
	case "ReplicaSet":
		return "rs"
	case "Pod":
		return "pod"
	case "Service":
		return "svc"
	case "Ingress":
		return "ing"
	case "StatefulSet":
		return "sts"
	case "DaemonSet":
		return "ds"
	case "Job":
		return "job"
	case "CronJob":
		return "cronjob"
	}
	return ""
}

func ownerKey(o Object) string {
	refs := o.List("metadata", "ownerReferences")
	pick := -1
	for i, r := range refs {
		if c, _ := asMap(r)["controller"].(bool); c {
			pick = i
			break
		}
	}
	if pick < 0 && len(refs) > 0 {
		pick = 0
	}
	if pick < 0 {
		return ""
	}
	m := asMap(refs[pick])
	k := kindOf(Object{Raw: map[string]any{"kind": m["kind"]}})
	name, _ := m["name"].(string)
	if k == "" || name == "" {
		return ""
	}
	return k + "/" + o.Metadata.Namespace + "/" + name
}

func labelsOf(v any) map[string]string {
	out := map[string]string{}
	m, _ := v.(map[string]any)
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

func selects(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func BuildXray(objs []Object, rootKind, name string) []*Node {
	children := map[string][]Object{}
	var pods, svcs, ings []Object
	for _, o := range objs {
		k := kindOf(o)
		if k == "" {
			continue
		}
		if ok := ownerKey(o); ok != "" {
			children[ok] = append(children[ok], o)
		}
		switch k {
		case "pod":
			pods = append(pods, o)
		case "svc":
			svcs = append(svcs, o)
		case "ing":
			ings = append(ings, o)
		}
	}
	var build func(o Object) *Node
	build = func(o Object) *Node {
		k := kindOf(o)
		n := objNode(k, o)
		key := n.Kind + "/" + o.Metadata.Namespace + "/" + o.Metadata.Name
		if k == "pod" {
			n.Children = podChildren(o)
			return n
		}
		if k == "svc" {
			for _, ing := range ings {
				if ing.Metadata.Namespace == o.Metadata.Namespace && ingressUses(ing, o.Metadata.Name) {
					n.Children = append(n.Children, objNode("ing", ing))
				}
			}
			for _, p := range pods {
				if p.Metadata.Namespace == o.Metadata.Namespace && selects(labelsOf(o.Get("spec", "selector")), labelsOf(p.Get("metadata", "labels"))) {
					n.Children = append(n.Children, build(p))
				}
			}
			return n
		}
		owned := children[key]
		sortObjs(owned)
		for _, c := range owned {
			n.Children = append(n.Children, build(c))
		}
		return n
	}
	var roots []*Node
	for _, o := range objs {
		k := kindOf(o)
		if k != rootKind || (name != "" && o.Metadata.Name != name) {
			continue
		}
		root := build(o)
		if k == "deploy" || k == "sts" || k == "ds" {

			tmpl := labelsOf(o.Get("spec", "template", "metadata", "labels"))
			var front []*Node
			for _, s := range svcs {
				if s.Metadata.Namespace == o.Metadata.Namespace && selects(labelsOf(s.Get("spec", "selector")), tmpl) {
					sn := objNode("svc", s)
					for _, ing := range ings {
						if ing.Metadata.Namespace == s.Metadata.Namespace && ingressUses(ing, s.Metadata.Name) {
							sn.Children = append(sn.Children, objNode("ing", ing))
						}
					}
					front = append(front, sn)
				}
			}
			root.Children = append(front, root.Children...)
		}
		roots = append(roots, root)
	}
	return roots
}

func sortObjs(objs []Object) {
	sort.SliceStable(objs, func(i, j int) bool { return objs[i].Metadata.Name < objs[j].Metadata.Name })
}

func ingressUses(ing Object, svc string) bool {
	if ing.Str("spec", "defaultBackend", "service", "name") == svc {
		return true
	}
	for _, r := range ing.List("spec", "rules") {
		for _, p := range listAt(r, "http", "paths") {
			if field(p, "backend", "service", "name") == svc {
				return true
			}
		}
	}
	return false
}

func listAt(v any, path ...string) []any {
	return Object{Raw: asMap(v)}.List(path...)
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func objNode(kind string, o Object) *Node {
	n := &Node{Kind: kind, Namespace: o.Metadata.Namespace, Name: o.Metadata.Name, Born: o.Metadata.CreationTimestamp}
	switch kind {
	case "pod":
		ready, total, restarts, status := PodState(o)
		n.Status, n.Restarts = status, restarts
		if total > 0 {
			n.Ready = fmt.Sprintf("%d/%d", ready, total)
		}
	case "deploy", "sts", "rs":
		want := o.IntOr(1, "spec", "replicas")
		n.Ready = fmt.Sprintf("%d/%d", o.IntOr(0, "status", "readyReplicas"), want)
		n.Status = readyWord(o.IntOr(0, "status", "readyReplicas"), want)
	case "ds":
		want := o.IntOr(0, "status", "desiredNumberScheduled")
		n.Ready = fmt.Sprintf("%d/%d", o.IntOr(0, "status", "numberReady"), want)
		n.Status = readyWord(o.IntOr(0, "status", "numberReady"), want)
	case "job":
		n.Status = JobStatus(o)
	case "cronjob":
		if s, _ := o.Get("spec", "suspend").(bool); s {
			n.Status = "Suspended"
		} else {
			n.Status = o.Str("spec", "schedule")
		}
	case "svc":
		n.Status = o.Str("spec", "type")
	}
	return n
}

func readyWord(ready, want int) string {
	switch {
	case want == 0:
		return "ScaledDown"
	case ready >= want:
		return "Ready"
	case ready == 0:
		return "NotReady"
	}
	return "Progressing"
}

func podChildren(o Object) []*Node {
	var out []*Node
	statuses := map[string]map[string]any{}
	for _, key := range []string{"containerStatuses", "initContainerStatuses"} {
		for _, s := range o.List("status", key) {
			m := asMap(s)
			if name, _ := m["name"].(string); name != "" {
				statuses[name] = m
			}
		}
	}
	add := func(c any, init bool) {
		name := field(c, "name")
		n := &Node{Kind: "container", Namespace: o.Metadata.Namespace, Name: name, Pod: o.Metadata.Name, Init: init, Born: o.Metadata.CreationTimestamp}
		if st := statuses[name]; st != nil {
			s := Object{Raw: st}
			n.Restarts = s.IntOr(0, "restartCount")
			switch {
			case s.Str("state", "waiting", "reason") != "":
				n.Status = s.Str("state", "waiting", "reason")
			case s.Get("state", "running") != nil:
				n.Status = "Running"
			case s.Str("state", "terminated", "reason") != "":
				n.Status = s.Str("state", "terminated", "reason")
			}
			if r, _ := st["ready"].(bool); r {
				n.Ready = "1/1"
			} else if !init {
				n.Ready = "0/1"
			}
		}
		out = append(out, n)
	}
	for _, c := range o.List("spec", "containers") {
		add(c, false)
	}
	for _, c := range o.List("spec", "initContainers") {
		add(c, true)
	}
	seen := map[string]bool{}
	ref := func(kind, name string) {
		if name == "" || seen[kind+"/"+name] {
			return
		}
		seen[kind+"/"+name] = true
		out = append(out, &Node{Kind: kind, Namespace: o.Metadata.Namespace, Name: name})
	}
	var cms, secs, pvcs []string
	for _, v := range o.List("spec", "volumes") {
		cms = append(cms, field(v, "configMap", "name"))
		secs = append(secs, field(v, "secret", "secretName"))
		pvcs = append(pvcs, field(v, "persistentVolumeClaim", "claimName"))
		for _, src := range listAt(v, "projected", "sources") {
			cms = append(cms, field(src, "configMap", "name"))
			secs = append(secs, field(src, "secret", "name"))
		}
	}
	for _, key := range []string{"containers", "initContainers"} {
		for _, c := range o.List("spec", key) {
			for _, e := range listAt(c, "envFrom") {
				cms = append(cms, field(e, "configMapRef", "name"))
				secs = append(secs, field(e, "secretRef", "name"))
			}
			for _, e := range listAt(c, "env") {
				cms = append(cms, field(e, "valueFrom", "configMapKeyRef", "name"))
				secs = append(secs, field(e, "valueFrom", "secretKeyRef", "name"))
			}
		}
	}
	for _, s := range o.List("spec", "imagePullSecrets") {
		secs = append(secs, field(s, "name"))
	}
	for _, list := range []struct {
		kind  string
		names []string
	}{{"cm", cms}, {"secret", secs}, {"pvc", pvcs}} {
		sort.Strings(list.names)
		for _, n := range list.names {
			ref(list.kind, n)
		}
	}
	return out
}

type XrayRow struct {
	Node   *Node
	Prefix string
	Depth  int
	Kids   bool
}

func Flatten(roots []*Node, collapsed map[string]bool) []XrayRow {
	var out []XrayRow
	var walk func(n *Node, lead string, last bool, depth int)
	walk = func(n *Node, lead string, last bool, depth int) {
		prefix := ""
		if depth > 0 {
			branch := "├ "
			if last {
				branch = "└ "
			}
			prefix = lead + branch
		}
		out = append(out, XrayRow{Node: n, Prefix: prefix, Depth: depth, Kids: len(n.Children) > 0})
		if collapsed[n.ID()] {
			return
		}
		next := lead
		if depth > 0 {
			if last {
				next += "  "
			} else {
				next += "│ "
			}
		}
		for i, c := range n.Children {
			walk(c, next, i == len(n.Children)-1, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, "", true, 0)
	}
	return out
}

func (n *Node) RestartsText() string {
	if n.Kind != "pod" && n.Kind != "container" {
		return ""
	}
	return strconv.Itoa(n.Restarts)
}

func Summary(n *Node) string {
	counts := map[string]int{}
	restarts, pods := 0, 0
	var walk func(c *Node)
	walk = func(c *Node) {
		counts[c.Kind]++
		if c.Kind == "pod" {
			pods++
			restarts += c.Restarts
		}
		for _, k := range c.Children {
			walk(k)
		}
	}
	for _, c := range n.Children {
		walk(c)
	}
	var parts []string
	if n.Ready != "" {
		parts = append(parts, n.Ready+" ready")
	}
	for _, k := range []struct{ kind, one, many string }{
		{"rs", "rs", "rs"}, {"job", "job", "jobs"}, {"pod", "pod", "pods"}, {"container", "container", "containers"},
		{"svc", "svc", "svc"}, {"ing", "ing", "ing"}, {"cm", "cm", "cm"}, {"secret", "secret", "secrets"}, {"pvc", "pvc", "pvc"},
	} {

		if k.kind == "container" && n.Kind != "pod" {
			continue
		}
		if c := counts[k.kind]; c > 0 {
			word := k.many
			if c == 1 {
				word = k.one
			}
			parts = append(parts, strconv.Itoa(c)+" "+word)
		}
	}
	if n.Kind == "pod" {
		restarts += n.Restarts
		pods++
	}
	if pods > 0 {
		word := "restarts"
		if restarts == 1 {
			word = "restart"
		}
		parts = append(parts, strconv.Itoa(restarts)+" "+word)
	}
	return strings.Join(parts, " · ")
}

func Trouble(n *Node) bool {
	if n.Kind == "pod" && podTrouble(n) {
		return true
	}
	for _, c := range n.Children {
		if Trouble(c) {
			return true
		}
	}
	return false
}

func podTrouble(n *Node) bool {
	if n.Restarts > 0 {
		return true
	}
	switch n.Status {
	case "Completed", "Succeeded":
		return false
	case "Running":
		a, b, ok := strings.Cut(n.Ready, "/")
		return ok && a != b
	}
	return true
}

func FilterXray(roots []*Node, q string) []*Node {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return roots
	}
	var keep func(n *Node) *Node
	keep = func(n *Node) *Node {
		if strings.Contains(strings.ToLower(n.Name), q) {
			return n
		}
		var kids []*Node
		for _, c := range n.Children {
			if k := keep(c); k != nil {
				kids = append(kids, k)
			}
		}
		if len(kids) == 0 {
			return nil
		}
		cp := *n
		cp.Children = kids
		return &cp
	}
	var out []*Node
	for _, r := range roots {
		if k := keep(r); k != nil {
			out = append(out, k)
		}
	}
	return out
}

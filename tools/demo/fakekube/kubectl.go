package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type flags struct {
	raw          []string
	pos          []string
	rest         []string
	kubeconfig   string
	context      string
	namespace    string
	namespaceSet bool
	all          bool
	output       string
	selector     string
	fieldSel     string
	container    string
	kustomize    string
	values       map[string]string
	bools        map[string]bool
}

var valued = map[string]bool{
	"--kubeconfig": true, "--context": true, "-n": true, "--namespace": true, "-o": true, "--output": true,
	"-l": true, "--selector": true, "--field-selector": true, "-c": true, "--container": true,
	"-k": true, "--kustomize": true, "--replicas": true, "--tail": true, "--since": true, "--since-time": true,
	"--sort-by": true, "--request-timeout": true, "--type": true, "-p": true, "--patch": true, "--from": true,
}

func parseFlags(args []string) flags {
	f := flags{raw: args, values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.rest = args[i+1:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			f.pos = append(f.pos, a)
			continue
		}
		key, val, hasVal := strings.Cut(a, "=")

		if !hasVal && len(a) > 2 && a[1] != '-' && valued[a[:2]] {
			key, val, hasVal = a[:2], a[2:], true
		}
		if !hasVal && valued[key] {
			if i+1 < len(args) {
				i++
				val = args[i]
			}
			hasVal = true
		}
		switch key {
		case "--kubeconfig":
			f.kubeconfig = val
		case "--context":
			f.context = val
		case "-n", "--namespace":
			f.namespace, f.namespaceSet = val, true
		case "-A", "--all-namespaces":
			f.all = true
		case "-o", "--output":
			f.output = val
		case "-l", "--selector":
			f.selector = val
		case "--field-selector":
			f.fieldSel = val
		case "-c", "--container":
			f.container = val
		case "-k", "--kustomize":
			f.kustomize = val
		default:
			if hasVal {
				f.values[key] = val
			} else {
				f.bools[key] = true
			}
		}
	}
	return f
}

func (f flags) target() (kc, ctx, ns string) {
	kc = kubeconfigPath(f.kubeconfig)
	ctx = f.context
	if ctx == "" {
		ctx = currentContext(kc)
	}
	ns = f.namespace
	if !f.namespaceSet {
		ns = defaultNamespace(kc, ctx)
	}
	if ns == "" {
		ns = "default"
	}
	return
}

func kubectlMain(args []string) int {
	f := parseFlags(args)
	if len(f.pos) == 0 {
		return unknown("kubectl", args)
	}
	verb := f.pos[0]
	switch verb {
	case "config":
		return configCmd(f)
	case "version":
		return versionCmd(f)
	case "kustomize":
		return kustomizeCmd(f)
	}
	_, ctx, _ := f.target()
	if ctx == "" {
		fmt.Fprintln(os.Stderr, "error: current-context is not set")
		return 1
	}
	if _, ok := readOnly().Contexts[ctx]; !ok {
		fmt.Fprintf(os.Stderr, "error: context %q does not exist\n", ctx)
		return 1
	}
	switch verb {
	case "api-resources":
		return apiResources()
	case "get":
		return getCmd(f)
	case "describe":
		return describeCmd(f)
	case "logs":
		return logsCmd(f)
	case "top":
		return topCmd(f)
	case "delete":
		return deleteCmd(f)
	case "scale":
		return scaleCmd(f)
	case "rollout":
		return rolloutCmd(f)
	case "label", "annotate":
		return labelCmd(f, verb)
	case "set":
		return setImageCmd(f)
	case "edit":
		return editCmd(f)
	case "exec":
		return execCmd(f)
	case "port-forward":
		return portForward(f)
	case "diff":
		return diffCmd(f)
	case "apply":
		return applyCmd(f)
	case "create":
		return createJobCmd(f)
	}
	return unknown("kubectl", args)
}

func versionCmd(f flags) int {
	client := `"clientVersion":{"major":"1","minor":"31","gitVersion":"v1.31.2","gitCommit":"5864a4677267e6adeae276ad85882a8714d69d9d","gitTreeState":"clean","goVersion":"go1.22.8","compiler":"gc","platform":"linux/amd64"},"kustomizeVersion":"v5.4.2"`
	_, ctx, _ := f.target()
	server := map[string]string{"kind-dev": "v1.31.2", "staging-eu-1": "v1.30.5-eks-ce1d5eb", "prod-eu-1": "v1.30.5-eks-ce1d5eb"}[ctx]
	if f.output != "json" {
		fmt.Println("Client Version: v1.31.2\nKustomize Version: v5.4.2")
		if server != "" && !f.bools["--client"] {
			fmt.Println("Server Version: " + server)
		}
		return 0
	}
	if f.bools["--client"] || server == "" {
		fmt.Println("{" + client + "}")
		return 0
	}
	fmt.Printf("{%s,\"serverVersion\":{\"major\":\"1\",\"minor\":\"31\",\"gitVersion\":%q,\"platform\":\"linux/amd64\"}}\n", client, server)
	return 0
}

func apiResources() int {
	for _, k := range kinds {
		fmt.Printf("%-34s %-12s %-30s %-6v %s\n", k.Plural, strings.Join(k.Short, ","), k.APIVersion, k.Namespaced, k.Kind)
	}
	return 0
}

func snapshot(ctx string) []obj {
	unlock := lockWorld()
	defer unlock()
	w := loadWorld()
	c := newCl(w, ctx)
	before, _ := json.Marshal(c.objs())
	c.reconcile()
	after, _ := json.Marshal(c.objs())
	if string(before) != string(after) {
		_ = w.save()
	}
	return c.view()
}

type resourceRef struct {
	kind kindInfo
	name string
}

func parseRefs(pos []string) ([]resourceRef, error) {
	if len(pos) == 0 {
		return nil, fmt.Errorf("You must specify the type of resource to get.")
	}
	var refs []resourceRef
	if strings.Contains(pos[0], "/") {
		for _, p := range pos {
			kn, nm, _ := strings.Cut(p, "/")
			k, ok := lookupKind(kn)
			if !ok {
				return nil, fmt.Errorf("the server doesn't have a resource type %q", kn)
			}
			refs = append(refs, resourceRef{k, nm})
		}
		return refs, nil
	}
	for _, kn := range strings.Split(pos[0], ",") {
		k, ok := lookupKind(kn)
		if !ok {
			return nil, fmt.Errorf("the server doesn't have a resource type %q", kn)
		}
		if len(pos) == 1 {
			refs = append(refs, resourceRef{k, ""})
		}
		for _, nm := range pos[1:] {
			refs = append(refs, resourceRef{k, nm})
		}
	}
	return refs, nil
}

func matchSelector(o obj, sel string) bool {
	if sel == "" {
		return true
	}
	lbl := labels(o, "metadata", "labels")
	for _, term := range strings.Split(sel, ",") {
		term = strings.TrimSpace(term)
		switch {
		case strings.Contains(term, "!="):
			k, v, _ := strings.Cut(term, "!=")
			if lbl[k] == v {
				return false
			}
		case strings.Contains(term, "="):
			k, v, _ := strings.Cut(strings.Replace(term, "==", "=", 1), "=")
			if lbl[k] != v {
				return false
			}
		case strings.HasPrefix(term, "!"):
			if _, ok := lbl[term[1:]]; ok {
				return false
			}
		default:
			if _, ok := lbl[term]; !ok {
				return false
			}
		}
	}
	return true
}

func matchFields(o obj, sel string) bool {
	if sel == "" {
		return true
	}
	for _, term := range strings.Split(sel, ",") {
		neg := strings.Contains(term, "!=")
		k, v, _ := strings.Cut(strings.Replace(term, "!=", "=", 1), "=")
		got := fmt.Sprint(get(o, strings.Split(k, ".")...))
		if s := str(o, strings.Split(k, ".")...); s != "" {
			got = s
		}
		if (got == v) == neg {
			return false
		}
	}
	return true
}

func selectObjects(f flags, refs []resourceRef) (out []obj, missing []resourceRef) {
	_, ctx, ns := f.target()
	view := snapshot(ctx)
	for _, r := range refs {
		found := false
		for _, o := range view {
			if str(o, "kind") != r.kind.Kind {
				continue
			}
			if r.kind.Namespaced && !f.all && nsOf(o) != ns {
				continue
			}
			if r.name != "" && objName(o) != r.name {
				continue
			}
			if !matchSelector(o, f.selector) || !matchFields(o, f.fieldSel) {
				continue
			}
			out = append(out, o)
			found = true
		}
		if r.name != "" && !found {
			missing = append(missing, r)
		}
	}
	return
}

func notFound(r resourceRef) {
	k := r.kind.Plural
	if g := r.kind.group(); g != "" {
		k += "." + g
	}
	fmt.Fprintf(os.Stderr, "Error from server (NotFound): %s %q not found\n", k, r.name)
}

func getCmd(f flags) int {
	if len(f.pos) > 1 && (f.pos[1] == "clusterversion" || strings.HasPrefix(f.pos[1], "clusterversion")) {
		fmt.Fprintln(os.Stderr, `error: the server doesn't have a resource type "clusterversion"`)
		return 1
	}
	refs, err := parseRefs(f.pos[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	objs, missing := selectObjects(f, refs)
	if len(missing) > 0 && !f.bools["--ignore-not-found"] {
		for _, r := range missing {
			notFound(r)
		}
		if len(objs) == 0 {
			return 1
		}
	}
	if len(refs) > 1 || refs[0].name == "" {

	} else {
		sortObjects(objs)
	}
	if sb := f.values["--sort-by"]; strings.Contains(sb, "lastTimestamp") {
		sort.SliceStable(objs, func(i, j int) bool { return str(objs[i], "lastTimestamp") < str(objs[j], "lastTimestamp") })
	}
	single := len(refs) == 1 && refs[0].name != "" && len(objs) == 1
	out := f.output
	switch {
	case out == "json":
		if single {
			printJSON(objs[0])
		} else if len(objs) == 0 && f.bools["--ignore-not-found"] && len(refs) > 0 && refs[0].name != "" {

		} else {
			printJSON(obj{"apiVersion": "v1", "kind": "List", "metadata": obj{"resourceVersion": ""}, "items": toAny(objs)})
		}
	case out == "yaml":
		if single {
			fmt.Print(yamlText(objs[0]))
		} else {
			fmt.Print(yamlText(obj{"apiVersion": "v1", "kind": "List", "items": toAny(objs)}))
		}
	case out == "name":
		for _, o := range objs {
			fmt.Println(kindOf(o).nameRef() + "/" + objName(o))
		}
	case strings.HasPrefix(out, "jsonpath="):
		fmt.Print(jsonPath(strings.TrimPrefix(out, "jsonpath="), objs, single))
	default:
		if len(objs) == 0 {
			_, _, ns := f.target()
			if f.all {
				fmt.Fprintln(os.Stderr, "No resources found")
			} else {
				fmt.Fprintf(os.Stderr, "No resources found in %s namespace.\n", ns)
			}
			return 0
		}
		printTable(objs, f.all, out == "wide", f.bools["--no-headers"])
	}
	return 0
}

func toAny(objs []obj) []any {
	out := make([]any, 0, len(objs))
	for _, o := range objs {
		out = append(out, o)
	}
	return out
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "    ")
	fmt.Println(string(b))
}

func jsonPath(tmpl string, objs []obj, single bool) string {
	var b strings.Builder
	switch {
	case strings.Contains(tmpl, "{range .items[*]}{.metadata.namespace}/{.metadata.name}"):
		for _, o := range objs {
			fmt.Fprintf(&b, "%s/%s\n", nsOf(o), objName(o))
		}
	case strings.Contains(tmpl, "{range .items[*]}{.kind}/{.metadata.name}"):
		for _, o := range objs {
			fmt.Fprintf(&b, "%s/%s\n", str(o, "kind"), objName(o))
		}
	case strings.Contains(tmpl, "{range .items[*]}{.metadata.name}"):
		for _, o := range objs {
			fmt.Fprintf(&b, "%s\n", objName(o))
		}
	case strings.Contains(tmpl, "ownerReferences[?(@.controller==true)]"):
		for _, o := range objs {
			for _, r := range list(o, "metadata", "ownerReferences") {
				if get(r, "controller") == true {
					fmt.Fprintf(&b, "%s/%s", str(r, "kind"), str(r, "name"))
				}
			}
		}
	default:

		path := strings.Trim(tmpl, "{}")
		path = strings.TrimPrefix(path, ".")
		for _, o := range objs {
			v := get(o, strings.Split(path, ".")...)
			if v != nil {
				if s, ok := v.(string); ok {
					b.WriteString(s)
				} else {
					js, _ := json.Marshal(v)
					b.Write(js)
				}
			}
			if !single {
				b.WriteString(" ")
			}
		}
	}
	return b.String()
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case t.IsZero():
		return "<unknown>"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		if m := int(d.Minutes()) % 60; m != 0 && d < 10*time.Hour {
			return fmt.Sprintf("%dh%dm", int(d.Hours()), m)
		}
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func podStatus(p obj) (ready string, status string, restarts int) {
	total, up := 0, 0
	status = str(p, "status", "phase")
	for _, s := range list(p, "status", "containerStatuses") {
		total++
		if get(s, "ready") == true {
			up++
		}
		restarts += num(s, "restartCount")
		if r := str(s, "state", "waiting", "reason"); r != "" {
			status = r
		}
		if r := str(s, "state", "terminated", "reason"); r != "" {
			status = r
		}
	}
	if terminating(p) {
		status = "Terminating"
	}
	return fmt.Sprintf("%d/%d", up, total), status, restarts
}

func printTable(objs []obj, all, wide, noHeaders bool) {
	rows := [][]string{}
	var head []string
	kind := str(objs[0], "kind")
	for _, o := range objs {
		var r []string
		switch kind {
		case "Pod":
			head = []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
			ready, st, rs := podStatus(o)
			r = []string{objName(o), ready, st, strconv.Itoa(rs), age(parseTS(str(o, "metadata", "creationTimestamp")))}
			if wide {
				head = append(head, "IP", "NODE")
				r = append(r, str(o, "status", "podIP"), str(o, "spec", "nodeName"))
			}
		case "Deployment":
			head = []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}
			r = []string{objName(o), fmt.Sprintf("%d/%d", num(o, "status", "readyReplicas"), num(o, "spec", "replicas")),
				strconv.Itoa(num(o, "status", "updatedReplicas")), strconv.Itoa(num(o, "status", "availableReplicas")), age(parseTS(str(o, "metadata", "creationTimestamp")))}
		case "Service":
			head = []string{"NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT(S)", "AGE"}
			var ports []string
			for _, p := range list(o, "spec", "ports") {
				ports = append(ports, fmt.Sprintf("%d/%s", num(p, "port"), str(p, "protocol")))
			}
			r = []string{objName(o), str(o, "spec", "type"), str(o, "spec", "clusterIP"), "<none>", strings.Join(ports, ","), age(parseTS(str(o, "metadata", "creationTimestamp")))}
		default:
			head = []string{"NAME", "AGE"}
			r = []string{objName(o), age(parseTS(str(o, "metadata", "creationTimestamp")))}
		}
		if all && kindOf(o).Namespaced {
			r = append([]string{nsOf(o)}, r...)
		}
		rows = append(rows, r)
	}
	if all && kindOf(objs[0]).Namespaced {
		head = append([]string{"NAMESPACE"}, head...)
	}
	if !noHeaders {
		rows = append([][]string{head}, rows...)
	}
	widths := make([]int, len(head))
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	for _, r := range rows {
		var b strings.Builder
		for i, c := range r {
			if i == len(r)-1 {
				b.WriteString(c)
			} else {
				fmt.Fprintf(&b, "%-*s   ", widths[i], c)
			}
		}
		fmt.Println(b.String())
	}
}

func topCmd(f flags) int {
	if len(f.pos) < 2 || !strings.HasPrefix(f.pos[1], "po") {
		return unknown("kubectl", f.raw)
	}
	pk, _ := lookupKind("pods")
	refs := []resourceRef{{kind: pk}}
	objs, _ := selectObjects(f, refs)
	sortObjects(objs)
	bucket := strconv.FormatInt(time.Now().Unix()/15, 10)
	if !f.bools["--no-headers"] {
		if f.all {
			fmt.Printf("%-14s ", "NAMESPACE")
		}
		fmt.Printf("%-52s %-12s %s\n", "NAME", "CPU(cores)", "MEMORY(bytes)")
	}
	for _, p := range objs {
		if str(p, "status", "phase") != "Running" {
			continue
		}
		cpu, mem := 0, 0
		for _, c := range list(p, "spec", "containers") {
			lim := milli(str(c, "resources", "limits", "cpu"))
			mlim := mebi(str(c, "resources", "limits", "memory"))
			pct := 12 + hashN(55, objName(p), str(c, "name"), bucket)
			mpct := 30 + hashN(40, objName(p), str(c, "name"))
			switch {
			case strings.HasPrefix(objName(p), "kafka-0"), strings.HasPrefix(objName(p), "api-"):
				pct = 74 + hashN(14, objName(p), bucket)
			case strings.HasPrefix(objName(p), "prometheus-0"):
				mpct = 91 + hashN(5, bucket)
			case strings.HasPrefix(objName(p), "cart-"):
				mpct = 72 + hashN(8, objName(p))
			}
			cpu += lim * pct / 100
			mem += mlim * mpct / 100
		}
		if cpu < 1 {
			cpu = 1
		}
		if f.all {
			fmt.Printf("%-14s ", nsOf(p))
		}
		fmt.Printf("%-52s %-12s %s\n", objName(p), fmt.Sprintf("%dm", cpu), fmt.Sprintf("%dMi", mem))
	}
	return 0
}

func milli(s string) int {
	if strings.HasSuffix(s, "m") {
		n, _ := strconv.Atoi(strings.TrimSuffix(s, "m"))
		return n
	}
	n, _ := strconv.ParseFloat(s, 64)
	return int(n * 1000)
}

func mebi(s string) int {
	switch {
	case strings.HasSuffix(s, "Gi"):
		n, _ := strconv.Atoi(strings.TrimSuffix(s, "Gi"))
		return n * 1024
	case strings.HasSuffix(s, "Mi"):
		n, _ := strconv.Atoi(strings.TrimSuffix(s, "Mi"))
		return n
	}
	return 0
}

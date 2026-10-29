package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type kustomization struct {
	Namespace string   `yaml:"namespace"`
	Resources []string `yaml:"resources"`
	Images    []struct {
		Name   string `yaml:"name"`
		NewTag string `yaml:"newTag"`
	} `yaml:"images"`
	Replicas []struct {
		Name  string `yaml:"name"`
		Count int    `yaml:"count"`
	} `yaml:"replicas"`
}

func build(dir string) ([]obj, error) {
	b, err := os.ReadFile(filepath.Join(dir, "kustomization.yaml"))
	if err != nil {
		return nil, fmt.Errorf("unable to find one of 'kustomization.yaml', 'kustomization.yml' or 'Kustomization' in directory '%s'", dir)
	}
	var k kustomization
	if err := yaml.Unmarshal(b, &k); err != nil {
		return nil, err
	}
	var out []obj
	for _, r := range k.Resources {
		p := filepath.Join(dir, r)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			objs, err := build(p)
			if err != nil {
				return nil, err
			}
			out = append(out, objs...)
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("accumulating resources: %s: no such file or directory", r)
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		for {
			var o map[string]any
			if err := dec.Decode(&o); err != nil {
				break
			}
			if o != nil {
				out = append(out, deepCopy(o))
			}
		}
	}
	for _, o := range out {
		if k.Namespace != "" && str(o, "kind") != "Namespace" {
			sub(o, "metadata")["namespace"] = k.Namespace
		}
		for _, im := range k.Images {
			for _, c := range list(o, "spec", "template", "spec", "containers") {
				img := str(c, "image")
				if strings.SplitN(img, ":", 2)[0] == im.Name && im.NewTag != "" {
					c.(obj)["image"] = im.Name + ":" + im.NewTag
				}
			}
		}
		for _, r := range k.Replicas {
			if kd := str(o, "kind"); objName(o) == r.Name && (kd == "Deployment" || kd == "StatefulSet" || kd == "ReplicaSet") {
				sub(o, "spec")["replicas"] = r.Count
			}
		}
	}
	return out, nil
}

func kustomizeCmd(f flags) int {
	if len(f.pos) < 2 {
		return unknown("kubectl", f.raw)
	}
	objs, err := build(f.pos[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	for i, o := range objs {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Print(yamlText(o))
	}
	return 0
}

func liveFor(c *cl, o obj) obj {
	k := kindOf(o)
	ns := nsOf(o)
	if ns == "" && k.Namespaced {
		ns = "default"
	}
	return c.find(k.Kind, ns, objName(o))
}

func comparable(live, rendered obj) obj {
	out := obj{"apiVersion": live["apiVersion"], "kind": live["kind"]}
	md := obj{"name": objName(live)}
	if nsOf(live) != "" {
		md["namespace"] = nsOf(live)
	}
	if l := get(rendered, "metadata", "labels"); l != nil {
		md["labels"] = pick2(get(live, "metadata", "labels"), l)
	}
	out["metadata"] = md
	for _, key := range []string{"spec", "data", "type"} {
		if _, ok := rendered[key]; ok {
			out[key] = pick2(live[key], rendered[key])
		}
	}
	return out
}

func pick2(live, want any) any {
	lm, ok1 := live.(map[string]any)
	wm, ok2 := want.(map[string]any)
	if !ok1 || !ok2 {
		return live
	}
	out := map[string]any{}
	for k := range wm {
		if v, ok := lm[k]; ok {
			out[k] = pick2(v, wm[k])
		}
	}
	return out
}

func diffCmd(f flags) int {
	if f.kustomize == "" {
		return unknown("kubectl", f.raw)
	}
	objs, err := build(f.kustomize)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	_, ctx, _ := f.target()
	w := readOnly()
	c := newCl(w, ctx)
	time.Sleep(600 * time.Millisecond)
	changed := false
	live := "/tmp/LIVE-" + suffix(9, f.kustomize, "live")
	merged := "/tmp/MERGED-" + suffix(9, f.kustomize, "merged")
	stamp := time.Now().Format("2006-01-02 15:04:05.000000000 -0700")
	for _, o := range objs {
		k := kindOf(o)
		file := strings.ReplaceAll(k.APIVersion, "/", ".") + "." + k.Kind + "."
		if nsOf(o) != "" {
			file += nsOf(o) + "."
		}
		file += objName(o)
		var before string
		if l := liveFor(c, o); l != nil {
			before = yamlText(comparable(l, o))
		}
		after := yamlText(mergeLive(c, o))
		if before == after {
			continue
		}
		changed = true
		fmt.Printf("diff -u -N %s/%s %s/%s\n--- %s/%s\t%s\n+++ %s/%s\t%s\n", live, file, merged, file, live, file, stamp, merged, file, stamp)
		for _, l := range hunks(lineDiff(splitLines(before), splitLines(after)), 3) {
			fmt.Println(l)
		}
	}
	if changed {
		return 1
	}
	return 0
}

func mergeLive(c *cl, o obj) obj {
	l := liveFor(c, o)
	if l == nil {
		x := deepCopy(o)
		if nsOf(x) == "" && kindOf(x).Namespaced {
			sub(x, "metadata")["namespace"] = "default"
		}
		return comparable(x, o)
	}
	return comparable(mergedFull(l, o), o)
}

func mergedFull(l, o obj) obj {
	m := deepCopy(l)
	lcs := list(l, "spec", "template", "spec", "containers")
	for _, key := range []string{"spec", "data"} {
		if v, ok := o[key]; ok {
			m[key] = mergeValues(asMap(m[key]), asMap(v))
		}
	}
	if lb := get(o, "metadata", "labels"); lb != nil {
		sub(m, "metadata")["labels"] = mergeValues(asMap(get(m, "metadata", "labels")), asMap(lb))
	}
	if cs := list(o, "spec", "template", "spec", "containers"); cs != nil {
		var merged []any
		for i, c := range cs {
			if i < len(lcs) {
				merged = append(merged, mergeValues(asMap(deepCopy(asMap(lcs[i]))), asMap(c)))
			} else {
				merged = append(merged, c)
			}
		}
		sub(m, "spec", "template", "spec")["containers"] = merged
	}
	return m
}

func asMap(v any) map[string]any {
	if mm, ok := v.(map[string]any); ok {
		return mm
	}
	return map[string]any{}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func hunks(d []string, ctx int) []string {
	var out []string
	changed := func(i int) bool { return d[i][0] != ' ' }
	i := 0
	for i < len(d) {

		for i < len(d) && !changed(i) {
			i++
		}
		if i == len(d) {
			break
		}
		start := i - ctx
		if start < 0 {
			start = 0
		}
		end := i

		for j := i; j < len(d); j++ {
			if changed(j) {
				end = j
			} else if j-end > 2*ctx {
				break
			}
		}
		stop := end + ctx + 1
		if stop > len(d) {
			stop = len(d)
		}

		oldLn, newLn := 1, 1
		for _, l := range d[:start] {
			if l[0] != '+' {
				oldLn++
			}
			if l[0] != '-' {
				newLn++
			}
		}
		oldN, newN := 0, 0
		for _, l := range d[start:stop] {
			if l[0] != '+' {
				oldN++
			}
			if l[0] != '-' {
				newN++
			}
		}
		if oldN == 0 {
			oldLn = 0
		}
		if newN == 0 {
			newLn = 0
		}
		out = append(out, fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldLn, oldN, newLn, newN))
		out = append(out, d[start:stop]...)
		i = stop
	}
	return out
}

func lineDiff(a, b []string) []string {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			out = append(out, " "+a[i])
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, "-"+a[i])
			i++
		default:
			out = append(out, "+"+b[j])
			j++
		}
	}
	return out
}

func applyCmd(f flags) int {
	if f.kustomize == "" {
		return unknown("kubectl", f.raw)
	}
	return kustomizeRun(f, "apply")
}

func kustomizeRun(f flags, verb string) int {
	objs, err := build(f.kustomize)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	_, ctx, _ := f.target()
	out := bufio.NewWriter(os.Stdout)
	deleted := 0
	for _, o := range objs {
		k := kindOf(o)
		ns := nsOf(o)
		if ns == "" && k.Namespaced {
			ns = "default"
		}
		result := ""
		mutate(func(w *world) int {
			c := newCl(w, ctx)
			l := liveFor(c, o)
			switch {
			case verb == "delete" && l == nil:
				result = fmt.Sprintf("Error from server (NotFound): %s %q not found", k.Plural, objName(o))
			case verb == "delete":
				c.terminate(l, 3*time.Second)
				for _, rs := range c.ownedBy("ReplicaSet", l) {
					c.terminate(rs, 3*time.Second)
					for _, p := range c.ownedBy("Pod", rs) {
						c.terminate(p, 3*time.Second)
					}
				}
				for _, p := range c.ownedBy("Pod", l) {
					c.terminate(p, 3*time.Second)
				}
				deleted++
				if k.Namespaced {
					result = fmt.Sprintf("%s %q deleted from %s namespace", k.nameRef(), objName(o), ns)
				} else {
					result = fmt.Sprintf("%s %q deleted", k.nameRef(), objName(o))
				}
			case l == nil:
				x := deepCopy(o)
				md := sub(x, "metadata")
				if k.Namespaced {
					md["namespace"] = ns
				}
				md["creationTimestamp"] = ts(c.now)
				md["uid"] = uid(ctx, "applied", ns, objName(o))
				if k.Kind == "Namespace" {
					x["status"] = obj{"phase": "Active"}
				}
				c.add(x)
				result = k.nameRef() + "/" + objName(o) + " created"
			default:
				before := yamlText(comparable(l, o))
				after := yamlText(mergeLive(c, o))
				if before == after {
					result = k.nameRef() + "/" + objName(o) + " unchanged"
					break
				}
				full := mergedFull(l, o)
				for _, key := range []string{"spec", "data"} {
					if v, ok := full[key]; ok {
						l[key] = v
					}
				}
				sub(l, "metadata")["labels"] = get(full, "metadata", "labels")
				sub(l, "metadata")["generation"] = num(l, "metadata", "generation") + 1
				result = k.nameRef() + "/" + objName(o) + " configured"
			}
			c.reconcile()
			return 0
		})
		fmt.Fprintln(out, result)
		out.Flush()
		time.Sleep(time.Duration(250+hashN(150, objName(o), verb)) * time.Millisecond)
	}
	if verb == "delete" && deleted > 0 {
		time.Sleep(3 * time.Second)
	}
	return 0
}

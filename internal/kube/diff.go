package kube

import (
	"fmt"
	"sort"
	"strings"
)

type DiffKind byte

const (
	DiffSame DiffKind = ' '
	DiffDel  DiffKind = '-'
	DiffAdd  DiffKind = '+'
)

type DiffLine struct {
	Kind DiffKind
	Text string
}

const maxEdits = 1000

func LineDiff(a, b []string) []DiffLine {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var out []DiffLine
	for _, l := range a[:pre] {
		out = append(out, DiffLine{DiffSame, l})
	}
	out = append(out, myers(a[pre:len(a)-suf], b[pre:len(b)-suf])...)
	for _, l := range a[len(a)-suf:] {
		out = append(out, DiffLine{DiffSame, l})
	}
	return out
}

func myers(a, b []string) []DiffLine {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return replaceAll(a, b)
	}
	max := n + m
	if max > maxEdits {
		max = maxEdits
	}
	off := max
	v := make([]int, 2*max+2)
	var trace [][]int
	found := -1
	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[off+k] = x
			if x >= n && y >= m {
				found = d
				break
			}
		}
		if found >= 0 {
			break
		}
	}
	if found < 0 {
		return replaceAll(a, b)
	}

	var rev []DiffLine
	x, y := n, m
	for d := found; d > 0; d-- {
		vd := trace[d]
		k := x - y
		var pk int
		if k == -d || (k != d && vd[off+k-1] < vd[off+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := vd[off+pk]
		py := px - pk
		for x > px && y > py {
			x, y = x-1, y-1
			rev = append(rev, DiffLine{DiffSame, a[x]})
		}
		if x == px {
			y--
			rev = append(rev, DiffLine{DiffAdd, b[y]})
		} else {
			x--
			rev = append(rev, DiffLine{DiffDel, a[x]})
		}
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		rev = append(rev, DiffLine{DiffSame, a[x]})
	}
	out := make([]DiffLine, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

func replaceAll(a, b []string) []DiffLine {
	out := make([]DiffLine, 0, len(a)+len(b))
	for _, l := range a {
		out = append(out, DiffLine{DiffDel, l})
	}
	for _, l := range b {
		out = append(out, DiffLine{DiffAdd, l})
	}
	return out
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func Unified(a, b string, ctx int) string {
	lines := LineDiff(splitLines(a), splitLines(b))
	changed := false
	for _, l := range lines {
		if l.Kind != DiffSame {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}
	var out strings.Builder

	oldAt, newAt := make([]int, len(lines)), make([]int, len(lines))
	o, n := 1, 1
	for i, l := range lines {
		oldAt[i], newAt[i] = o, n
		if l.Kind != DiffAdd {
			o++
		}
		if l.Kind != DiffDel {
			n++
		}
	}
	for i := 0; i < len(lines); {
		if lines[i].Kind == DiffSame {
			i++
			continue
		}

		start := max(0, i-ctx)
		end := i
		for j := i; j < len(lines); j++ {
			if lines[j].Kind != DiffSame {
				end = j
			} else if j-end > 2*ctx {
				break
			}
		}
		end = min(len(lines)-1, end+ctx)
		oc, nc := 0, 0
		for _, l := range lines[start : end+1] {
			if l.Kind != DiffAdd {
				oc++
			}
			if l.Kind != DiffDel {
				nc++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldAt[start], oc, newAt[start], nc)
		for _, l := range lines[start : end+1] {
			out.WriteString(string(l.Kind) + l.Text + "\n")
		}
		i = end + 1
	}
	return out.String()
}

func manifestDocs(manifest string) (map[string]string, []string) {
	docs := map[string]string{}
	var order []string
	for i, doc := range strings.Split("\n"+manifest, "\n---") {
		doc = strings.Trim(doc, "\n")
		if strings.TrimSpace(doc) == "" {
			continue
		}
		key := docKey(doc)
		if key == "" {
			key = fmt.Sprintf("document %d", i)
		}
		if _, dup := docs[key]; dup {
			key = fmt.Sprintf("%s (%d)", key, i)
		}
		docs[key] = doc + "\n"
		order = append(order, key)
	}
	return docs, order
}

func docKey(doc string) string {
	kind, name, inMeta := "", "", false
	for _, l := range strings.Split(doc, "\n") {
		switch {
		case strings.HasPrefix(l, "kind:"):
			kind = strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "kind:")), `"'`)
			inMeta = false
		case strings.HasPrefix(l, "metadata:"):
			inMeta = true
		case inMeta && name == "" && strings.HasPrefix(strings.TrimLeft(l, " "), "name:") && len(l)-len(strings.TrimLeft(l, " ")) == 2:
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "name:")), `"'`)
		case l != "" && l[0] != ' ' && l[0] != '#':
			inMeta = false
		}
	}
	if kind == "" || name == "" {
		return ""
	}
	return kind + "/" + name
}

func RevisionDiff(a, b HelmRelease) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s/%s: revision %d (%s) → revision %d (%s)\n", b.Namespace, b.Name, a.Revision, orDash(a.Chart), b.Revision, orDash(b.Chart))
	oldDocs, oldOrder := manifestDocs(a.Manifest)
	newDocs, newOrder := manifestDocs(b.Manifest)
	keys := map[string]bool{}
	for _, k := range append(oldOrder, newOrder...) {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var added, removed, changed, same int
	var body strings.Builder
	for _, k := range sorted {
		o, inOld := oldDocs[k]
		n, inNew := newDocs[k]
		switch {
		case !inOld:
			added++
			fmt.Fprintf(&body, "=== + %s (added)\n", k)
			for _, l := range splitLines(n) {
				body.WriteString("+" + l + "\n")
			}
		case !inNew:
			removed++
			fmt.Fprintf(&body, "=== - %s (removed)\n", k)
			for _, l := range splitLines(o) {
				body.WriteString("-" + l + "\n")
			}
		default:
			u := Unified(o, n, 3)
			if u == "" {
				same++
				continue
			}
			changed++
			fmt.Fprintf(&body, "=== ~ %s\n", k)
			body.WriteString(u)
		}
	}
	fmt.Fprintf(&out, "# manifest: %d changed, %d added, %d removed, %d unchanged\n", changed, added, removed, same)
	if u := Unified(ValuesYAML(a.Config), ValuesYAML(b.Config), 3); u != "" {
		out.WriteString("=== ~ values (user-supplied)\n" + u)
	} else {
		out.WriteString("# values (user-supplied): unchanged\n")
	}
	out.WriteString(body.String())
	return out.String()
}

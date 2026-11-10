package kube

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k2/internal/shell"
)

type PrinterColumn struct {
	Name     string
	Type     string
	JSONPath string
}

type printerCache struct {
	mu    sync.Mutex
	byKey map[string][]PrinterColumn
}

var printers = printerCache{byKey: map[string][]PrinterColumn{}}

func (c Client) PrinterColumns(kubeconfig, context, kind string) []PrinterColumn {
	key := kubeconfig + "\x00" + context + "\x00" + strings.ToLower(kind)
	printers.mu.Lock()
	if cols, ok := printers.byKey[key]; ok {
		printers.mu.Unlock()
		return cols
	}
	printers.mu.Unlock()
	cols := c.loadPrinterColumns(kubeconfig, context, kind)
	printers.mu.Lock()
	printers.byKey[key] = cols
	printers.mu.Unlock()
	return cols
}

func (c Client) loadPrinterColumns(kubeconfig, context, kind string) []PrinterColumn {
	res, _, grp := SplitKind(kind)
	if res == "" {
		res = BaseKind(kind)
	}
	if grp == "" {
		return nil
	}
	crd := res + "." + grp
	out, err := shell.Combined(c.Run, "kubectl", "get", "crd", crd, "-o", "json", "--kubeconfig", kubeconfig, "--context", context)
	if err != nil {
		return nil
	}
	return ParsePrinterColumns(out)
}

func ParsePrinterColumns(out []byte) []PrinterColumn {
	var crd struct {
		Spec struct {
			Versions []struct {
				Name                     string `json:"name"`
				Storage                  bool   `json:"storage"`
				AdditionalPrinterColumns []struct {
					Name     string `json:"name"`
					Type     string `json:"type"`
					JSONPath string `json:"jsonPath"`
				} `json:"additionalPrinterColumns"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if json.Unmarshal(out, &crd) != nil {
		i := strings.Index(string(out), "\n{")
		if i < 0 || json.Unmarshal(out[i+1:], &crd) != nil {
			return nil
		}
	}
	var cols []PrinterColumn
	pick := -1
	for i, v := range crd.Spec.Versions {
		if v.Storage {
			pick = i
			break
		}
	}
	if pick < 0 && len(crd.Spec.Versions) > 0 {
		pick = len(crd.Spec.Versions) - 1
	}
	if pick < 0 {
		return nil
	}
	for _, c := range crd.Spec.Versions[pick].AdditionalPrinterColumns {
		name := strings.TrimSpace(c.Name)
		path := strings.TrimSpace(c.JSONPath)
		if name == "" || path == "" {
			continue
		}
		if strings.EqualFold(name, "name") || strings.EqualFold(name, "age") {
			continue
		}
		cols = append(cols, PrinterColumn{Name: strings.ToUpper(name), Type: c.Type, JSONPath: path})
	}
	return cols
}

func EvalJSONPath(o Object, path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "{")
	path = strings.TrimSuffix(path, "}")
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return ""
	}
	path = strings.TrimPrefix(path, ".")
	cur := any(o.Raw)
	if cur == nil {
		raw, _ := json.Marshal(o)
		_ = json.Unmarshal(raw, &cur)
	}
	for path != "" {
		if i := strings.Index(path, "[?"); i >= 0 {
			head, rest := path[:i], path[i:]
			if head != "" {
				cur = dig(cur, strings.Split(strings.Trim(head, "."), ".")...)
			}
			end := strings.Index(rest, "]")
			if end < 0 {
				return ""
			}
			filter, after := rest[2:end], rest[end+1:]
			after = strings.TrimPrefix(after, ".")
			path = after
			key, want, ok := parseFilter(filter)
			if !ok {
				return ""
			}
			list, _ := cur.([]any)
			var hit any
			for _, item := range list {
				if field(item, key) == want {
					hit = item
					break
				}
			}
			cur = hit
			continue
		}
		if i := strings.IndexAny(path, ".["); i >= 0 && path[i] == '[' {
			head, rest := path[:i], path[i:]
			if head != "" {
				cur = dig(cur, strings.Split(head, ".")...)
			}
			end := strings.Index(rest, "]")
			if end < 0 {
				return ""
			}
			idx, err := strconv.Atoi(rest[1:end])
			if err != nil {
				return ""
			}
			list, _ := cur.([]any)
			if idx < 0 || idx >= len(list) {
				return ""
			}
			cur = list[idx]
			path = strings.TrimPrefix(rest[end+1:], ".")
			continue
		}
		parts := strings.Split(path, ".")
		cur = dig(cur, parts...)
		path = ""
	}
	return stringify(cur)
}

func parseFilter(f string) (key, want string, ok bool) {
	f = strings.TrimSpace(f)
	f = strings.TrimPrefix(f, "(")
	f = strings.TrimSuffix(f, ")")
	f = strings.TrimSpace(f)
	f = strings.TrimPrefix(f, "@.")
	key, rest, cut := strings.Cut(f, "==")
	if !cut {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	want = strings.TrimSpace(rest)
	want = strings.Trim(want, `"'`)
	return key, want, key != ""
}

func dig(v any, parts ...string) any {
	cur := v
	for _, p := range parts {
		if p == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case []any:
		var parts []string
		for _, x := range t {
			if s := stringify(x); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(t)
	}
}

func (c Client) GenericTable(kubeconfig, context, kind, namespace string, objs []Object) Table {
	showNS := namespace == ""
	if headers, rows, ok := c.fetchPrintedTable(kubeconfig, context, kind, namespace); ok {
		return TableFromPrinted(objs, headers, rows)
	}
	cols := c.PrinterColumns(kubeconfig, context, kind)
	return ResourceTableWithColumns(kind, objs, showNS, cols)
}

func (c Client) fetchPrintedTable(kubeconfig, context, kind, namespace string) ([]string, [][]string, bool) {
	run := func(all bool) ([]byte, error) {
		args := []string{"get", kind, "--kubeconfig", kubeconfig, "--context", context}
		if all {
			args = append(args, "-A")
		} else if namespace != "" {
			args = append(args, "-n", namespace)
		}
		return shell.Combined(c.Run, "kubectl", args...)
	}
	all := namespace == ""
	out, err := run(all)
	if err != nil && all && strings.Contains(strings.ToLower(string(out)), "namespaced") {
		all = false
		out, err = run(false)
	}
	if err != nil {
		return nil, nil, false
	}
	return ParseKubectlTable(out)
}

func ParseKubectlTable(out []byte) ([]string, [][]string, bool) {
	text := string(out)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		low := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(low, "warning:") || strings.HasPrefix(low, "error:") {
			continue
		}
		if strings.HasPrefix(low, "no resources found") {
			return nil, nil, false
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil, nil, false
	}
	starts := columnStarts(lines[0])
	if len(starts) < 2 {
		return nil, nil, false
	}
	headers := splitColumns(lines[0], starts)
	if len(headers) < 2 {
		return nil, nil, false
	}
	nameOK := false
	for _, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), "name") {
			nameOK = true
			break
		}
	}
	if !nameOK {
		return nil, nil, false
	}
	var rows [][]string
	for _, line := range lines[1:] {
		cells := splitColumns(line, starts)
		if len(cells) == 0 || strings.TrimSpace(cells[0]) == "" {
			continue
		}
		for len(cells) < len(headers) {
			cells = append(cells, "")
		}
		if len(cells) > len(headers) {
			cells = cells[:len(headers)]
		}
		rows = append(rows, cells)
	}
	return headers, rows, true
}

func columnStarts(header string) []int {
	var starts []int
	in := false
	for i := 0; i < len(header); i++ {
		if header[i] != ' ' && header[i] != '\t' {
			if !in {
				starts = append(starts, i)
				in = true
			}
		} else {
			in = false
		}
	}
	return starts
}

func splitColumns(line string, starts []int) []string {
	cells := make([]string, len(starts))
	for i, start := range starts {
		end := len(line)
		if i+1 < len(starts) {
			end = starts[i+1]
			if end > len(line) {
				end = len(line)
			}
		}
		if start >= len(line) {
			cells[i] = ""
			continue
		}
		cells[i] = strings.TrimSpace(line[start:end])
	}
	return cells
}

func TableFromPrinted(objs []Object, headers []string, rows [][]string) Table {
	heads := make([]string, len(headers))
	for i, h := range headers {
		heads[i] = strings.ToUpper(strings.TrimSpace(h))
	}
	nameIdx, nsIdx, colorCol := -1, -1, -1
	for i, h := range heads {
		switch h {
		case "NAME":
			nameIdx = i
		case "NAMESPACE":
			nsIdx = i
		}
		if colorCol < 0 && (h == "READY" || h == "STATUS" || h == "PHASE" || h == "STATE") {
			colorCol = i
		}
	}
	byNSName := map[string]Object{}
	byName := map[string]Object{}
	for _, o := range objs {
		byNSName[o.Metadata.Namespace+"/"+o.Metadata.Name] = o
		byName[o.Metadata.Name] = o
	}
	t := Table{Headers: heads, ColorCol: colorCol}
	for _, row := range rows {
		cells := make([]string, len(heads))
		for i := range heads {
			if i < len(row) {
				cells[i] = row[i]
			}
			if cells[i] == "" {
				cells[i] = "-"
			}
		}
		name, ns := "", ""
		if nameIdx >= 0 {
			name = strings.TrimSuffix(cells[nameIdx], " terminating")
			if name == "-" {
				name = ""
			}
		}
		if nsIdx >= 0 && cells[nsIdx] != "-" {
			ns = cells[nsIdx]
		}
		o, ok := byNSName[ns+"/"+name]
		if !ok {
			o, ok = byName[name]
		}
		if ok {
			if ns == "" {
				ns = o.Metadata.Namespace
			}
			if o.Metadata.DeletionTimestamp != nil && nameIdx >= 0 {
				cells[nameIdx] = name + " terminating"
			}
			t.Born = append(t.Born, o.Metadata.CreationTimestamp)
		} else {
			t.Born = append(t.Born, time.Time{})
		}
		t.Rows = append(t.Rows, cells)
		t.Refs = append(t.Refs, [2]string{ns, name})
	}
	return t
}

func ResourceTableWithColumns(kind string, objs []Object, showNS bool, cols []PrinterColumn) Table {
	if len(cols) == 0 {
		return ResourceTable(kind, objs, showNS)
	}
	secret := kind == "secrets" || kind == "secret"
	if secret {
		return ResourceTable(kind, objs, showNS)
	}
	headers := []string{"NAME"}
	if showNS {
		headers = []string{"NAMESPACE", "NAME"}
	}
	colorCol := -1
	for _, c := range cols {
		headers = append(headers, c.Name)
		if colorCol < 0 && (strings.EqualFold(c.Name, "ready") || strings.EqualFold(c.Name, "status") || strings.EqualFold(c.Name, "phase") || strings.EqualFold(c.Name, "state")) {
			colorCol = len(headers) - 1
		}
	}
	headers = append(headers, "AGE")
	t := Table{Headers: headers, ColorCol: colorCol}
	for _, o := range objs {
		name := o.Metadata.Name
		label := name
		if o.Metadata.DeletionTimestamp != nil {
			label = name + " terminating"
		}
		ns := o.Metadata.Namespace
		var row []string
		if showNS {
			row = append(row, ns)
		}
		row = append(row, label)
		for _, c := range cols {
			val := EvalJSONPath(o, c.JSONPath)
			if strings.EqualFold(c.Type, "date") {
				if ts, err := time.Parse(time.RFC3339, val); err == nil {
					val = Age(ts)
				}
			}
			row = append(row, orDash(val))
		}
		row = append(row, Age(o.Metadata.CreationTimestamp))
		t.Rows = append(t.Rows, row)
		t.Refs = append(t.Refs, [2]string{ns, name})
		t.Born = append(t.Born, o.Metadata.CreationTimestamp)
	}
	return t
}

func TableForColumns(kind string, objs []Object, showNS bool, cols []PrinterColumn) Table {
	showNS = showNS && anyNamespaced(objs)
	if s, ok := specs[BaseKind(kind)]; ok {
		return build(objs, showNS, s)
	}
	if s, ok := specFor(kind, objs); ok {
		return build(objs, showNS, s)
	}
	switch BaseKind(kind) {
	case "pods":
		return PodTable(objs, showNS)
	case "events":
		return EventTable(objs, showNS)
	}
	return ResourceTableWithColumns(kind, objs, showNS, cols)
}

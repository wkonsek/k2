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

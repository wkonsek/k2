package kube

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (o Object) Get(path ...string) any {
	var cur any = o.Raw
	for _, p := range path {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	return cur
}

func (o Object) Str(path ...string) string {
	switch v := o.Get(path...).(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

func (o Object) Int(path ...string) (int, bool) {
	switch v := o.Get(path...).(type) {
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	}
	return 0, false
}

func (o Object) IntOr(def int, path ...string) int {
	if n, ok := o.Int(path...); ok {
		return n
	}
	return def
}

func (o Object) List(path ...string) []any {
	l, _ := o.Get(path...).([]any)
	return l
}

func (o Object) Time(path ...string) time.Time {
	t, err := time.Parse(time.RFC3339, o.Str(path...))
	if err != nil {
		return time.Time{}
	}
	return t
}

func field(v any, path ...string) string {
	return Object{Raw: map[string]any{"x": v}}.Str(append([]string{"x"}, path...)...)
}

func (o Object) condition(typ string) string {
	for _, c := range o.List("status", "conditions") {
		if field(c, "type") == typ {
			return field(c, "status")
		}
	}
	return ""
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func (o Object) Containers() (init, main []string) {
	for _, c := range o.List("spec", "initContainers") {
		init = append(init, field(c, "name"))
	}
	for _, c := range o.List("spec", "containers") {
		main = append(main, field(c, "name"))
	}
	return init, main
}

type column struct {
	head string
	cell func(o Object) string
}

type spec struct {
	cols   []column
	status string
	extra  func(o Object) map[string]string
}

func build(objs []Object, showNS bool, s spec) Table {
	heads := []string{"NAME"}
	if showNS {
		heads = []string{"NAMESPACE", "NAME"}
	}
	for _, c := range s.cols {
		heads = append(heads, c.head)
	}
	heads = append(heads, "AGE")
	t := Table{Headers: heads, ColorCol: -1}
	for i, h := range heads {
		if s.status != "" && h == s.status {
			t.ColorCol = i
		}
	}
	for _, o := range objs {
		name := o.Metadata.Name
		label := name
		if o.Metadata.DeletionTimestamp != nil {
			label = name + " terminating"
		}
		row := []string{label}
		if showNS {
			row = []string{o.Metadata.Namespace, label}
		}
		for _, c := range s.cols {
			row = append(row, orDash(c.cell(o)))
		}
		row = append(row, Age(o.Metadata.CreationTimestamp))
		t.Rows = append(t.Rows, row)
		t.Refs = append(t.Refs, [2]string{o.Metadata.Namespace, name})
		t.Born = append(t.Born, o.Metadata.CreationTimestamp)
		var ex map[string]string
		if s.extra != nil {
			ex = s.extra(o)
		}
		t.Extra = append(t.Extra, ex)
	}
	return t
}

func TableFor(kind string, objs []Object, showNS bool) Table {
	if s, ok := specs[BaseKind(kind)]; ok {
		return build(objs, showNS, s)
	}
	switch BaseKind(kind) {
	case "pods":
		return PodTable(objs, showNS)
	case "events":
		return EventTable(objs, showNS)
	}
	return ResourceTable(kind, objs, showNS)
}

func ratio(a, b int) string { return fmt.Sprintf("%d/%d", a, b) }

func intAt(path ...string) func(Object) string {
	return func(o Object) string { return strconv.Itoa(o.IntOr(0, path...)) }
}

func replicasExtra(o Object) map[string]string {
	return map[string]string{"replicas": strconv.Itoa(o.IntOr(1, "spec", "replicas"))}
}

var specs = map[string]spec{
	"deployments": {cols: []column{
		{"READY", func(o Object) string {
			return ratio(o.IntOr(0, "status", "readyReplicas"), o.IntOr(1, "spec", "replicas"))
		}},
		{"UP-TO-DATE", intAt("status", "updatedReplicas")},
		{"AVAILABLE", intAt("status", "availableReplicas")},
	}, extra: replicasExtra},
	"statefulsets": {cols: []column{
		{"READY", func(o Object) string {
			return ratio(o.IntOr(0, "status", "readyReplicas"), o.IntOr(1, "spec", "replicas"))
		}},
	}, extra: replicasExtra},
	"replicasets": {cols: []column{
		{"DESIRED", func(o Object) string { return strconv.Itoa(o.IntOr(1, "spec", "replicas")) }},
		{"CURRENT", intAt("status", "replicas")},
		{"READY", func(o Object) string {
			return ratio(o.IntOr(0, "status", "readyReplicas"), o.IntOr(1, "spec", "replicas"))
		}},
	}, extra: replicasExtra},
	"daemonsets": {cols: []column{
		{"DESIRED", intAt("status", "desiredNumberScheduled")},
		{"CURRENT", intAt("status", "currentNumberScheduled")},
		{"READY", func(o Object) string {
			return ratio(o.IntOr(0, "status", "numberReady"), o.IntOr(0, "status", "desiredNumberScheduled"))
		}},
		{"UP-TO-DATE", intAt("status", "updatedNumberScheduled")},
		{"AVAILABLE", intAt("status", "numberAvailable")},
	}},
	"jobs": {cols: []column{
		{"STATUS", JobStatus},
		{"COMPLETIONS", func(o Object) string {
			return ratio(o.IntOr(0, "status", "succeeded"), o.IntOr(1, "spec", "completions"))
		}},
		{"DURATION", func(o Object) string { return jobDuration(o, time.Now()) }},
	}, status: "STATUS"},
	"cronjobs": {cols: []column{
		{"SCHEDULE", func(o Object) string { return o.Str("spec", "schedule") }},
		{"SUSPEND", func(o Object) string {
			if o.Str("spec", "suspend") == "true" {
				return "Suspended"
			}
			return "False"
		}},
		{"ACTIVE", func(o Object) string { return strconv.Itoa(len(o.List("status", "active"))) }},
		{"LAST SCHEDULE", func(o Object) string {
			if t := o.Time("status", "lastScheduleTime"); !t.IsZero() {
				return Age(t) + " ago"
			}
			return "never"
		}},
	}, status: "SUSPEND"},
}

func JobStatus(o Object) string {
	switch {
	case o.condition("Complete") == "True":
		return "Complete"
	case o.condition("Failed") == "True":
		return "Failed"
	case o.condition("Suspended") == "True" || o.Str("spec", "suspend") == "true":
		return "Suspended"
	case o.IntOr(0, "status", "active") > 0:
		return "Running"
	}
	return "Pending"
}

func jobDuration(o Object, now time.Time) string {
	start := o.Time("status", "startTime")
	if start.IsZero() {
		return ""
	}
	end := o.Time("status", "completionTime")
	if end.IsZero() {
		end = now
	}
	return AgeAt(start, end)
}

func eventTime(o Object) time.Time {
	for _, p := range [][]string{{"lastTimestamp"}, {"eventTime"}, {"series", "lastObservedTime"}, {"firstTimestamp"}} {
		if t := o.Time(p...); !t.IsZero() {
			return t
		}
	}
	return o.Metadata.CreationTimestamp
}

func EventTable(objs []Object, showNS bool) Table {
	sorted := append([]Object(nil), objs...)
	sort.SliceStable(sorted, func(i, j int) bool { return eventTime(sorted[i]).Before(eventTime(sorted[j])) })
	heads := []string{"LAST SEEN", "TYPE", "REASON", "OBJECT", "MESSAGE"}
	if showNS {
		heads = append([]string{"NAMESPACE"}, heads...)
	}
	t := Table{Headers: heads, ColorCol: 1}
	if showNS {
		t.ColorCol = 2
	}
	for _, o := range sorted {
		kind := strings.ToLower(o.Str("involvedObject", "kind"))
		name := o.Str("involvedObject", "name")
		if kind == "" {
			kind = strings.ToLower(o.Str("regarding", "kind"))
			name = o.Str("regarding", "name")
		}
		msg := strings.Join(strings.Fields(o.Str("message")), " ")
		if msg == "" {
			msg = o.Str("note")
		}
		when := eventTime(o)
		row := []string{Age(when), orDash(o.Str("type")), orDash(o.Str("reason")), orDash(kind + "/" + name), orDash(msg)}
		if showNS {
			row = append([]string{o.Metadata.Namespace}, row...)
		}
		t.Rows = append(t.Rows, row)
		t.Refs = append(t.Refs, [2]string{o.Metadata.Namespace, o.Metadata.Name})
		t.Born = append(t.Born, when)
		t.Extra = append(t.Extra, nil)
	}
	return t
}

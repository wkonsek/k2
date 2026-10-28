package kube

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"
)

func (c Client) APIResources(kubeconfig, context string) ([]string, error) {
	out, err := shell.Output(c.Run, "kubectl", "api-resources", "--no-headers", "--verbs=get", "--kubeconfig", kubeconfig, "--context", context)
	if err != nil {
		return nil, err
	}
	return ParseAPIResources(string(out)), nil
}

func ParseAPIResources(out string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		add(f[0])
		nsIdx := -1
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] == "true" || f[i] == "false" {
				nsIdx = i
				break
			}
		}
		if nsIdx > 2 {
			for _, short := range strings.Split(strings.Join(f[1:nsIdx-1], ","), ",") {
				add(short)
			}
		}
	}
	sort.Strings(names)
	return names
}

type Object struct {
	Metadata struct {
		Name              string     `json:"name"`
		Namespace         string     `json:"namespace"`
		CreationTimestamp time.Time  `json:"creationTimestamp"`
		DeletionTimestamp *time.Time `json:"deletionTimestamp"`
	} `json:"metadata"`
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		PodIP             string `json:"podIP"`
		ContainerStatuses []struct {
			Ready        bool `json:"ready"`
			RestartCount int  `json:"restartCount"`
			State        struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting"`
				Terminated *struct {
					Reason string `json:"reason"`
				} `json:"terminated"`
			} `json:"state"`

			LastState struct {
				Terminated *struct {
					FinishedAt time.Time `json:"finishedAt"`
				} `json:"terminated"`
			} `json:"lastState"`
		} `json:"containerStatuses"`
	} `json:"status"`

	Raw map[string]any `json:"-"`
}

func (c Client) Objects(kubeconfig, context, kind, namespace string) ([]Object, error) {
	return c.ObjectsWhere(kubeconfig, context, kind, namespace, "")
}

func (c Client) ObjectsWhere(kubeconfig, context, kind, namespace, selector string) ([]Object, error) {
	run := func(all bool) ([]byte, error) {
		args := []string{"get", kind, "-o", "json", "--kubeconfig", kubeconfig, "--context", context}
		if selector != "" {
			args = append(args, "-l", selector)
		}
		if BaseKind(kind) == "events" {

			args = append(args, "--sort-by=.lastTimestamp")
		}
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
		return nil, fmt.Errorf("%s", shell.FailText(out, err))
	}
	return ParseObjects(out)
}

func ParseObjects(out []byte) ([]Object, error) {
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(out, &list) != nil {
		return nil, fmt.Errorf("bad kubectl json")
	}
	var wrap struct{ Items []Object }
	for _, raw := range list.Items {
		var o Object
		if json.Unmarshal(raw, &o) != nil || json.Unmarshal(raw, &o.Raw) != nil {
			return nil, fmt.Errorf("bad kubectl json")
		}
		wrap.Items = append(wrap.Items, o)
	}
	sort.Slice(wrap.Items, func(i, j int) bool {
		a, b := wrap.Items[i], wrap.Items[j]
		if a.Metadata.Namespace != b.Metadata.Namespace {
			return a.Metadata.Namespace < b.Metadata.Namespace
		}
		return a.Metadata.Name < b.Metadata.Name
	})
	return wrap.Items, nil
}

func Age(t time.Time) string {
	return AgeAt(t, time.Now())
}

func AgeAt(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dd", int(d.Hours())/24)
}

type Table struct {
	Headers  []string
	Rows     [][]string
	Refs     [][2]string
	Born     []time.Time
	ColorCol int

	Extra []map[string]string
}

func IsSecretKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "secret", "secrets":
		return true
	default:
		return false
	}
}

func ResourceTable(kind string, objs []Object, showNS bool) Table {
	secret := kind == "secrets" || kind == "secret"
	headers := []string{"NAME", "AGE"}
	if showNS {
		headers = []string{"NAMESPACE", "NAME", "AGE"}
	}
	if secret {
		if showNS {
			headers = []string{"NAMESPACE", "NAME", "TYPE", "DATA", "AGE"}
		} else {
			headers = []string{"NAME", "TYPE", "DATA", "AGE"}
		}
	}
	t := Table{Headers: headers, ColorCol: -1}
	for _, o := range objs {
		age := Age(o.Metadata.CreationTimestamp)
		name := o.Metadata.Name
		label := name
		if o.Metadata.DeletionTimestamp != nil {
			label = name + " terminating"
		}
		ns := o.Metadata.Namespace
		var row []string
		if secret {
			data := strconv.Itoa(len(o.Data))
			typ := o.Type
			if typ == "" {
				typ = "-"
			}
			if showNS {
				row = []string{ns, label, typ, data, age}
			} else {
				row = []string{label, typ, data, age}
			}
		} else if showNS {
			row = []string{ns, label, age}
		} else {
			row = []string{label, age}
		}
		t.Rows = append(t.Rows, row)
		t.Refs = append(t.Refs, [2]string{ns, name})
		t.Born = append(t.Born, o.Metadata.CreationTimestamp)
		t.Extra = append(t.Extra, nil)
	}
	if secret {
		t.ColorCol = 2
		if !showNS {
			t.ColorCol = 1
		}
	}
	return t
}

func PodState(o Object) (ready, total, restarts int, status string) {
	ready, total, restarts, status, _ = podState(o)
	return
}

func podState(o Object) (ready, total, restarts int, status string, lastRestart time.Time) {
	status = o.Status.Phase
	if status == "" {
		status = "-"
	}
	for _, c := range o.Status.ContainerStatuses {
		total++
		restarts += c.RestartCount
		if lt := c.LastState.Terminated; lt != nil && lt.FinishedAt.After(lastRestart) {
			lastRestart = lt.FinishedAt
		}
		if c.Ready {
			ready++
		}
		if c.State.Waiting != nil && c.State.Waiting.Reason != "" {
			status = c.State.Waiting.Reason
		} else if c.State.Terminated != nil && c.State.Terminated.Reason != "" && status != "Running" {
			status = c.State.Terminated.Reason
		}
	}
	if o.Metadata.DeletionTimestamp != nil {
		status = "Terminating"
	}
	return
}

func PodTable(objs []Object, showNS bool) Table {
	headers := []string{"NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}
	colorCol := 2
	if showNS {
		headers = []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}
		colorCol = 3
	}
	t := Table{Headers: headers, ColorCol: colorCol}
	for _, o := range objs {
		ready, total, restarts, status, lastRestart := podState(o)
		readyTxt := "-"
		if total > 0 {
			readyTxt = fmt.Sprintf("%d/%d", ready, total)
		}
		ip := o.Status.PodIP
		if ip == "" {
			ip = "-"
		}
		node := o.Spec.NodeName
		if node == "" {
			node = "-"
		}
		age := Age(o.Metadata.CreationTimestamp)
		rest := strconv.Itoa(restarts)
		if restarts > 0 && !lastRestart.IsZero() {

			rest += " (" + Age(lastRestart) + " ago)"
		}
		ns, name := o.Metadata.Namespace, o.Metadata.Name
		if showNS {
			t.Rows = append(t.Rows, []string{ns, name, readyTxt, status, rest, ip, node, age})
		} else {
			t.Rows = append(t.Rows, []string{name, readyTxt, status, rest, ip, node, age})
		}
		t.Refs = append(t.Refs, [2]string{ns, name})
		t.Born = append(t.Born, o.Metadata.CreationTimestamp)

		init, main := o.Containers()

		cpuLim, memLim := podLimits(o)
		cpuReq, memReq := podRequests(o)
		t.Extra = append(t.Extra, map[string]string{"init": strings.Join(init, ","), "containers": strings.Join(main, ","),
			"cpuLimit": cpuLim, "memLimit": memLim, "cpuRequest": cpuReq, "memRequest": memReq})
	}
	return t
}

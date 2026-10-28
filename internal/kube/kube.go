package kube

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k2/internal/shell"

	"gopkg.in/yaml.v3"
)

type Client struct {
	Run shell.Runner
}

func New(r shell.Runner) Client { return Client{Run: r} }

func SplitRef(id, fallback string) (ns, name string) {
	if ns, name, ok := strings.Cut(id, "/"); ok && ns != "" && name != "" {
		return ns, name
	}
	return fallback, id
}

func (c Client) Contexts(kubeconfig string) ([]string, error) {
	out, err := shell.Output(c.Run, "kubectl", "config", "get-contexts", "-o", "name", "--kubeconfig", kubeconfig)
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(string(out)), nil
}

func (c Client) Pods(kubeconfig, context, namespace string) ([]string, error) {
	args := []string{"get", "pods"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	} else {
		args = append(args, "-A")
	}
	args = append(args,
		"-o", "jsonpath={range .items[*]}{.metadata.namespace}/{.metadata.name}{\"\\n\"}{end}",
		"--kubeconfig", kubeconfig, "--context", context)
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return splitNonEmpty(string(out)), nil
}

func (c Client) NamespaceOf(kubeconfig, context string) string {
	if kubeconfig == "" || context == "" {
		return ""
	}
	// --minify follows the file's current context, not --context, so an
	// OpenShift switch kept the previous project. read the file instead.
	if ns, ok := contextNamespace(kubeconfig, context); ok {
		return ns
	}
	out, err := shell.Output(c.Run, "kubectl", "--kubeconfig", kubeconfig, "--context", context,
		"config", "view", "--minify", "-o", "jsonpath={.contexts[0].context.namespace}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func contextNamespace(kubeconfig, context string) (ns string, found bool) {
	data, err := os.ReadFile(kubeconfig)
	if err != nil {
		return "", false
	}
	var kc struct {
		Contexts []struct {
			Name    string `yaml:"name"`
			Context struct {
				Namespace string `yaml:"namespace"`
			} `yaml:"context"`
		} `yaml:"contexts"`
	}
	if yaml.Unmarshal(data, &kc) != nil {
		return "", false
	}
	for _, c := range kc.Contexts {
		if c.Name != context {
			continue
		}
		ns = strings.TrimSpace(c.Context.Namespace)
		if ns == "" {
			ns = openShiftContextNS(context)
		}
		return ns, true
	}
	return "", false
}

func openShiftContextNS(name string) string {
	// oc names contexts "namespace/cluster/user" and often leaves the
	// namespace field empty.
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return parts[0]
}

func (c Client) CurrentContext(kubeconfig string) string {
	out, err := shell.Output(c.Run, "kubectl", "--kubeconfig", kubeconfig, "config", "current-context")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c Client) Projects(kubeconfig, context string) ([]string, error) {
	jp := "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}"
	try := [][]string{
		{"kubectl", "--kubeconfig", kubeconfig, "--context", context, "get", "projects", "-o", jp},
		{"kubectl", "--kubeconfig", kubeconfig, "--context", context, "get", "ns", "-o", jp},
	}
	var last error
	for _, args := range try {
		if _, err := c.Run.LookPath(args[0]); err != nil {
			continue
		}
		out, err := shell.Combined(c.Run, args[0], args[1:]...)
		if err != nil {
			last = fmt.Errorf("%s", strings.TrimSpace(string(out)))
			continue
		}
		names := splitNonEmpty(string(out))
		if len(names) > 0 {
			return names, nil
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("no projects")
}

func (c Client) UseProject(kubeconfig, context, project string) (string, string, error) {
	if out, err := shell.Combined(c.Run, "kubectl", "--kubeconfig", kubeconfig, "config", "use-context", context); err != nil {
		return "", "", cmdError(out, err)
	}
	nsFlag := project
	if project == "*" {
		nsFlag = ""
	}
	if out, err := shell.Combined(c.Run, "kubectl", "--kubeconfig", kubeconfig, "config", "set-context", context, "--namespace", nsFlag); err != nil {
		return "", "", cmdError(out, err)
	}
	if project == "*" {
		return context, "", nil
	}
	ns := c.NamespaceOf(kubeconfig, context)
	if ns == "" {
		ns = project
	}
	return context, ns, nil
}

func (c Client) PersistSelection(kubeconfig, context, namespace string) {
	_, _ = shell.Output(c.Run, "kubectl", "config", "use-context", context, "--kubeconfig", kubeconfig)
	_, _ = shell.Output(c.Run, "kubectl", "config", "set-context", context, "--namespace", namespace, "--kubeconfig", kubeconfig)
}

func (c Client) GetJSON(kubeconfig, context string, args ...string) (map[string]any, error) {
	full := append([]string{"--kubeconfig", kubeconfig, "--context", context}, args...)
	full = append(full, "-o", "json")
	out, err := shell.Output(c.Run, "kubectl", full...)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func (c Client) YAML(kubeconfig, context, kind, name, namespace string) (string, error) {
	args := []string{"get", kind, name, "-o", "yaml", "--kubeconfig", kubeconfig, "--context", context}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	out, err := shell.Combined(c.Run, "kubectl", args...)
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("%s", shell.FailText(out, err))
	}
	return text, nil
}

func (c Client) DeletePod(kubeconfig, context, namespace, name string) error {
	out, err := shell.Combined(c.Run, "kubectl", "delete", "pod", name, "-n", namespace, "--kubeconfig", kubeconfig, "--context", context)
	if err != nil {
		return fmt.Errorf("%s", shell.FailText(out, err))
	}
	return nil
}

func EditArgs(kind, name, kubeconfig, context, namespace string) []string {
	args := []string{"edit", kind, name, "--kubeconfig", kubeconfig, "--context", context}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return args
}

func DescribeArgs(kind, name, kubeconfig, context, namespace string) []string {
	args := []string{"describe", kind, name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return append(args, "--kubeconfig", kubeconfig, "--context", context)
}

func (c Client) Describe(kubeconfig, context, kind, name, namespace string) (string, error) {
	out, err := shell.Combined(c.Run, "kubectl", DescribeArgs(kind, name, kubeconfig, context, namespace)...)
	text := strings.TrimRight(string(out), "\n ")
	if err != nil {
		return "", fmt.Errorf("%s", shell.FailText(out, err))
	}
	return text, nil
}

type PortSuggestion struct {
	Local, Remote string
	Source        string
}

func (p PortSuggestion) Spec() string { return p.Local + ":" + p.Remote }

func (c Client) PortSuggestions(kubeconfig, context, podRef string) []PortSuggestion {
	parts := strings.SplitN(podRef, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	ns, name := parts[0], parts[1]

	pod, err := c.GetJSON(kubeconfig, context, "get", "pod", name, "-n", ns)
	if err != nil {
		return nil
	}
	labels := map[string]string{}
	if meta, ok := pod["metadata"].(map[string]any); ok {
		if raw, ok := meta["labels"].(map[string]any); ok {
			for k, v := range raw {
				labels[k] = fmt.Sprint(v)
			}
		}
	}
	type namedPort struct{ name, port string }
	var containerPorts []namedPort
	if spec, ok := pod["spec"].(map[string]any); ok {
		if containers, ok := spec["containers"].([]any); ok {
			for _, ct := range containers {
				cm, _ := ct.(map[string]any)
				ports, _ := cm["ports"].([]any)
				for _, p := range ports {
					pm, _ := p.(map[string]any)
					cp := num(pm["containerPort"])
					if cp == "" {
						continue
					}
					pn := ""
					if pm["name"] != nil {
						pn = fmt.Sprint(pm["name"])
					}
					containerPorts = append(containerPorts, namedPort{name: pn, port: cp})
				}
			}
		}
	}
	resolve := func(target string) string {
		if target == "" {
			return ""
		}
		if _, err := strconv.Atoi(target); err == nil {
			return target
		}
		for _, cp := range containerPorts {
			if cp.name == target {
				return cp.port
			}
		}
		return ""
	}

	seen := map[string]bool{}
	var out []PortSuggestion
	add := func(local, remote, source string) {
		if local == "" || remote == "" {
			return
		}
		key := local + ":" + remote
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, PortSuggestion{Local: local, Remote: remote, Source: source})
	}

	svcs, err := c.GetJSON(kubeconfig, context, "get", "svc", "-n", ns)
	if err == nil {
		items, _ := svcs["items"].([]any)
		for _, it := range items {
			sm, _ := it.(map[string]any)
			meta, _ := sm["metadata"].(map[string]any)
			spec, _ := sm["spec"].(map[string]any)
			if meta == nil || spec == nil {
				continue
			}
			sel, _ := spec["selector"].(map[string]any)
			if len(sel) == 0 {
				continue
			}
			match := true
			for k, v := range sel {
				if labels[k] != fmt.Sprint(v) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			svcName := fmt.Sprint(meta["name"])
			ports, _ := spec["ports"].([]any)
			for _, p := range ports {
				pm, _ := p.(map[string]any)
				port := num(pm["port"])
				target := resolve(num(pm["targetPort"]))
				if target == "" {
					target = port
				}
				add(port, target, "svc/"+svcName)
			}
		}
	}
	for _, cp := range containerPorts {
		add(cp.port, cp.port, "pod")
	}
	return out
}

func ForwardKind(kind string) string {
	k := strings.ToLower(kind)
	if i := strings.Index(k, "."); i > 0 {
		k = k[:i]
	}
	switch k {
	case "pod", "pods", "po":
		return "pod"
	case "service", "services", "svc":
		return "svc"
	case "deployment", "deployments", "deploy":
		return "deploy"
	case "statefulset", "statefulsets", "sts":
		return "sts"
	}
	return ""
}

func (c Client) TargetPortSuggestions(kubeconfig, context, kind, ns, name string) []PortSuggestion {
	if kind == "pod" {
		return c.PortSuggestions(kubeconfig, context, ns+"/"+name)
	}
	resource := map[string]string{"svc": "svc", "deploy": "deployment", "sts": "statefulset"}[kind]
	if resource == "" {
		return nil
	}
	obj, err := c.GetJSON(kubeconfig, context, "get", resource, name, "-n", ns)
	if err != nil {
		return nil
	}
	var out []PortSuggestion
	seen := map[string]bool{}
	add := func(port, source string) {
		if port != "" && !seen[port] {
			seen[port] = true
			out = append(out, PortSuggestion{Local: port, Remote: port, Source: source})
		}
	}
	spec, _ := obj["spec"].(map[string]any)
	if kind == "svc" {
		ports, _ := spec["ports"].([]any)
		for _, p := range ports {
			pm, _ := p.(map[string]any)
			add(num(pm["port"]), "svc/"+name)
		}
		return out
	}
	tmpl, _ := spec["template"].(map[string]any)
	podSpec, _ := tmpl["spec"].(map[string]any)
	containers, _ := podSpec["containers"].([]any)
	for _, ct := range containers {
		cm, _ := ct.(map[string]any)
		ports, _ := cm["ports"].([]any)
		for _, p := range ports {
			pm, _ := p.(map[string]any)
			add(num(pm["containerPort"]), kind+"/"+name)
		}
	}
	return out
}

func num(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.Itoa(int(t))
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func splitNonEmpty(s string) []string {
	var res []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			res = append(res, t)
		}
	}
	return res
}

func cmdError(out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("%s", msg)
}

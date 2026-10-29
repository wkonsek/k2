package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func kubeconfigPath(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return strings.Split(env, string(os.PathListSeparator))[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}

func readKubeconfig(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

func writeKubeconfig(path string, cfg map[string]any) error {
	return os.WriteFile(path, []byte(yamlText(cfg)), 0o600)
}

func contextNames(cfg map[string]any) []string {
	var out []string
	for _, c := range list(cfg, "contexts") {
		out = append(out, str(c, "name"))
	}
	return out
}

func contextEntry(cfg map[string]any, ctx string) map[string]any {
	for _, c := range list(cfg, "contexts") {
		if str(c, "name") == ctx {
			e, _ := get(c, "context").(map[string]any)
			return e
		}
	}
	return nil
}

func currentContext(path string) string {
	cfg, err := readKubeconfig(path)
	if err != nil {
		return ""
	}
	return str(cfg, "current-context")
}

func defaultNamespace(path, ctx string) string {
	cfg, err := readKubeconfig(path)
	if err != nil {
		return ""
	}
	return str(contextEntry(cfg, ctx), "namespace")
}

func configCmd(f flags) int {
	path := kubeconfigPath(f.kubeconfig)
	cfg, err := readKubeconfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open %s: no such file or directory\n", "kubeconfig")
		return 1
	}
	pos := f.pos[1:]
	if len(pos) == 0 {
		return unknown("kubectl", f.raw)
	}
	switch pos[0] {
	case "get-contexts":
		if f.output == "name" {
			for _, n := range contextNames(cfg) {
				fmt.Println(n)
			}
			return 0
		}
		fmt.Printf("%-9s %-15s %-15s %-15s %s\n", "CURRENT", "NAME", "CLUSTER", "AUTHINFO", "NAMESPACE")
		for _, n := range contextNames(cfg) {
			cur := ""
			if n == str(cfg, "current-context") {
				cur = "*"
			}
			e := contextEntry(cfg, n)
			fmt.Printf("%-9s %-15s %-15s %-15s %s\n", cur, n, str(e, "cluster"), str(e, "user"), str(e, "namespace"))
		}
		return 0
	case "current-context":
		cur := str(cfg, "current-context")
		if cur == "" {
			fmt.Fprintln(os.Stderr, "error: current-context is not set")
			return 1
		}
		fmt.Println(cur)
		return 0
	case "use-context":
		if len(pos) < 2 || contextEntry(cfg, pos[1]) == nil {
			fmt.Fprintf(os.Stderr, "error: no context exists with the name: %q\n", strings.Join(pos[1:], " "))
			return 1
		}
		cfg["current-context"] = pos[1]
		if err := writeKubeconfig(path, cfg); err != nil {
			return 1
		}
		fmt.Printf("Switched to context %q.\n", pos[1])
		return 0
	case "set-context":
		ctx := str(cfg, "current-context")
		if len(pos) > 1 {
			ctx = pos[1]
		}
		if len(pos) > 1 && pos[1] == "--current" {
			ctx = str(cfg, "current-context")
		}
		e := contextEntry(cfg, ctx)
		if e == nil {
			fmt.Fprintf(os.Stderr, "error: no context exists with the name: %q\n", ctx)
			return 1
		}
		if f.namespaceSet {
			if f.namespace == "" {
				delete(e, "namespace")
			} else {
				e["namespace"] = f.namespace
			}
		}
		if err := writeKubeconfig(path, cfg); err != nil {
			return 1
		}
		fmt.Printf("Context %q modified.\n", ctx)
		return 0
	case "view":
		ctx := f.context
		if ctx == "" {
			ctx = str(cfg, "current-context")
		}
		if strings.HasPrefix(f.output, "jsonpath=") && strings.Contains(f.output, ".context.namespace") {
			fmt.Print(str(contextEntry(cfg, ctx), "namespace"))
			return 0
		}
		if strings.HasPrefix(f.output, "jsonpath=") && strings.Contains(f.output, "current-context") {
			fmt.Print(str(cfg, "current-context"))
			return 0
		}

		fmt.Printf("apiVersion: v1\nkind: Config\ncurrent-context: %s\ncontexts: %v\n", str(cfg, "current-context"), contextNames(cfg))
		return 0
	}
	return unknown("kubectl", f.raw)
}

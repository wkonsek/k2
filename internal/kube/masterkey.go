package kube

import (
	"fmt"
	"io"
	"os"
	"strings"

	"k2/internal/shell"

	"gopkg.in/yaml.v3"
)

func ApplyMasterKey(run shell.Runner, path, namespace, kubeconfig, context string, createNS bool, onLine func(string)) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if onLine == nil {
		onLine = func(string) {}
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return fmt.Errorf("master key file not found: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("master key file: %w", err)
	}
	if createNS && namespace != "" && namespace != "default" && namespace != "kube-system" {
		args := []string{"create", "namespace", namespace}
		args = appendKubeTarget(args, kubeconfig, context)
		onLine("$ kubectl " + strings.Join(args, " "))
		out, err := shell.Combined(run, "kubectl", args...)
		onLine(string(out))
		if err != nil && !namespaceExists(string(out), err) {
			return err
		}
	}
	args := MasterKeyApplyArgs(path, namespace, kubeconfig, context, string(raw))
	onLine("$ kubectl " + strings.Join(args, " "))
	out, err := shell.Combined(run, "kubectl", args...)
	onLine(string(out))
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func MasterKeyApplyArgs(path, namespace, kubeconfig, context, manifest string) []string {
	args := []string{"apply", "-f", path}
	if namespace != "" && !YAMLDeclaresNamespace(manifest) {
		args = append(args, "-n", namespace)
	}
	return appendKubeTarget(args, kubeconfig, context)
}

func YAMLDeclaresNamespace(text string) bool {
	dec := yaml.NewDecoder(strings.NewReader(text))
	found := false
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		if len(doc) == 0 {
			continue
		}
		found = true
		meta, _ := doc["metadata"].(map[string]any)
		if meta == nil {
			return false
		}
		ns, _ := meta["namespace"].(string)
		if strings.TrimSpace(ns) == "" {
			return false
		}
	}
	return found
}

func appendKubeTarget(args []string, kubeconfig, context string) []string {
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	if context != "" {
		args = append(args, "--context", context)
	}
	return args
}

func namespaceExists(out string, err error) bool {
	text := out
	if err != nil {
		text += err.Error()
	}
	low := strings.ToLower(text)
	return strings.Contains(text, "AlreadyExists") || strings.Contains(low, "already exists")
}

package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func KindContext(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "kind-" + name
}

func KindKubeconfig(name, home string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	base := "k2"
	if name != "k2" {
		base = "k2-" + name
	}
	return filepath.Join(home, ".kube", base)
}

func ManagedKindKubeconfig(path, name, home string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	want := KindKubeconfig(name, home)
	if path == "" || want == "" {
		return false
	}
	if filepath.Base(path) == "config" {
		return false
	}
	return path == filepath.Clean(want)
}

func KindConfigYAML(controlPlanes, workers int, image string) (string, error) {
	if controlPlanes < 1 {
		return "", fmt.Errorf("need at least one control-plane node")
	}
	if workers < 0 {
		return "", fmt.Errorf("worker count cannot be negative")
	}
	image = strings.TrimSpace(image)
	var b strings.Builder
	b.WriteString("kind: Cluster\n")
	b.WriteString("apiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n")
	writeKindNode(&b, "control-plane", image, controlPlanes)
	writeKindNode(&b, "worker", image, workers)
	return b.String(), nil
}

func writeKindNode(b *strings.Builder, role, image string, n int) {
	for i := 0; i < n; i++ {
		b.WriteString("- role: ")
		b.WriteString(role)
		b.WriteByte('\n')
		if image != "" {
			b.WriteString("  image: ")
			b.WriteString(image)
			b.WriteByte('\n')
		}
	}
}

func KindCreateArgs(name, kubeconfig, configPath, image string) []string {
	args := []string{"create", "cluster", "--name", name, "--kubeconfig", kubeconfig}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if image != "" {
		args = append(args, "--image", image)
	}
	return append(args, "--wait", "5m")
}

func KindDeleteArgs(name string) []string {
	return []string{"delete", "cluster", "--name", name}
}

func KindGetArgs() []string {
	return []string{"get", "clusters"}
}

func ParseKindClusters(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "error:") || strings.HasPrefix(low, "warning:") || strings.HasPrefix(low, "no kind") {
			continue
		}
		names = append(names, line)
	}
	return names
}

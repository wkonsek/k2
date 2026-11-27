package kube

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const kindKubeletConfigPatch = `kind: KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
serializeImagePulls: false
maxParallelImagePulls: 4
`

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
	writeKindKubeadmPatches(&b)
	return b.String(), nil
}

func writeKindKubeadmPatches(b *strings.Builder) {
	b.WriteString("kubeadmConfigPatches:\n- |\n")
	for _, line := range strings.Split(strings.TrimSuffix(kindKubeletConfigPatch, "\n"), "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func defaultKindKubeadmPatches(n *yaml.Node) bool {
	if n == nil || n.Kind != yaml.SequenceNode || len(n.Content) != 1 {
		return false
	}
	item := n.Content[0]
	if item == nil || item.Kind != yaml.ScalarNode {
		return false
	}
	return strings.TrimSpace(item.Value) == strings.TrimSpace(kindKubeletConfigPatch)
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

type KindConfigInfo struct {
	Planes   int
	Workers  int
	Image    string
	HasImage bool
}

func ParseKindConfig(text string) (KindConfigInfo, error) {
	var doc struct {
		Nodes []struct {
			Role  string `yaml:"role"`
			Image string `yaml:"image"`
		} `yaml:"nodes"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return KindConfigInfo{}, err
	}
	var info KindConfigInfo
	var first string
	same := true
	for _, n := range doc.Nodes {
		switch strings.TrimSpace(n.Role) {
		case "control-plane":
			info.Planes++
		case "worker":
			info.Workers++
		}
		img := strings.TrimSpace(n.Image)
		if img == "" {
			continue
		}
		info.HasImage = true
		if first == "" {
			first = img
		} else if img != first {
			same = false
		}
	}
	if same && first != "" {
		info.Image = first
	} else {
		for _, n := range doc.Nodes {
			if img := strings.TrimSpace(n.Image); img != "" {
				info.Image = img
				break
			}
		}
	}
	return info, nil
}

func KindCreateImage(configText, image string) string {
	info, err := ParseKindConfig(configText)
	if err == nil && info.HasImage {
		return ""
	}
	return strings.TrimSpace(image)
}

func SyncKindConfigNodes(text string, controlPlanes, workers int, image string) (string, error) {
	if controlPlanes < 1 {
		return "", fmt.Errorf("need at least one control-plane node")
	}
	if workers < 0 {
		return "", fmt.Errorf("worker count cannot be negative")
	}
	image = strings.TrimSpace(image)
	if strings.TrimSpace(text) == "" {
		return KindConfigYAML(controlPlanes, workers, image)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return KindConfigYAML(controlPlanes, workers, image)
	}
	root := doc.Content[0]
	nodes := yamlMapValue(root, "nodes")
	if nodes == nil || nodes.Kind != yaml.SequenceNode || kindConfigSimple(root, nodes) {
		return KindConfigYAML(controlPlanes, workers, image)
	}
	adjustKindNodes(nodes, controlPlanes, workers, image)
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		enc.Close()
		return KindConfigYAML(controlPlanes, workers, image)
	}
	if err := enc.Close(); err != nil {
		return KindConfigYAML(controlPlanes, workers, image)
	}
	return b.String(), nil
}

func kindConfigSimple(root, nodes *yaml.Node) bool {
	if root == nil || nodes == nil || root.Kind != yaml.MappingNode || nodes.Kind != yaml.SequenceNode {
		return true
	}
	hasPatches := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := strings.TrimSpace(root.Content[i].Value)
		switch key {
		case "kind", "apiVersion", "nodes":
		case "kubeadmConfigPatches":
			if !defaultKindKubeadmPatches(root.Content[i+1]) {
				return false
			}
			hasPatches = true
		default:
			return false
		}
	}
	if !hasPatches {
		return false
	}
	for _, n := range nodes.Content {
		if n.Kind != yaml.MappingNode {
			return false
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := strings.TrimSpace(n.Content[i].Value)
			if key != "role" && key != "image" {
				return false
			}
		}
	}
	return true
}

func adjustKindNodes(seq *yaml.Node, planes, workers int, image string) {
	var cps, wks, other []*yaml.Node
	for _, n := range seq.Content {
		switch kindNodeRole(n) {
		case "control-plane":
			cps = append(cps, n)
		case "worker":
			wks = append(wks, n)
		default:
			other = append(other, n)
		}
	}
	cps = resizeKindRole(cps, "control-plane", planes, image)
	wks = resizeKindRole(wks, "worker", workers, image)
	seq.Content = append(append(cps, wks...), other...)
	for _, n := range seq.Content {
		setKindNodeImage(n, image)
	}
}

func resizeKindRole(nodes []*yaml.Node, role string, n int, image string) []*yaml.Node {
	if n < 0 {
		n = 0
	}
	for len(nodes) < n {
		nodes = append(nodes, newKindNode(role, image))
	}
	if len(nodes) > n {
		nodes = nodes[:n]
	}
	return nodes
}

func kindNodeRole(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.MappingNode {
		return ""
	}
	if v := yamlMapValue(n, "role"); v != nil {
		return strings.TrimSpace(v.Value)
	}
	return ""
}

func newKindNode(role, image string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	yamlSetMap(n, "role", role)
	if image != "" {
		yamlSetMap(n, "image", image)
	}
	return n
}

func setKindNodeImage(n *yaml.Node, image string) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	if image == "" {
		yamlDeleteMap(n, "image")
		return
	}
	yamlSetMap(n, "image", image)
}

func yamlMapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func yamlSetMap(n *yaml.Node, key, value string) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1].Kind = yaml.ScalarNode
			n.Content[i+1].Tag = "!!str"
			n.Content[i+1].Value = value
			return
		}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func yamlDeleteMap(n *yaml.Node, key string) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	out := n.Content[:0]
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			continue
		}
		out = append(out, n.Content[i], n.Content[i+1])
	}
	if len(n.Content)%2 == 1 {
		out = append(out, n.Content[len(n.Content)-1])
	}
	n.Content = out
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

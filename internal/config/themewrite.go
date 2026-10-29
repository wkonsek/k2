package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k2/internal/theme"

	"gopkg.in/yaml.v3"
)

func exactContext(s string) bool {
	return s != "" && !strings.ContainsAny(s, "*?")
}

func sameKC(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return filepath.Clean(ExpandHome(a)) == filepath.Clean(ExpandHome(b))
}

func knownTheme(name string) bool {
	if name == "custom" {
		return true
	}
	_, ok := theme.Lookup(name)
	return ok
}

func UpsertContextTheme(in []ContextTheme, context, kubeconfig, themeName string) ([]ContextTheme, error) {
	context = strings.TrimSpace(context)
	kubeconfig = strings.TrimSpace(kubeconfig)
	themeName = strings.ToLower(strings.TrimSpace(themeName))
	if context == "" && (kubeconfig == "" || kubeconfig == "no kubeconfig") {
		return in, fmt.Errorf("context theme needs a context")
	}
	if !knownTheme(themeName) {
		return in, fmt.Errorf("unknown theme %q", themeName)
	}
	out := append([]ContextTheme(nil), in...)
	idx := -1
	for i, e := range out {
		if !exactContext(e.Context) || e.Context != context {
			continue
		}
		if e.Kubeconfig != "" && !sameKC(e.Kubeconfig, kubeconfig) {
			continue
		}
		if idx < 0 || (e.Kubeconfig != "" && out[idx].Kubeconfig == "") {
			idx = i
		}
	}
	var e ContextTheme
	if idx >= 0 {
		e = out[idx]
		e.Theme = themeName
		out = append(out[:idx], out[idx+1:]...)
	} else {
		e = ContextTheme{Context: context, Theme: themeName}
		if kubeconfig != "" && kubeconfig != "no kubeconfig" {
			e.Kubeconfig = HomePath(kubeconfig)
		}
	}
	return append([]ContextTheme{e}, out...), nil
}

func WriteContextTheme(path string, entries []ContextTheme, context, kubeconfig, themeName string) (next []ContextTheme, err error) {
	next, err = UpsertContextTheme(entries, context, kubeconfig, themeName)
	if err != nil {
		return entries, err
	}
	if path == "" {
		path = FilePath()
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return next, readErr
	}
	out, err := renderContextThemes(data, next)
	if err != nil {
		return next, err
	}
	if err := writeFile(path, out); err != nil {
		return next, err
	}
	return next, nil
}

func renderContextThemes(data []byte, entries []ContextTheme) ([]byte, error) {
	doc, empty, err := loadDoc(data)
	if err != nil {
		return nil, err
	}
	if empty {
		return appendThemeBlock(data, entries), nil
	}
	root := doc.Content[0]
	if err := ensureThemes(root, entries); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadDoc(data []byte) (*yaml.Node, bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, true, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if err == io.EOF {
			return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, true, nil
		}
		return nil, false, fmt.Errorf("config: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode || len(doc.Content[0].Content) == 0 {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, true, nil
	}
	return &doc, false, nil
}

func mapIndex(n *yaml.Node, key string) int {
	if n == nil || n.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func ensureThemes(root *yaml.Node, entries []ContextTheme) error {
	ui := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if i := mapIndex(root, "ui"); i >= 0 && root.Content[i+1].Kind == yaml.MappingNode {
		ui = root.Content[i+1]
	} else if i >= 0 {
		root.Content[i+1] = ui
	} else {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "ui"},
			ui,
		)
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if i := mapIndex(ui, "context_themes"); i >= 0 {
		ui.Content[i+1] = seq
	} else {
		ui.Content = append(ui.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "context_themes"},
			seq,
		)
	}
	for _, e := range entries {
		seq.Content = append(seq.Content, themeNode(e))
	}
	return nil
}

func themeNode(e ContextTheme) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	add := func(k, v string) {
		if v == "" {
			return
		}
		n.Content = append(n.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Value: v},
		)
	}
	add("context", e.Context)
	add("kubeconfig", e.Kubeconfig)
	add("theme", e.Theme)
	return n
}

func appendThemeBlock(data []byte, entries []ContextTheme) []byte {
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		b.WriteByte('\n')
	}
	if len(bytes.TrimSpace(data)) > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("ui:\n  context_themes:\n")
	for _, e := range entries {
		b.WriteString("    - " + flowEntry(e) + "\n")
	}
	return []byte(b.String())
}

func flowEntry(e ContextTheme) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(themeNode(e))
	_ = enc.Close()
	return strings.TrimSpace(buf.String())
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".k2-config-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

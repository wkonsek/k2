package kube

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func FormatYAML(in string, display bool) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(in), &doc); err != nil {
		return "", err
	}
	root := docRoot(&doc)
	if root == nil {
		return "", nil
	}
	if display {
		trimYAML(root)
	}
	var b strings.Builder
	writeNode(&b, root, 0, false)
	s := b.String()
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s, nil
}

func YAMLEqual(a, b string) (bool, error) {
	var x, y any
	if err := yaml.Unmarshal([]byte(a), &x); err != nil {
		return false, err
	}
	if err := yaml.Unmarshal([]byte(b), &y); err != nil {
		return false, err
	}
	return yamlSame(x, y), nil
}

func yamlSame(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !yamlSame(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !yamlSame(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func trimYAML(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode && isStringTag(n.Tag) && isMultiline(n.Value) {
		n.Value = trimDisplay(n.Value)
	}
	for _, c := range n.Content {
		trimYAML(c)
	}
}

func trimDisplay(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	keep := 0
	for keep < len(s) && s[len(s)-1-keep] == '\n' {
		keep++
	}
	body := s
	nl := ""
	if keep > 0 {
		body = s[:len(s)-keep]
		nl = s[len(s)-keep:]
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + nl
}

func isMultiline(s string) bool {
	return strings.Contains(s, "\n") || strings.Contains(s, "\r")
}

func isStringTag(tag string) bool {
	return tag == "" || tag == "!!str" || tag == "!" || strings.HasSuffix(tag, ":str")
}

func writeNode(b *strings.Builder, n *yaml.Node, indent int, seq bool) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			writeNode(b, n.Content[0], indent, false)
		}
	case yaml.MappingNode:
		if len(n.Content) == 0 {
			writeScalarToken(b, "{}", seq, indent)
			return
		}
		if seq {
			b.WriteString(pad(indent) + "- ")
			writeMap(b, n, indent+2, true)
			return
		}
		writeMap(b, n, indent, false)
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			writeScalarToken(b, "[]", seq, indent)
			return
		}
		if seq {
			b.WriteString(pad(indent) + "-\n")
			indent += 2
		}
		for _, c := range n.Content {
			writeNode(b, c, indent, true)
		}
	case yaml.ScalarNode:
		writeScalar(b, n, indent, seq)
	case yaml.AliasNode:
		writeScalarToken(b, "*"+n.Value, seq, indent)
	}
}

func writeMap(b *strings.Builder, n *yaml.Node, indent int, firstCompact bool) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if i == 0 && firstCompact {
		} else {
			b.WriteString(pad(indent))
		}
		b.WriteString(formatKey(k))
		b.WriteString(":")
		writeValue(b, v, indent)
	}
}

func writeValue(b *strings.Builder, v *yaml.Node, indent int) {
	if v == nil || (v.Kind == yaml.ScalarNode && v.Tag == "!!null" && v.Value == "") {
		b.WriteString(" null\n")
		return
	}
	if v.Kind == yaml.MappingNode || v.Kind == yaml.SequenceNode {
		if len(v.Content) == 0 {
			b.WriteByte(' ')
			if v.Kind == yaml.MappingNode {
				b.WriteString("{}\n")
			} else {
				b.WriteString("[]\n")
			}
			return
		}
		b.WriteByte('\n')
		writeNode(b, v, indent+2, false)
		return
	}
	if v.Kind == yaml.ScalarNode && shouldBlock(v) {
		writeBlock(b, v.Value, indent+2, true)
		return
	}
	b.WriteByte(' ')
	writeScalar(b, v, indent+2, false)
}

func writeScalar(b *strings.Builder, n *yaml.Node, indent int, seq bool) {
	if n.Kind == yaml.ScalarNode && shouldBlock(n) {
		if seq {
			b.WriteString(pad(indent) + "- ")
			writeBlock(b, n.Value, indent+2, true)
			return
		}
		writeBlock(b, n.Value, indent, false)
		return
	}
	writeScalarToken(b, formatScalar(n), seq, indent)
}

func writeScalarToken(b *strings.Builder, tok string, seq bool, indent int) {
	if seq {
		b.WriteString(pad(indent) + "- " + tok + "\n")
		return
	}
	b.WriteString(tok)
	if !strings.HasSuffix(tok, "\n") {
		b.WriteByte('\n')
	}
}

func shouldBlock(n *yaml.Node) bool {
	if n == nil || n.Kind != yaml.ScalarNode || !isStringTag(n.Tag) {
		return false
	}
	if !isMultiline(n.Value) {
		return false
	}
	if strings.Contains(n.Value, "\r") {
		return false
	}
	return true
}

func writeBlock(b *strings.Builder, s string, contentIndent int, afterKey bool) {
	ind := "|"
	if !strings.HasSuffix(s, "\n") {
		ind = "|-"
	} else if strings.HasSuffix(s, "\n\n") {
		ind = "|+"
	}
	if afterKey {
		b.WriteByte(' ')
	}
	b.WriteString(ind)
	b.WriteByte('\n')
	p := pad(contentIndent)
	core := strings.TrimRight(s, "\n")
	for _, line := range strings.Split(core, "\n") {
		b.WriteString(p)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if ind == "|+" {
		n := len(s) - len(core)
		for i := 1; i < n; i++ {
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
}

func formatKey(n *yaml.Node) string {
	if n == nil {
		return `""`
	}
	if n.Kind != yaml.ScalarNode {
		return formatScalar(n)
	}
	return formatString(n.Value)
}

func formatScalar(n *yaml.Node) string {
	if n == nil {
		return "null"
	}
	tag := n.ShortTag()
	switch tag {
	case "!!null":
		if n.Value == "" || n.Value == "null" || n.Value == "~" {
			return "null"
		}
		return n.Value
	case "!!bool", "!!int", "!!float":
		if n.Value != "" {
			return n.Value
		}
	}
	return formatString(n.Value)
}

func formatString(s string) string {
	var n yaml.Node
	n.Kind = yaml.ScalarNode
	n.Tag = "!!str"
	n.Value = s
	if isMultiline(s) {
		n.Style = yaml.DoubleQuotedStyle
	}
	out, err := yaml.Marshal(&n)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimRight(string(out), "\n")
}

func pad(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

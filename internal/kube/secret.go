package kube

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k2/internal/shell"
)

func (c Client) Secret(kubeconfig, context, namespace, name string) (string, error) {
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		namespace, name = parts[0], parts[1]
	}
	args := []string{"get", "secret", name, "-o", "json", "--kubeconfig", kubeconfig, "--context", context}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil {
		return "", fmt.Errorf("%s", shell.FailText(out, err))
	}
	return DecodeSecret(out)
}

func DecodeSecret(raw []byte) (string, error) {
	var doc struct {
		Data map[string]string `json:"data"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return "", fmt.Errorf("cannot read secret")
	}
	if len(doc.Data) == 0 {
		return "(no data)", nil
	}
	keys := make([]string, 0, len(doc.Data))
	for k := range doc.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		raw, decErr := base64.StdEncoding.DecodeString(doc.Data[k])
		b.WriteString(k)
		b.WriteString("\n")
		if decErr != nil {
			b.WriteString("  [not base64]\n\n")
			continue
		}
		if IsBinary(raw) {
			b.WriteString(fmt.Sprintf("  [binary %d bytes]\n\n", len(raw)))
			continue
		}
		text := strings.TrimRight(string(raw), "\n")
		if text == "" {
			b.WriteString("  [empty]\n\n")
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func IsBinary(b []byte) bool {
	if bytes.Contains(b, []byte{0}) {
		return true
	}
	bad := 0
	for _, c := range b {
		if c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		if c < 32 || c == 127 {
			bad++
		}
	}
	return len(b) > 0 && bad*5 > len(b)
}

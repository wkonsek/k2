package kube

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"k2/internal/shell"
)

type SecretText struct {
	Decoded  string
	Certs    string
	HasCerts bool
}

func (c Client) Secret(kubeconfig, context, namespace, name string) (string, error) {
	v, err := c.SecretViews(kubeconfig, context, namespace, name)
	return v.Decoded, err
}

func (c Client) SecretViews(kubeconfig, context, namespace, name string) (SecretText, error) {
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
		return SecretText{}, fmt.Errorf("%s", shell.FailText(out, err))
	}
	return SecretViewsAt(out, time.Now())
}

func DecodeSecret(raw []byte) (string, error) {
	v, err := SecretViewsAt(raw, time.Now())
	return v.Decoded, err
}

func SecretViewsAt(raw []byte, now time.Time) (SecretText, error) {
	var doc struct {
		Data map[string]string `json:"data"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return SecretText{}, fmt.Errorf("cannot read secret")
	}
	if len(doc.Data) == 0 {
		return SecretText{Decoded: "(no data)", Certs: "(no data)"}, nil
	}
	keys := make([]string, 0, len(doc.Data))
	for k := range doc.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var plain, certs strings.Builder
	has := false
	for _, k := range keys {
		value, decErr := base64.StdEncoding.DecodeString(doc.Data[k])
		body := decodedBody(value, decErr)
		plain.WriteString(k + "\n" + body + "\n")
		if decErr != nil {
			certs.WriteString(k + "\n" + body + "\n")
			continue
		}
		entries, private := certEntries(value)
		switch {
		case len(entries) > 0:
			has = true
			certs.WriteString(certKeyText(k, entries, now))
		case private:
			certs.WriteString(k + "\n  [private key hidden]\n\n")
		default:
			certs.WriteString(k + "\n" + body + "\n")
		}
	}
	return SecretText{
		Decoded:  strings.TrimRight(plain.String(), "\n"),
		Certs:    strings.TrimRight(certs.String(), "\n"),
		HasCerts: has,
	}, nil
}

func decodedBody(raw []byte, decErr error) string {
	if decErr != nil {
		return "  [not base64]\n"
	}
	if IsBinary(raw) {
		return fmt.Sprintf("  [binary %d bytes]\n", len(raw))
	}
	text := strings.TrimRight(string(raw), "\n")
	if text == "" {
		return "  [empty]\n"
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func certKeyText(key string, entries []certEntry, now time.Time) string {
	var b strings.Builder
	b.WriteString(key)
	if len(entries) > 1 {
		fmt.Fprintf(&b, " · %d certificates", len(entries))
	}
	b.WriteString("\n")
	indent := "  "
	if len(entries) > 1 {
		indent = "    "
	}
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		if len(entries) > 1 {
			fmt.Fprintf(&b, "  certificate %d of %d\n", i+1, len(entries))
		}
		if e.cert == nil {
			b.WriteString(indent + "[cannot parse certificate]\n")
			continue
		}
		for _, l := range CertDetails(e.cert, now) {
			b.WriteString(indent + l + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
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

package kube

import (
	"os"
	"strings"
	"testing"

	"k2/internal/shell"

	"gopkg.in/yaml.v3"
)

const quotedNginx = `apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: shop
  annotations:
    note: "line one\nline two\n"
data:
  nginx.conf: "server {\n  listen 80;\n  server_name example.com;\n  location / {\n    root /usr/share/nginx/html;\n  }\n}\n"
  FEATURE_FLAG: "true"
`

func TestFormatYAMLMultilineBlock(t *testing.T) {
	got, err := FormatYAML(quotedNginx, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `\n`) {
		t.Fatalf("still escaped:\n%s", got)
	}
	if !strings.Contains(got, "nginx.conf: |") {
		t.Fatalf("nginx.conf not a block:\n%s", got)
	}
	if !strings.Contains(got, "  server_name example.com;") {
		t.Fatalf("missing line:\n%s", got)
	}
	if !strings.Contains(got, "note: |") {
		t.Fatalf("annotation not a block:\n%s", got)
	}
	if !strings.Contains(got, `FEATURE_FLAG: "true"`) && !strings.Contains(got, "FEATURE_FLAG: \"true\"") {
		t.Fatalf("bool-like string lost quotes:\n%s", got)
	}
}

func TestFormatYAMLRoundTrip(t *testing.T) {
	got, err := FormatYAML(quotedNginx, false)
	if err != nil {
		t.Fatal(err)
	}
	same, err := YAMLEqual(quotedNginx, got)
	if err != nil || !same {
		t.Fatalf("semantic equal %v %v\n%s", same, err, got)
	}
	var orig, pretty map[string]any
	if err := yaml.Unmarshal([]byte(quotedNginx), &orig); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(got), &pretty); err != nil {
		t.Fatal(err)
	}
	want := orig["data"].(map[string]any)["nginx.conf"].(string)
	have := pretty["data"].(map[string]any)["nginx.conf"].(string)
	if want != have {
		t.Fatalf("nginx.conf bytes\n want %#v\n have %#v", want, have)
	}
}

func TestFormatYAMLTrailingSpace(t *testing.T) {
	in := "data:\n  nginx.conf: \"server {  \\n  listen 80;  \\n}\\n\"\n"
	edit, err := FormatYAML(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edit, "nginx.conf: |") {
		t.Fatalf("edit should use block:\n%s", edit)
	}
	if !strings.Contains(edit, "server {  ") {
		t.Fatalf("edit lost trailing spaces:\n%s", edit)
	}
	var orig, out map[string]any
	yaml.Unmarshal([]byte(in), &orig)
	yaml.Unmarshal([]byte(edit), &out)
	if orig["data"].(map[string]any)["nginx.conf"] != out["data"].(map[string]any)["nginx.conf"] {
		t.Fatalf("edit round-trip\n orig %#v\n out %#v\n yaml:\n%s", orig["data"].(map[string]any)["nginx.conf"], out["data"].(map[string]any)["nginx.conf"], edit)
	}
	disp, err := FormatYAML(in, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disp, "nginx.conf: |") {
		t.Fatalf("display should use block:\n%s", disp)
	}
	if strings.Contains(disp, "server {  ") || strings.Contains(disp, "listen 80;  ") {
		t.Fatalf("display should trim trailing spaces:\n%s", disp)
	}
	var d map[string]any
	yaml.Unmarshal([]byte(disp), &d)
	got := d["data"].(map[string]any)["nginx.conf"].(string)
	if strings.Contains(got, " {  \n") || strings.HasSuffix(strings.TrimSuffix(got, "\n"), " ") {
		t.Fatalf("display value still has trailing spaces: %#v", got)
	}
}

func TestFormatYAMLTab(t *testing.T) {
	in := "data:\n  file: \"line\\twith tab\\nnext\\n\"\n"
	got, err := FormatYAML(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "file: |") {
		t.Fatalf("tabs should stay a block:\n%s", got)
	}
	var orig, out map[string]any
	yaml.Unmarshal([]byte(in), &orig)
	yaml.Unmarshal([]byte(got), &out)
	if orig["data"].(map[string]any)["file"] != out["data"].(map[string]any)["file"] {
		t.Fatalf("tab round-trip %#v vs %#v\n%s", orig["data"].(map[string]any)["file"], out["data"].(map[string]any)["file"], got)
	}
}

func TestFormatYAMLCRLF(t *testing.T) {
	raw := "data:\n  file: \"a\\r\\nb\\r\\n\"\n"
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	src, err := yaml.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := FormatYAML(string(src), false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := yaml.Unmarshal([]byte(edit), &out); err != nil {
		t.Fatal(err)
	}
	want := parsed["data"].(map[string]any)["file"].(string)
	have := out["data"].(map[string]any)["file"].(string)
	if want != have {
		t.Fatalf("edit must keep CR\n want %#v\n have %#v\n yaml:\n%s", want, have, edit)
	}
	if strings.Contains(want, "\r") && strings.Contains(edit, "file: |") {
		t.Fatalf("edit should quote CRLF, not use a block:\n%s", edit)
	}
	disp, err := FormatYAML(string(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disp, "file: |") {
		t.Fatalf("display should use a block for CRLF:\n%s", disp)
	}
	var d map[string]any
	yaml.Unmarshal([]byte(disp), &d)
	got := d["data"].(map[string]any)["file"].(string)
	if strings.Contains(got, "\r") {
		t.Fatalf("display should normalize CRLF: %#v", got)
	}
	if got != "a\nb\n" {
		t.Fatalf("display value %#v", got)
	}
}

func TestFormatYAMLChomp(t *testing.T) {
	cases := []string{
		"a\nb",
		"a\nb\n",
		"a\nb\n\n",
		"a\nb\n\n\n",
	}
	for _, s := range cases {
		in := "k: " + formatString(s) + "\n"
		got, err := FormatYAML(in, false)
		if err != nil {
			t.Fatalf("%#v: %v", s, err)
		}
		var out map[string]any
		if err := yaml.Unmarshal([]byte(got), &out); err != nil {
			t.Fatalf("%#v parse %v\n%s", s, err, got)
		}
		if out["k"] != s {
			t.Fatalf("chomp\n want %#v\n have %#v\n%s", s, out["k"], got)
		}
	}
}

func TestYAMLEqualIgnoresStyle(t *testing.T) {
	a := "k: \"a\\nb\\n\"\n"
	b, err := FormatYAML(a, false)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := YAMLEqual(a, b)
	if err != nil || !ok {
		t.Fatalf("equal %v %v\n%s", ok, err, b)
	}
	ok, err = YAMLEqual(a, "k: \"other\\n\"\n")
	if err != nil || ok {
		t.Fatalf("should differ %v %v", ok, err)
	}
}

func TestFormatYAMLSeqMap(t *testing.T) {
	in := `spec:
  containers:
  - name: nginx
    image: nginx:1.27
    args:
    - -c
    - |
      echo hello
      echo world
`
	got, err := FormatYAML(in, false)
	if err != nil {
		t.Fatal(err)
	}
	same, err := YAMLEqual(in, got)
	if err != nil || !same {
		t.Fatalf("equal %v %v\n%s", same, err, got)
	}
	if !strings.Contains(got, "args:") || !strings.Contains(got, "name: nginx") {
		t.Fatalf("lost fields:\n%s", got)
	}
}

func TestClientYAMLPrettyAndReplace(t *testing.T) {
	raw := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: nginx-config\ndata:\n  nginx.conf: \"server {\\n  listen 80;\\n}\\n\"\n"
	var replaced []string
	c := Client{Run: &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) >= 5 && r.Args[0] == "get" && r.Args[3] == "-o" && r.Args[4] == "yaml" {
			return shell.OK(raw)
		}
		if len(r.Args) > 0 && r.Args[0] == "replace" {
			replaced = append(replaced, r.Args[2])
			return shell.OK("configmap/nginx-config replaced")
		}
		return shell.Fail(1, "unexpected "+r.Line())
	}}}
	got, err := c.YAML("/kc", "kind-dev", "configmap", "nginx-config", "shop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "nginx.conf: |") || !strings.Contains(got, "  listen 80;") {
		t.Fatalf("display yaml not a block:\n%s", got)
	}
	edit, err := c.YAMLForEdit("/kc", "kind-dev", "configmap", "nginx-config", "shop")
	if err != nil {
		t.Fatal(err)
	}
	same, err := YAMLEqual(raw, edit)
	if err != nil || !same {
		t.Fatalf("edit yaml not equal: %v %v\n%s", same, err, edit)
	}
	tmp := t.TempDir() + "/obj.yaml"
	if err := os.WriteFile(tmp, []byte(edit), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.ReplaceYAML("/kc", "kind-dev", tmp); err != nil {
		t.Fatal(err)
	}
	if len(replaced) != 1 || replaced[0] != tmp {
		t.Fatalf("replace %v", replaced)
	}
}

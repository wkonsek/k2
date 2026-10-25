package kustomize

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k2/internal/shell"
)

const applyOut = `namespace/shop created
service/web created
deployment.apps/web configured
configmap/settings unchanged
secret/tls unchanged
serviceaccount/web serverside-applied
`

func TestSummary(t *testing.T) {
	got := Summary(applyOut)
	if got != "\n\ncreated 2   configured 1   unchanged 2   serverside-applied 1" {
		t.Fatalf("%q", got)
	}
	if Summary("error: no objects passed to apply") != "" {
		t.Fatal("summary for an error")
	}
}

func TestFinish(t *testing.T) {
	cases := []struct {
		verb string
		code int
		tail string
	}{
		{"apply", 0, "── exit 0 · done ──"},
		{"apply", 1, "── exit 1 · failed ──"},
		{"diff", 1, "── exit 1 · differences ──"},
		{"diff", 2, "── exit 2 · failed ──"},
		{"delete", 0, "── exit 0 · done ──"},
	}
	for _, c := range cases {
		got := Finish(applyOut, c.code, c.verb)
		if !strings.HasSuffix(got, "\n\n"+c.tail) || !strings.Contains(got, "created 2") {
			t.Errorf("Finish(%s,%d) = %q", c.verb, c.code, got)
		}
	}
}

func TestIsFileAndList(t *testing.T) {
	for _, n := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization", "KUSTOMIZATION.YAML"} {
		if !IsFile(n) {
			t.Errorf("IsFile(%q)", n)
		}
	}
	if IsFile("kustomization.json") {
		t.Fatal("json accepted")
	}
	root := t.TempDir()
	mk := func(p string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		_ = os.WriteFile(filepath.Join(root, p), []byte("resources: []\n"), 0o644)
	}
	mk("kustomization.yaml")
	mk("base/kustomization.yml")
	mk("overlays/prod/kustomization.yaml")
	mk("plain/readme.md")
	mk(".git/kustomization.yaml")
	mk("vendor/x/kustomization.yaml")
	entries, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name+"="+e.Kind)
	}
	want := []string{"..=parent", "kustomization.yaml=file", "base=kust", "overlays=", "plain="}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v", got)
	}
	found := Find(root)
	wantDirs := []string{root, filepath.Join(root, "base"), filepath.Join(root, "overlays/prod")}
	if !reflect.DeepEqual(found, wantDirs) {
		t.Fatalf("Find = %v", found)
	}
	if FileIn(filepath.Join(root, "base")) == "" || FileIn(filepath.Join(root, "plain")) != "" {
		t.Fatal("FileIn")
	}
}

func TestRun(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Args[0] == "diff" {
			return shell.Result{Stdout: []byte("+ replicas: 3"), Code: 1, Err: os.ErrInvalid}
		}
		return shell.Result{Stdout: []byte(applyOut), Stderr: []byte("Warning: x")}
	}}
	text, code := Run(f, "apply", "/k/app", "/kc", "ctx")
	if code != 0 || !strings.HasSuffix(text, "Warning: x") {
		t.Fatalf("%d %q", code, text)
	}
	if got := f.Calls()[0].Line(); got != "kubectl apply -k /k/app --kubeconfig /kc --context ctx" {
		t.Fatal(got)
	}
	if _, code := Run(f, "diff", "/k/app", "/kc", "ctx"); code != 1 {
		t.Fatal(code)
	}
	f.Handler = func(shell.Request) shell.Result { return shell.Result{} }
	if text, _ := Run(f, "diff", "/d", "/kc", "c"); text != "no differences" {
		t.Fatal(text)
	}
	f.Handler = func(shell.Request) shell.Result { return shell.Result{Code: -1, Err: os.ErrNotExist} }
	if text, code := Run(f, "apply", "/d", "/kc", "c"); code != 1 || text != os.ErrNotExist.Error() {
		t.Fatal(text, code)
	}
}

func TestListShowsEveryFile(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		_ = os.WriteFile(filepath.Join(root, p), []byte("x\n"), 0o644)
	}
	mk("kustomization.yaml")
	mk("deploy.yaml")
	mk("README.md")
	mk(".env")
	mk("base/kustomization.yaml")
	mk("node_modules/a.js")
	if err := os.Symlink(filepath.Join(root, "base"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	entries, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name+"="+e.Kind)
	}
	want := []string{"..=parent", "kustomization.yaml=file", "base=kust", "link=kust", ".env=other", "README.md=other", "deploy.yaml=other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v", got)
	}
	for name, yaml := range map[string]bool{"a.yaml": true, "B.YML": true, "c.json": false, "Makefile": false} {
		if IsYAML(name) != yaml {
			t.Errorf("IsYAML(%q) = %v", name, !yaml)
		}
	}
}

package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestYAMLDeclaresNamespace(t *testing.T) {
	withNS := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: key\n  namespace: kube-system\n"
	if !YAMLDeclaresNamespace(withNS) {
		t.Fatal("declared namespace")
	}
	without := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: key\n"
	if YAMLDeclaresNamespace(without) {
		t.Fatal("missing namespace")
	}
	mixed := withNS + "---\n" + without
	if YAMLDeclaresNamespace(mixed) {
		t.Fatal("mixed docs should pass -n")
	}
	if YAMLDeclaresNamespace("") || YAMLDeclaresNamespace("not: [yaml") {
		t.Fatal("empty or invalid")
	}
}

func TestMasterKeyApplyArgsOmitsNamespaceWhenDeclared(t *testing.T) {
	withNS := "metadata:\n  name: k\n  namespace: kube-system\n"
	got := MasterKeyApplyArgs("/tmp/key.yml", "kube-system", "/kc", "kind-k2", withNS)
	want := "apply -f /tmp/key.yml --kubeconfig /kc --context kind-k2"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %q", strings.Join(got, " "))
	}
	noNS := "metadata:\n  name: k\n"
	got = MasterKeyApplyArgs("/tmp/key.yml", "shop", "/kc", "kind-k2", noNS)
	want = "apply -f /tmp/key.yml -n shop --kubeconfig /kc --context kind-k2"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %q", strings.Join(got, " "))
	}
}

func TestApplyMasterKeyMissingFile(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result { return shell.OK("ok") }}
	err := ApplyMasterKey(f, "/no/such/master.yml", "kube-system", "/kc", "kind-k2", false, nil)
	if err == nil || !strings.Contains(err.Error(), "master key file not found") {
		t.Fatalf("err %v", err)
	}
	if len(f.Calls()) != 0 {
		t.Fatalf("should not call kubectl: %v", f.Calls())
	}
}

func TestApplyMasterKeyRunsBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.yml")
	body := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: sealed-secrets-key\n  namespace: kube-system\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Name != "kubectl" {
			return shell.Fail(1, "unexpected "+r.Name)
		}
		return shell.OK("secret/sealed-secrets-key created")
	}}
	err := ApplyMasterKey(f, path, "kube-system", "/kc", "kind-k2", false, func(line string) { log.WriteString(line + "\n") })
	if err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if len(calls) != 1 || calls[0].Name != "kubectl" {
		t.Fatalf("calls %v", calls)
	}
	line := strings.Join(calls[0].Args, " ")
	if !strings.HasPrefix(line, "apply -f "+path) || strings.Contains(line, "-n ") || !strings.Contains(line, "--kubeconfig /kc") || !strings.Contains(line, "--context kind-k2") {
		t.Fatalf("args %q", line)
	}
	if !strings.Contains(log.String(), "kubectl apply -f") || !strings.Contains(log.String(), "secret/sealed-secrets-key created") {
		t.Fatalf("log %q", log.String())
	}
}

func TestApplyMasterKeyCreatesNamespaceWhenAsked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.yml")
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if strings.HasPrefix(line, "create namespace shop") {
			return shell.OK("namespace/shop created")
		}
		if strings.HasPrefix(line, "apply -f") && strings.Contains(line, "-n shop") {
			return shell.OK("secret/key created")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	if err := ApplyMasterKey(f, path, "shop", "/kc", "ctx", true, nil); err != nil {
		t.Fatal(err)
	}
	var verbs []string
	for _, c := range f.Calls() {
		verbs = append(verbs, strings.Join(c.Args, " "))
	}
	if len(verbs) != 2 || !strings.HasPrefix(verbs[0], "create namespace shop") || !strings.Contains(verbs[1], "apply -f") {
		t.Fatalf("order %v", verbs)
	}
}

func TestApplyMasterKeyIgnoresExistingNamespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.yml")
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if strings.HasPrefix(line, "create namespace shop") {
			return shell.Fail(1, `Error from server (AlreadyExists): namespaces "shop" already exists`)
		}
		if strings.HasPrefix(line, "apply -f") {
			return shell.OK("secret/key configured")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	if err := ApplyMasterKey(f, path, "shop", "", "", true, nil); err != nil {
		t.Fatal(err)
	}
}

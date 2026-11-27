package kube

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"k2/internal/shell"
)

const helmSecret = `{"apiVersion":"v1","kind":"Secret","type":"kubernetes.io/tls","immutable":true,
 "metadata":{"name":"web-tls","namespace":"shop","uid":"u-1","resourceVersion":"42","creationTimestamp":"2026-10-22T10:00:00Z",
  "generation":3,"finalizers":["x/y"],"managedFields":[{"manager":"helm"}],"ownerReferences":[{"kind":"Certificate","name":"web"}],
  "labels":{"app":"web","app.kubernetes.io/managed-by":"Helm","helm.sh/chart":"web-1.2.3","app.kubernetes.io/instance":"web"},
  "annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{}","meta.helm.sh/release-name":"web",
   "meta.helm.sh/release-namespace":"shop","helm.sh/resource-policy":"keep","team":"core"}},
 "data":{"tls.crt":"Y3J0","tls.key":"a2V5"}}`

func TestSecretCopyStripsServerAndHelmFields(t *testing.T) {
	out, err := SecretCopy([]byte(helmSecret), "web-tls-copy")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	meta := doc["metadata"].(map[string]any)
	wantMeta := map[string]any{
		"name":        "web-tls-copy",
		"labels":      map[string]any{"app": "web", "app.kubernetes.io/instance": "web"},
		"annotations": map[string]any{"team": "core"},
	}
	if !reflect.DeepEqual(meta, wantMeta) {
		t.Fatalf("metadata %v", meta)
	}
	if doc["type"] != "kubernetes.io/tls" || doc["immutable"] != true || doc["kind"] != "Secret" || doc["apiVersion"] != "v1" {
		t.Fatalf("doc %v", doc)
	}
	if !reflect.DeepEqual(doc["data"], map[string]any{"tls.crt": "Y3J0", "tls.key": "a2V5"}) {
		t.Fatalf("data %v", doc["data"])
	}
}

func TestSecretCopyKeepsNonHelmManagedBy(t *testing.T) {
	in := `{"kind":"Secret","metadata":{"name":"a","labels":{"app.kubernetes.io/managed-by":"kustomize"},"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{}"}}}`
	out, err := SecretCopy([]byte(in), "a")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"app.kubernetes.io/managed-by":"kustomize"`) || strings.Contains(s, "annotations") {
		t.Fatalf("got %s", s)
	}
}

func TestSecretCopyRejects(t *testing.T) {
	if _, err := SecretCopy([]byte("nope"), "a"); err == nil {
		t.Fatal("bad json accepted")
	}
	if _, err := SecretCopy([]byte(`{"kind":"ConfigMap","metadata":{"name":"a"}}`), "a"); err == nil {
		t.Fatal("configmap accepted")
	}
}

func TestValidSecretName(t *testing.T) {
	for _, ok := range []string{"a", "web-tls", "a.b-c", "x1"} {
		if err := ValidSecretName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Web", "-a", "a-", "a_b", "a b", strings.Repeat("a", 254)} {
		if ValidSecretName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCopySecretCreatesOrReplaces(t *testing.T) {
	var stdin, verb string
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(line, "get secret web-tls -n shop -o json"):
			return shell.OK(helmSecret)
		case r.Args[0] == "create" || r.Args[0] == "replace":
			verb, stdin = line, r.Stdin
			return shell.OK("secret/web-tls created")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	c := New(f)
	if err := c.CopySecret("/kc", "dev", "shop", "web-tls", "stage", "web-tls", false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(verb, "create -f - -n stage --kubeconfig /kc --context dev") || strings.Contains(stdin, "uid") || strings.Contains(stdin, `"namespace"`) {
		t.Fatalf("verb %q stdin %s", verb, stdin)
	}
	if err := c.CopySecret("/kc", "dev", "shop", "web-tls", "stage", "web-tls", true); err != nil || !strings.HasPrefix(verb, "replace -f - -n stage") {
		t.Fatalf("replace %q %v", verb, err)
	}
}

func TestCopySecretErrors(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Args[0] == "get" {
			return shell.Fail(1, `Error from server (NotFound): secrets "gone" not found`)
		}
		return shell.OK("")
	}}
	err := New(f).CopySecret("/kc", "dev", "shop", "gone", "stage", "gone", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err %v", err)
	}
}

func TestSecretExists(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(strings.Join(r.Args, " "), "get secret there -n stage --ignore-not-found -o name") {
			return shell.OK("secret/there\n")
		}
		return shell.OK("")
	}}
	c := New(f)
	if ok, err := c.SecretExists("/kc", "dev", "stage", "there"); !ok || err != nil {
		t.Fatalf("there: %v %v", ok, err)
	}
	if ok, err := c.SecretExists("/kc", "dev", "stage", "missing"); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
}

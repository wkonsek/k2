package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRegistryProxyDefaults(t *testing.T) {
	r := Default().Kind.RegistryProxy
	if r.Image != "rpardini/docker-registry-proxy:latest" || r.Container != "k2-registry-proxy" {
		t.Fatalf("%+v", r)
	}
	if r.CacheDir != filepath.Join(DefaultDir(), "kind-cache") {
		t.Fatalf("cache_dir %s", r.CacheDir)
	}
	if !reflect.DeepEqual(r.Registries, []string{"docker.io", "registry.k8s.io", "gcr.io", "quay.io", "ghcr.io"}) {
		t.Fatalf("registries %v", r.Registries)
	}
	if DefaultKeys().KindProxy.Label() != "p" {
		t.Fatal(DefaultKeys().KindProxy)
	}
}

func TestRegistryProxyOverrides(t *testing.T) {
	c, w := Parse([]byte(`kind:
  registry_proxy:
    image: example/proxy:1
    container: my-cache
    cache_dir: ~/cache/kind
    registries:
      - quay.io
      - ghcr.io
keys:
  kind_proxy: P
`))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	r := c.Kind.RegistryProxy
	if r.Image != "example/proxy:1" || r.Container != "my-cache" || r.CacheDir != ExpandHome("~/cache/kind") || !reflect.DeepEqual(r.Registries, []string{"quay.io", "ghcr.io"}) {
		t.Fatalf("%+v", r)
	}
	if c.Kind.Binary != "kind" || c.Keys.KindProxy.Label() != "P" {
		t.Fatalf("binary %q key %v", c.Kind.Binary, c.Keys.KindProxy)
	}
}

func TestRegistryProxyBadValues(t *testing.T) {
	c, w := Parse([]byte(`kind:
  registry_proxy:
    image: ""
    container: "bad name"
    registries: [quay.io, "not/a/host"]
    colour: red
`))
	r := c.Kind.RegistryProxy
	def := DefaultRegistryProxy()
	if r.Image != def.Image || r.Container != def.Container || !reflect.DeepEqual(r.Registries, []string{"quay.io"}) {
		t.Fatalf("%+v", r)
	}
	all := strings.Join(w, "\n")
	for _, want := range []string{"image: empty", "not a valid container name", "not a registry host", `unknown setting "colour"`} {
		if !strings.Contains(all, want) {
			t.Fatalf("warnings lack %q:\n%s", want, all)
		}
	}
	c, w = Parse([]byte("kind:\n  registry_proxy: yes\n"))
	if len(w) != 1 || !reflect.DeepEqual(c.Kind.RegistryProxy, def) {
		t.Fatalf("%v %+v", w, c.Kind.RegistryProxy)
	}
}

func TestKindProxyKeyConflicts(t *testing.T) {
	_, w := Parse([]byte("keys:\n  kind_proxy: e\n"))
	if len(w) != 1 || !strings.Contains(w[0], "kind_proxy") {
		t.Fatalf("want a conflict with edit, got %v", w)
	}
}

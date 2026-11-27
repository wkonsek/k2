package kube

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKindKubeconfigPaths(t *testing.T) {
	home := "/home/wojtek"
	if got := KindKubeconfig("k2", home); got != filepath.Join(home, ".kube", "k2") {
		t.Fatalf("k2: %s", got)
	}
	if got := KindKubeconfig("lab", home); got != filepath.Join(home, ".kube", "k2-lab") {
		t.Fatalf("lab: %s", got)
	}
	if KindContext("lab") != "kind-lab" || KindContext("k2") != "kind-k2" || KindContext(" ") != "" {
		t.Fatalf("context %q %q %q", KindContext("lab"), KindContext("k2"), KindContext(" "))
	}
	if KindKubeconfig("", home) != "" {
		t.Fatal("empty name")
	}
	cfg := filepath.Join(home, ".kube", "config")
	if ManagedKindKubeconfig(cfg, "k2", home) || ManagedKindKubeconfig(cfg, "config", home) {
		t.Fatal("must never treat ~/.kube/config as managed")
	}
	if !ManagedKindKubeconfig(KindKubeconfig("k2", home), "k2", home) {
		t.Fatal("k2 should be managed")
	}
	if !ManagedKindKubeconfig(KindKubeconfig("lab", home), "lab", home) {
		t.Fatal("lab should be managed")
	}
	if ManagedKindKubeconfig(KindKubeconfig("lab", home), "k2", home) {
		t.Fatal("lab path is not the k2 file")
	}
}

func TestKindConfigYAML(t *testing.T) {
	got, err := KindConfigYAML(1, 0, "")
	if err != nil || got != "kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n" {
		t.Fatalf("default: %q %v", got, err)
	}
	got, err = KindConfigYAML(1, 2, "kindest/node:v1.31.0")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"kind: Cluster",
		"apiVersion: kind.x-k8s.io/v1alpha4",
		"nodes:",
		"- role: control-plane",
		"  image: kindest/node:v1.31.0",
		"- role: worker",
		"  image: kindest/node:v1.31.0",
		"- role: worker",
		"  image: kindest/node:v1.31.0",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("nodes:\n%s", got)
	}
	if _, err := KindConfigYAML(0, 1, ""); err == nil {
		t.Fatal("zero control-plane should fail")
	}
	if _, err := KindConfigYAML(1, -1, ""); err == nil {
		t.Fatal("negative workers should fail")
	}
}

func TestKindCreateArgs(t *testing.T) {
	got := strings.Join(KindCreateArgs("k2", "/home/u/.kube/k2", "/tmp/kind.yaml", "kindest/node:v1.31.0"), " ")
	want := "create cluster --name k2 --kubeconfig /home/u/.kube/k2 --config /tmp/kind.yaml --image kindest/node:v1.31.0 --wait 5m"
	if got != want {
		t.Fatalf("%s", got)
	}
	if strings.Join(KindDeleteArgs("lab"), " ") != "delete cluster --name lab" {
		t.Fatal(KindDeleteArgs("lab"))
	}
}

func TestParseKindClusters(t *testing.T) {
	got := ParseKindClusters("k2\nlab\n\nWARNING: something\nerror: nope\n")
	if len(got) != 2 || got[0] != "k2" || got[1] != "lab" {
		t.Fatalf("%q", got)
	}
}

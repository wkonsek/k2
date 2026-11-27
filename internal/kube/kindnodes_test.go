package kube

import "testing"

func TestParseKindNodes(t *testing.T) {
	out := "k2\tcontrol-plane\tkindest/node:v1.33.1\trunning\tUp 2 hours\n" +
		"lab\tcontrol-plane\tkindest/node:v1.31.0\trunning\tUp 5 minutes\n" +
		"lab\tworker\tkindest/node:v1.31.0\trunning\tUp 5 minutes\n" +
		"lab\tworker\tkindest/node:v1.31.0\texited\tExited (0) 3 minutes ago\n" +
		"dead\tcontrol-plane\tkindest/node:v1.29.2\texited\tExited (0) 1 hour ago\n"
	got := ParseKindNodes(out)
	k2 := got["k2"]
	if k2.Planes != 1 || k2.Workers != 0 || k2.Running != 1 || k2.Total != 1 || k2.Version != "v1.33.1" {
		t.Fatalf("k2 %+v", k2)
	}
	if s := k2.Summary(); s != "1 cp · 0 workers · v1.33.1" {
		t.Fatalf("k2 summary %q", s)
	}
	lab := got["lab"]
	if lab.Planes != 1 || lab.Workers != 2 || lab.Running != 2 || lab.Total != 3 || lab.Version != "v1.31.0" {
		t.Fatalf("lab %+v", lab)
	}
	if s := lab.Summary(); s != "1 cp · 2 workers · v1.31.0 · 2/3" {
		t.Fatalf("lab summary %q", s)
	}
	dead := got["dead"]
	if dead.Running != 0 || dead.Total != 1 || dead.Summary() != "1 cp · 0 workers · v1.29.2 · 0/1" {
		t.Fatalf("dead %+v %q", dead, dead.Summary())
	}
}

func TestKindImageVersion(t *testing.T) {
	cases := map[string]string{
		"kindest/node:v1.33.1":            "v1.33.1",
		"kindest/node:v1.33.1@sha256:abc": "v1.33.1",
		"private.example/kind/node:1.2.3": "1.2.3",
		"kindest/node":                    "",
		"kindest/node@sha256:abc":         "",
		"":                                "",
	}
	for in, want := range cases {
		if got := KindImageVersion(in); got != want {
			t.Errorf("KindImageVersion(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDockerKindNodesArgs(t *testing.T) {
	args := DockerKindNodesArgs()
	if len(args) < 5 || args[0] != "ps" {
		t.Fatalf("%v", args)
	}
}

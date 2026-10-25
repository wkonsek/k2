package kube

import (
	"strings"
	"testing"

	"k2/internal/shell"
)

const versionJSON = `{
  "clientVersion": {"major": "1", "minor": "31", "gitVersion": "v1.31.2"},
  "kustomizeVersion": "v5.4.2",
  "serverVersion": {"major": "1", "minor": "29", "gitVersion": "v1.29.3+k3s1"}
}`

func TestClusterVersions(t *testing.T) {
	var ocpAsked bool
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		args := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(args, "version -o json --request-timeout=5s --kubeconfig /kc --context dev"):
			return shell.OK(versionJSON)
		case strings.HasPrefix(args, "get clusterversion version"):
			ocpAsked = true
			return shell.OK("4.16.3")
		}
		return shell.Fail(1, "unexpected "+args)
	}}
	v := New(f).ClusterVersions("/kc", "dev")
	if v != (Versions{Client: "v1.31.2", Server: "v1.29.3+k3s1", OpenShift: "4.16.3"}) || !ocpAsked {
		t.Fatalf("versions %+v", v)
	}
}

func TestClusterVersionsDegrade(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Args[0] == "version" {
			return shell.OK(versionJSON)
		}
		return shell.Fail(1, `Error from server (Forbidden): clusterversions.config.openshift.io "version" is forbidden`)
	}}
	if v := New(f).ClusterVersions("/kc", "dev"); v.Server == "" || v.OpenShift != "" {
		t.Fatalf("forbidden: %+v", v)
	}
	calls := 0
	f = &shell.Fake{Handler: func(r shell.Request) shell.Result {
		calls++
		res := shell.Fail(1, "")
		res.Stdout = []byte(`{"clientVersion": {"gitVersion": "v1.31.2"}}` + "\n")
		res.Stderr = []byte("The connection to the server localhost:8080 was refused")
		return res
	}}
	if v := New(f).ClusterVersions("/kc", "dev"); v != (Versions{Client: "v1.31.2"}) || calls != 1 {
		t.Fatalf("unreachable: %+v calls %d", v, calls)
	}
	if v := ParseVersion([]byte("garbage")); v != (Versions{}) {
		t.Fatalf("garbage: %+v", v)
	}
}

func TestClusterVersionsClientOnly(t *testing.T) {
	var got []string
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		got = append(got, strings.Join(r.Args, " "))
		return shell.OK(`{"clientVersion": {"gitVersion": "v1.31.2"}}`)
	}}
	v := New(f).ClusterVersions("", "")
	if v.Client != "v1.31.2" || len(got) != 1 || !strings.HasSuffix(got[0], "--client") {
		t.Fatalf("%+v %v", v, got)
	}
}

package kube

import (
	"testing"

	"k2/internal/shell"
)

func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		text   string
		want   Failure
		reason string
	}{
		{"error: You must be logged in to the server (Unauthorized)", FailAuth, "token expired or rejected"},
		{"Error from server (Unauthorized): the server has asked for the client to provide credentials", FailAuth, "token expired or rejected"},
		{"error: the server has asked for the client to provide credentials", FailAuth, "token expired or rejected"},
		{"Unable to connect to the server: getting credentials: exec: executable oc failed with exit code 1", FailAuth, "credentials failed"},
		{"error: You must be logged in to the server (the server has asked for the client to provide credentials)", FailAuth, "token expired or rejected"},
		{"Unable to connect to the server: tls: expired certificate", FailAuth, "certificate expired"},
		{"Unable to connect to the server: dial tcp 10.0.0.1:6443: connect: connection refused", FailConn, "connection refused"},
		{"Unable to connect to the server: dial tcp: lookup api.x.example: no such host", FailConn, "no such host"},
		{"Unable to connect to the server: dial tcp 10.0.0.1:6443: i/o timeout", FailConn, "timed out"},
		{"Unable to connect to the server: net/http: TLS handshake timeout", FailConn, "timed out"},
		{"Unable to connect to the server: dial tcp 10.0.0.1:6443: connect: no route to host", FailConn, "no route to host"},
		{"Unable to connect to the server: dial tcp: connect: network is unreachable", FailConn, "network unreachable"},
		{"Error from server (Forbidden): pods is forbidden", FailOther, "Error from server (Forbidden): pods is forbidden"},
		{"error: the server doesn't have a resource type \"foo\"", FailOther, "the server doesn't have a resource type \"foo\""},
	}
	for _, c := range cases {
		if got := ClassifyFailure(c.text); got != c.want {
			t.Errorf("ClassifyFailure(%q) = %v, want %v", c.text, got, c.want)
			continue
		}
		if got := FailureReason(c.want, c.text); got != c.reason {
			t.Errorf("FailureReason(%q) = %q, want %q", c.text, got, c.reason)
		}
	}
}

func TestAPIListErrorCarriesTheClusterMessage(t *testing.T) {
	_, err := Client{Run: &shell.Fake{Handler: func(shell.Request) shell.Result {
		return shell.Fail(1, "error: You must be logged in to the server (Unauthorized)")
	}}}.APIList("/kc", "ctx")
	if err == nil || ClassifyFailure(err.Error()) != FailAuth {
		t.Fatalf("err %v", err)
	}
}

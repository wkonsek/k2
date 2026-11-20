package ui

import (
	"strings"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"
)

const apiTable = "secrets   secret   v1   true   Secret\npods   po   v1   true   Pod\n"

func discoveryFake(apiRes shell.Result, get shell.Result) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(line, "api-resources"):
			return apiRes
		case strings.Contains(line, " get "), strings.HasPrefix(line, "get "):
			return get
		}
		return shell.Fail(1, "unexpected "+line)
	}}
}

func discovered(t *testing.T, f *shell.Fake) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	msg := fetchResources(m.k, m.kubeconfig, m.context)()
	next, _ := m.Update(msg)
	return next.(model)
}

func TestExpiredTokenShowsLoginHintNotUnknownResource(t *testing.T) {
	unauth := shell.Fail(1, "error: You must be logged in to the server (Unauthorized)")
	m := discovered(t, discoveryFake(unauth, unauth))
	next, cmd := m.runCommand("certs")
	m = next.(model)
	want := "not logged in: token expired or rejected, press " + m.loginHint()
	if m.status != want {
		t.Fatalf("status %q, want %q", m.status, want)
	}
	if strings.Contains(m.status, "unknown resource") {
		t.Fatalf("status %q", m.status)
	}
	if cmd == nil || m.toast.tone != toneErr || m.toast.text != want {
		t.Fatalf("toast %+v cmd %v", m.toast, cmd)
	}
}

func TestKnownExpiredTokenNamesItInTheMessage(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.tok = tokenState{ctx: infoKey(m.kubeconfig, m.context), life: kube.TokenLife{Expired: true}}
	next, _ := m.runCommand("certs")
	m = next.(model)
	want := "not logged in: token expired, press " + m.loginHint()
	if m.status != want || m.toast.tone != toneErr {
		t.Fatalf("status %q tone %v, want %q", m.status, m.toast.tone, want)
	}

	m = discovered(t, discoveryFake(shell.Fail(1, "error: You must be logged in to the server (Unauthorized)"), shell.Fail(1, "")))
	m.tok = tokenState{ctx: infoKey(m.kubeconfig, m.context), life: kube.TokenLife{Expired: true}}
	next, _ = m.runCommand("certs")
	if got := next.(model).status; got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
}

func TestUnreachableClusterShowsConnectivityError(t *testing.T) {
	down := shell.Fail(1, "Unable to connect to the server: dial tcp 10.0.0.1:6443: connect: connection refused")
	m := discovered(t, discoveryFake(down, down))
	next, _ := m.runCommand("certs")
	m = next.(model)
	if m.status != "cannot reach cluster: connection refused" || m.toast.tone != toneErr {
		t.Fatalf("status %q tone %v", m.status, m.toast.tone)
	}

	timeout := shell.Fail(1, "Unable to connect to the server: net/http: TLS handshake timeout")
	m = discovered(t, discoveryFake(timeout, timeout))
	next, _ = m.runCommand("certs")
	if got := next.(model).status; got != "cannot reach cluster: timed out" {
		t.Fatalf("status %q", got)
	}
}

func TestGenuinelyUnknownResourceStaysUnknown(t *testing.T) {
	m := discovered(t, discoveryFake(shell.OK(apiTable), shell.OK(`{"items":[]}`)))
	if len(m.apis) == 0 || m.resFail != kube.FailNone {
		t.Fatalf("discovery apis %d fail %v", len(m.apis), m.resFail)
	}
	next, _ := m.runCommand("franek")
	m = next.(model)
	if m.status != "unknown resource: franek" || m.toast.tone != toneWarn {
		t.Fatalf("status %q tone %v", m.status, m.toast.tone)
	}
}

func TestOtherDiscoveryFailureIsNotUnknownResource(t *testing.T) {
	m := discovered(t, discoveryFake(shell.Fail(1, "error: something odd happened"), shell.Fail(1, "")))
	next, _ := m.runCommand("certs")
	got := next.(model).status
	if strings.Contains(got, "unknown resource") || !strings.Contains(got, "something odd happened") {
		t.Fatalf("status %q", got)
	}
}

func TestListLoadUnauthorizedShowsLoginHint(t *testing.T) {
	unauth := shell.Fail(1, "error: You must be logged in to the server (Unauthorized)")
	m := newTestModel(t, testEnv(t), discoveryFake(shell.OK(apiTable), unauth))
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	want := "not logged in: token expired or rejected, press " + m.loginHint()
	if m.status != want || m.toast.tone != toneErr || !strings.HasPrefix(m.toast.text, want) {
		t.Fatalf("status %q toast %+v", m.status, m.toast)
	}

	next, cmd = m.openPods()
	m = settle(t, next.(model), cmd)
	if m.status != want {
		t.Fatalf("pods status %q", m.status)
	}
}

func TestListLoadUnreachableShowsConnectivityError(t *testing.T) {
	down := shell.Fail(1, "Unable to connect to the server: dial tcp: lookup api.example.com: no such host")
	m := newTestModel(t, testEnv(t), discoveryFake(shell.OK(apiTable), down))
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	if m.status != "cannot reach cluster: no such host" || m.toast.tone != toneErr {
		t.Fatalf("status %q tone %v", m.status, m.toast.tone)
	}
}

func TestOrdinaryListErrorKeepsItsMessage(t *testing.T) {
	m := newTestModel(t, testEnv(t), discoveryFake(shell.OK(apiTable), shell.Fail(1, "Error from server (Forbidden): secrets is forbidden")))
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	if !strings.Contains(m.status, "forbidden") || strings.Contains(m.status, "not logged in") || !strings.HasPrefix(m.toast.text, "could not list secrets") {
		t.Fatalf("status %q toast %q", m.status, m.toast.text)
	}
}

func TestFailedDiscoveryIsRetriedOnTheNextCommand(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.resContext, m.resFail = m.context, kube.FailAuth
	m.stage = "nav"
	_, cmd := m.Update(key(t, ":"))
	if cmd == nil {
		t.Fatal("no discovery retry")
	}
}

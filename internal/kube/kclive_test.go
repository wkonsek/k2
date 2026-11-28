package kube

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"gitVersion":"v1.30.0"}`))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer good", "Bearer exec-token":
			w.Write([]byte(`{"kind":"APIVersions"}`))
		case "Bearer limited":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	})
	mux.HandleFunc("/apis/authentication.k8s.io/v1/selfsubjectreviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte(`{"status":{"userInfo":{"username":"alice"}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func liveFor(t *testing.T, server, user string, run shell.Runner) Live {
	t.Helper()
	p := writeKC(t, server, map[string]string{"prod": user})
	return CheckContext(run, p, "prod")
}

func TestReachabilityIsSeparateFromAccess(t *testing.T) {
	srv := fakeAPI(t)
	cases := []struct {
		name   string
		user   string
		access AccessState
		user_  string
	}{
		{"valid token", "    token: good\n", AccessOK, "alice"},
		{"expired token", "    token: stale\n", AccessUnauthorized, ""},
		{"forbidden token", "    token: limited\n", AccessForbidden, ""},
	}
	for _, c := range cases {
		live := liveFor(t, srv.URL, c.user, nil)
		if !live.Reach.OK || live.Reach.Code != 200 {
			t.Fatalf("%s: reach %+v", c.name, live.Reach)
		}
		if live.Access.State != c.access || live.Access.User != c.user_ {
			t.Fatalf("%s: access %+v", c.name, live.Access)
		}
	}
}

func TestAnyHTTPAnswerIsReachable(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		live := liveFor(t, srv.URL, "    token: x\n", nil)
		srv.Close()
		if !live.Reach.OK || live.Reach.Code != code {
			t.Fatalf("code %d: reach %+v", code, live.Reach)
		}
	}
}

func TestClosedPortIsUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	live := liveFor(t, "http://"+addr, "    token: good\n", nil)
	if live.Reach.OK || live.Reach.Reason != "connection refused" {
		t.Fatalf("reach %+v", live.Reach)
	}
	if live.Access.State != AccessUnknown || live.Access.Reason == "" {
		t.Fatalf("access %+v", live.Access)
	}
}

func TestSlowServerTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	live := liveFor(t, srv.URL, "    token: good\n", nil)
	if live.Reach.OK || live.Reach.Reason != "timeout" {
		t.Fatalf("reach %+v", live.Reach)
	}
	if d := time.Since(start); d > LiveTimeout+2*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestTLSFailureIsUnreachableWithReason(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }))
	defer srv.Close()
	live := liveFor(t, srv.URL, "    token: good\n", nil)
	if live.Reach.OK || live.Reach.Reason != "tls: unknown authority" {
		t.Fatalf("reach %+v", live.Reach)
	}

	ca := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	p := writeKC(t, srv.URL, map[string]string{"prod": "    token: good\n"})
	data := "    certificate-authority-data: " + ca + "\n"
	withCA := strings.Replace(readFile(t, p), "    server: "+srv.URL+"\n", "    server: "+srv.URL+"\n"+data, 1)
	writeFile(t, p, withCA)
	live = CheckContext(nil, p, "prod")
	if !live.Reach.OK {
		t.Fatalf("with CA: %+v", live.Reach)
	}

	insecure := strings.Replace(readFile(t, p), data, "    insecure-skip-tls-verify: true\n", 1)
	writeFile(t, p, insecure)
	if live = CheckContext(nil, p, "prod"); !live.Reach.OK || live.Access.State != AccessOK {
		t.Fatalf("insecure: %+v", live)
	}
}

func TestExecPluginCredentials(t *testing.T) {
	srv := fakeAPI(t)
	user := "    exec:\n      apiVersion: client.authentication.k8s.io/v1\n      command: get-token\n      args: [--cluster, prod]\n      env:\n      - name: PLUGIN_MODE\n        value: quiet\n"
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.OK(`{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"exec-token"}}`)
	}}
	live := liveFor(t, srv.URL, user, f)
	if live.Access.State != AccessOK || live.Access.User != "alice" {
		t.Fatalf("access %+v", live.Access)
	}
	calls := f.Calls()
	if len(calls) != 1 || calls[0].Name != "get-token" || calls[0].Stdin != "" || calls[0].TTY {
		t.Fatalf("calls %+v", calls)
	}
	if dl, ok := calls[0].Ctx.Deadline(); !ok || time.Until(dl) > LiveTimeout {
		t.Fatal("exec call has no timeout")
	}
	env := strings.Join(calls[0].Env, "\n")
	if !strings.Contains(env, "PLUGIN_MODE=quiet") || !strings.Contains(env, "KUBERNETES_EXEC_INFO=") {
		t.Fatalf("env %q", env)
	}

	for name, res := range map[string]shell.Result{
		"exit":    shell.Fail(2, "boom"),
		"garbage": shell.OK("not json"),
		"empty":   shell.OK(`{"status":{}}`),
	} {
		bad := &shell.Fake{Handler: func(shell.Request) shell.Result { return res }}
		live := liveFor(t, srv.URL, user, bad)
		if !live.Reach.OK || live.Access.State != AccessExecFailed || live.Access.Reason == "" {
			t.Fatalf("%s: %+v", name, live)
		}
	}
}

func TestLiveResultHasNoCredentials(t *testing.T) {
	srv := fakeAPI(t)
	live := liveFor(t, srv.URL, "    token: good\n", nil)
	if dump := fmt.Sprintf("%+v %#v", live, live); strings.Contains(dump, "good") && strings.Contains(dump, "Bearer") {
		t.Fatalf("leak: %s", dump)
	}
}

func TestMissingServerAndContext(t *testing.T) {
	p := writeKC(t, "", map[string]string{"prod": "    token: x\n"})
	if live := CheckContext(nil, p, "prod"); live.Reach.OK || live.Reach.Reason == "" {
		t.Fatalf("no server: %+v", live)
	}
	if live := CheckContext(nil, p, "absent"); live.Reach.OK || live.Reach.Reason == "" {
		t.Fatalf("no context: %+v", live)
	}
}

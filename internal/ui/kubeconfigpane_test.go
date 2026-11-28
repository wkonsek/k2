package ui

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"k2/internal/config"

	"github.com/charmbracelet/x/ansi"
)

type kcFixture struct {
	m       model
	srv     *httptest.Server
	hits    *atomic.Int64
	config  string
	k2      string
	kindDev string
}

func kcWrite(t *testing.T, path, server, current string, ctxs ...string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: v1\nkind: Config\ncurrent-context: %s\nclusters:\n- name: shared\n  cluster:\n    server: %s\ncontexts:\n", current, server)
	for _, c := range ctxs {
		name, _, _ := strings.Cut(c, "|")
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: shared\n    user: %s-user\n", name, name)
	}
	b.WriteString("users:\n")
	for _, c := range ctxs {
		name, body, _ := strings.Cut(c, "|")
		fmt.Fprintf(&b, "- name: %s-user\n  user:\n    %s\n", name, body)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newKCFixture(t *testing.T, width int) *kcFixture {
	t.Helper()
	cfg := testEnv(t)
	hits := &atomic.Int64{}
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) })
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.Header.Get("Authorization") {
		case "Bearer good", "Bearer fresh":
			w.Write([]byte("{}"))
		case "Bearer limited":
			http.Error(w, "no", http.StatusForbidden)
		default:
			http.Error(w, "no", http.StatusUnauthorized)
		}
	})
	mux.HandleFunc("/apis/authentication.k8s.io/v1/selfsubjectreviews", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"userInfo":{"username":"alice"}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + ln.Addr().String()
	ln.Close()

	home := os.Getenv("HOME")
	f := &kcFixture{
		srv: srv, hits: hits,
		config:  filepath.Join(home, ".kube", "config"),
		k2:      filepath.Join(home, ".kube", "k2"),
		kindDev: filepath.Join(home, ".kube", "kind-dev"),
	}
	kcWrite(t, f.config, srv.URL, "prod", "prod|token: good", "staging|token: stale", "dev|token: limited")
	kcWrite(t, f.k2, srv.URL, "dev", "dev|token: sha256~oc-login-secret")
	kcWrite(t, f.kindDev, dead, "kind-dev", "kind-dev|token: good")
	oldDebounce := kcDebounce
	kcDebounce = time.Millisecond
	t.Cleanup(func() { kcDebounce = oldDebounce })
	m := newTestModel(t, cfg, nil)
	m.width, m.height = width, 40
	f.m = m
	return f
}

func (f *kcFixture) open(t *testing.T) {
	t.Helper()
	m := pressSettle(t, f.m, "C")
	if m.stage != "files" {
		t.Fatalf("stage %q", m.stage)
	}
	f.m = m
}

func (f *kcFixture) screen() string { return ansi.Strip(f.m.View()) }

func TestKubeconfigPaneShowsContextsAndSeparateSignals(t *testing.T) {
	f := newKCFixture(t, 160)
	f.m.height = 60
	f.open(t)
	view := f.screen()
	for _, want := range []string{
		"Kubeconfig", "3 contexts", "current prod",
		"▸ prod", "staging", "dev",
		"bearer token", "token present", "server", f.srv.URL,
		"● reachable", "● auth ok · alice", "● 401 unauthorized / expired", "● 403 forbidden",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "checking…") {
		t.Fatalf("checks not finished:\n%s", view)
	}

	f.m = pressSettle(t, f.m, "down")
	view = f.screen()
	if !strings.Contains(view, "bearer token (OpenShift oc login)") || !strings.Contains(view, "401 unauthorized") || !strings.Contains(view, "● reachable") {
		t.Fatalf("second file:\n%s", view)
	}
	if strings.Contains(view, "oc-login-secret") || strings.Contains(view, "sha256~") {
		t.Fatalf("token leaked:\n%s", view)
	}

	f.m = pressSettle(t, f.m, "down")
	view = f.screen()
	for _, want := range []string{"● unreachable · connection refused", "unknown", "api unreachable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("closed port: missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "401 unauthorized") {
		t.Fatalf("unreachable must not claim an access verdict:\n%s", view)
	}
}

func TestKubeconfigPaneWidths(t *testing.T) {
	for _, w := range []int{90, 120, 160, 220} {
		f := newKCFixture(t, w)
		f.open(t)
		view := f.screen()
		for i, line := range strings.Split(view, "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Fatalf("width %d: line %d is %d wide: %q", w, i, got, line)
			}
		}
		if !strings.Contains(view, ".kube/config") && !strings.Contains(view, "config") {
			t.Fatalf("width %d: list missing:\n%s", w, view)
		}
		has := strings.Contains(view, "● reachable")
		if w == 90 && has {
			t.Fatalf("width %d: pane should be hidden:\n%s", w, view)
		}
		if w >= 120 && !has {
			t.Fatalf("width %d: pane missing:\n%s", w, view)
		}
	}
}

func TestKubeconfigPaneOffSwitches(t *testing.T) {
	f := newKCFixture(t, 160)
	f.m.cfg.UI.KubeconfigPane = false
	f.open(t)
	if view := f.screen(); strings.Contains(view, "reachable") || f.hits.Load() != 0 {
		t.Fatalf("pane off still checks (%d hits):\n%s", f.hits.Load(), view)
	}

	g := newKCFixture(t, 160)
	g.m.cfg.UI.KubeconfigLive = false
	g.open(t)
	view := g.screen()
	if !strings.Contains(view, "live checks off") || strings.Contains(view, "● reachable") || g.hits.Load() != 0 {
		t.Fatalf("live off (%d hits):\n%s", g.hits.Load(), view)
	}
	if !strings.Contains(view, "bearer token") {
		t.Fatalf("static details must stay:\n%s", view)
	}
}

func TestKubeconfigScrollingDoesNotStartChecks(t *testing.T) {
	f := newKCFixture(t, 160)
	kcDebounce = time.Hour
	m := f.m
	next, cmd := m.Update(key(t, "C"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("no inspect command")
	}
	start := time.Now()
	for _, k := range []string{"down", "down", "up", "down", "up"} {
		next, _ := m.Update(key(t, k))
		m = next.(model)
		_ = m.View()
	}
	if time.Since(start) > time.Second {
		t.Fatal("scrolling blocked")
	}
	if m.kcp != nil {
		for k, e := range m.kcp.live {
			if e.running || e.done {
				t.Fatalf("check started while scrolling: %q", k)
			}
		}
	}
	if f.hits.Load() != 0 {
		t.Fatalf("%d api calls while scrolling", f.hits.Load())
	}
}

func TestKubeconfigCacheAndRefreshKey(t *testing.T) {
	f := newKCFixture(t, 160)
	f.open(t)
	first := f.hits.Load()
	if first != 3 {
		t.Fatalf("want one access check per context, got %d", first)
	}
	f.m = pressSettle(t, f.m, "down", "up")
	if got := f.hits.Load(); got != first+1 {
		t.Fatalf("only the newly visited file should be checked: %d -> %d", first, got)
	}
	f.m = pressSettle(t, f.m, "down", "up")
	if got := f.hits.Load(); got != first+1 {
		t.Fatalf("cached result was not reused: %d", got)
	}
	f.m = pressSettle(t, f.m, "r")
	if got := f.hits.Load(); got != first+4 {
		t.Fatalf("recheck did not rerun the checks: %d", got)
	}
	if !strings.Contains(f.screen(), "● auth ok · alice") {
		t.Fatalf("pane lost its status:\n%s", f.screen())
	}

	f.m.cfg.UI.KubeconfigTTL = 5
	f.m.cfg.Keys.KubeconfigRefresh = config.Binding{"ctrl+r"}
	f.m = pressSettle(t, f.m, "ctrl+r")
	if got := f.hits.Load(); got != first+7 {
		t.Fatalf("rebound refresh key: %d", got)
	}
}

func TestKubeconfigLoginUsesTheSharedFlow(t *testing.T) {
	f := newKCFixture(t, 160)
	f.open(t)
	f.m = press(t, f.m, "down")
	wantFile := f.k2
	before, _ := os.ReadFile(f.config)

	f.m = press(t, f.m, "ctrl+l")
	if f.m.dlg == nil || f.m.dlg.act != "login" || f.m.dlg.loginKC != wantFile || f.m.dlg.name != "dev" {
		t.Fatalf("dialog %s", kcDlg(f.m.dlg))
	}
	other := newKCFixture(t, 160)
	other.m.kubeconfig, other.m.context = other.config, "prod"
	other.m = press(t, other.m, "ctrl+l")
	if other.m.dlg == nil || other.m.dlg.act != "login" || other.m.dlg.loginKC != other.config || other.m.dlg.name != "prod" {
		t.Fatalf("context screen dialog %s", kcDlg(other.m.dlg))
	}

	hitsBefore := f.hits.Load()
	f.m.dlg.input.SetValue("fresh")
	next, cmd := f.m.submitLogin()
	f.m = settle(t, next.(model), cmd)

	if f.m.dlg != nil || !strings.Contains(f.m.status, "token updated") || strings.Contains(f.m.status, "fresh") {
		t.Fatalf("status %q", f.m.status)
	}
	if data, _ := os.ReadFile(f.k2); !strings.Contains(string(data), "fresh") || strings.Contains(string(data), "oc-login-secret") {
		t.Fatalf("selected file not updated:\n%s", data)
	}
	if after, _ := os.ReadFile(f.config); string(after) != string(before) {
		t.Fatal("another kubeconfig was touched")
	}
	if f.m.kubeconfig != "" {
		t.Fatalf("login must not switch the pane kubeconfig: %q", f.m.kubeconfig)
	}
	if f.hits.Load() <= hitsBefore {
		t.Fatal("cache was not invalidated: no new access check after login")
	}
	view := f.screen()
	if !strings.Contains(view, "● auth ok · alice") || strings.Contains(view, "fresh") {
		t.Fatalf("pane after login:\n%s", view)
	}

	f.m = press(t, f.m, "esc")
	f.m.stage = "files"
	f.m = press(t, f.m, ":")
	if next, _ := f.m.runCommand("login"); next.(model).dlg == nil || next.(model).dlg.act != "login" {
		t.Fatal(":login did not open in the picker")
	}
}

func TestKubeconfigLoginKeyIsConfigurable(t *testing.T) {
	f := newKCFixture(t, 160)
	f.m.cfg.Keys.Login = config.Binding{"ctrl+k"}
	f.open(t)
	f.m = press(t, f.m, "ctrl+l")
	if f.m.dlg != nil {
		t.Fatal("old key still opens login")
	}
	f.m = press(t, f.m, "ctrl+k")
	if f.m.dlg == nil || f.m.dlg.act != "login" || f.m.dlg.loginKC != f.config {
		t.Fatalf("dialog %s", kcDlg(f.m.dlg))
	}
}

func TestKubeconfigPaneTruncatesLongURL(t *testing.T) {
	got := kcTruncMid("https://api.production-cluster.region.example.com:6443/k8s/clusters/c-abc123", 40)
	if w := ansi.StringWidth(got); w != 40 || !strings.HasPrefix(got, "https://") || !strings.HasSuffix(got, "c-abc123") || !strings.Contains(got, "…") {
		t.Fatalf("%q (%d)", got, w)
	}
	if short := "https://api.dev.example.com"; kcTruncMid(short, 60) != short {
		t.Fatal("short url changed")
	}
}

func TestKubeconfigPaneUnreadableFile(t *testing.T) {
	f := newKCFixture(t, 160)
	if err := os.WriteFile(f.config, []byte("apiVersion: v1\nclusters: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if view := f.screen(); !strings.Contains(view, "cannot read") {
		t.Fatalf("no error shown:\n%s", view)
	}
}

func kcDlg(d *dialog) string {
	if d == nil {
		return "nil"
	}
	return fmt.Sprintf("act=%s kc=%s ctx=%s", d.act, d.loginKC, d.name)
}

func TestKubeconfigContextRowsAreSeparateAndAligned(t *testing.T) {
	const cluster = "api-host-example-com:6443"
	const user = "developer/api-host-example-com:6443"
	f := newKCFixture(t, 160)
	body := "apiVersion: v1\nkind: Config\ncurrent-context: prod\nclusters:\n- name: " + cluster + "\n  cluster:\n    server: https://api-host.example.com:6443\n" +
		"contexts:\n- name: prod\n  context:\n    cluster: " + cluster + "\n    user: " + user + "\n    namespace: apps\n" +
		"users:\n- name: " + user + "\n  user:\n    token: sha256~secret\n"
	if err := os.WriteFile(f.config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f.open(t)

	for _, w := range []int{80, 60, 36} {
		entry := f.m.kcp.info[f.config]
		lines := f.m.kcContextLines(f.config, entry, entry.info.Contexts[0], w)
		got := map[string]string{}
		for _, l := range lines {
			plain := ansi.Strip(l)
			if ansi.StringWidth(plain) > w {
				t.Fatalf("width %d: %q is %d wide", w, plain, ansi.StringWidth(plain))
			}
			if len(plain) > kcLabelWidth {
				if label := strings.TrimSpace(plain[:kcLabelWidth]); label != "" {
					got[label] = plain[kcLabelWidth:]
				}
			}
		}
		for _, label := range []string{"server", "cluster", "namespace", "user", "auth", "valid", "api", "access"} {
			if _, ok := got[label]; !ok {
				t.Fatalf("width %d: no %q row in %v", w, label, got)
			}
		}
		if strings.Contains(got["cluster"], "·") || strings.Contains(got["cluster"], "user") || !strings.HasPrefix(got["namespace"], "apps") {
			t.Fatalf("width %d: cluster row %q namespace %q", w, got["cluster"], got["namespace"])
		}
		switch {
		case w >= 80:
			if got["cluster"] != cluster || got["user"] != user {
				t.Fatalf("width %d: long values should fit: %q %q", w, got["cluster"], got["user"])
			}
		case w == 36:
			if !strings.Contains(got["user"], "…") || !strings.HasSuffix(got["user"], ":6443") {
				t.Fatalf("width %d: long user should be cut in the middle: %q", w, got["user"])
			}
		}
	}

	f.m = pressSettle(t, f.m, "r")
	view := f.screen()
	for _, want := range []string{"cluster   " + cluster, "namespace apps", "user      " + user} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

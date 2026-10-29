package ui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func stylePrefix(s lipgloss.Style) string {
	out := s.Render("x")
	return out[:strings.Index(out, "x")]
}

func tokenModel(t *testing.T, life kube.TokenLife, at time.Time) model {
	m := newTestModel(t, testEnv(t), nil)
	m.tok = tokenState{life: life, at: at, seen: life.Start, oshift: true, ctx: infoKey(m.kubeconfig, m.context)}
	return m
}

func TestTokenLeftFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		364 * 24 * time.Hour:            "364d",
		27 * time.Hour:                  "1d3h",
		5*time.Hour + 12*time.Minute:    "5h12m",
		42*time.Minute + 30*time.Second: "42m",
		14*time.Minute + 5*time.Second:  "14m05s",
		45 * time.Second:                "45s",
	} {
		if got := tokenLeft(d); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}

func TestTokenBarCells(t *testing.T) {
	for _, w := range []int{4, 7, 12} {
		for _, f := range []float64{0, 0.01, 0.33, 0.5, 0.999, 1} {
			full, part, empty := tokenBar(f, w)
			if n := ansi.StringWidth(full + part + empty); n != w {
				t.Fatalf("frac %v width %d: %d cells", f, w, n)
			}
		}
	}

	if full, part, _ := tokenBar(0.53, 12); full != strings.Repeat("█", 6) || part != "▎" {
		t.Fatalf("eighths: %q %q", full, part)
	}
}

func TestTokenRows(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	day := kube.TokenLife{Known: true, Start: t0, End: t0.Add(24 * time.Hour)}
	cases := []struct {
		at    time.Time
		style lipgloss.Style
		text  string
	}{
		{t0.Add(6 * time.Hour), okStyle, "18h00m"},
		{t0.Add(19 * time.Hour), warnStyle, "5h00m"},
		{t0.Add(22*time.Hour + 30*time.Minute), errStyle, "1h30m"},
		{t0.Add(24*time.Hour - 90*time.Second), errStyle, "1m30s"},
	}
	for _, c := range cases {
		m := tokenModel(t, day, c.at)
		for _, inner := range []int{18, 22, 26, 40} {
			out := m.tokenRows(inner)
			plain := ansi.Strip(out)
			if !strings.HasPrefix(plain, "token") || !strings.Contains(plain, c.text) || !strings.Contains(out, stylePrefix(c.style)) {
				t.Errorf("at %v inner %d: %q", c.at, inner, plain)
			}
			for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				if w := ansi.StringWidth(line); w > inner {
					t.Errorf("inner %d: row is %d cells: %q", inner, w, line)
				}
			}
			if inner >= 26 && !strings.ContainsAny(plain, "█░") {
				t.Errorf("inner %d: no bar in %q", inner, plain)
			}
		}
	}

	week := kube.TokenLife{Known: true, Start: t0, End: t0.Add(7 * 24 * time.Hour)}
	m := tokenModel(t, week, week.End.Add(-50*time.Minute))
	if out := m.tokenRows(28); !strings.Contains(out, stylePrefix(warnStyle)) {
		t.Errorf("< 1h should be yellow: %q", out)
	}

	want := "token       expired\n            ^l or :login\n"
	m = tokenModel(t, day, day.End.Add(time.Minute))
	if plain := ansi.Strip(m.tokenRows(40)); plain != want {
		t.Errorf("expired: %q", plain)
	}

	m = tokenModel(t, kube.TokenLife{Expired: true}, t0)
	if plain := ansi.Strip(m.tokenRows(40)); plain != want {
		t.Errorf("rejected token: %q", plain)
	}
	want = "token       invalid\n            ^l or :login\n"
	m = tokenModel(t, kube.TokenLife{Invalid: true}, t0)
	if plain := ansi.Strip(m.tokenRows(40)); plain != want {
		t.Errorf("invalid token: %q", plain)
	}

	m = tokenModel(t, day, t0)
	m.context = "other"
	if out := m.tokenRows(40); out != "" {
		t.Errorf("row of the previous context: %q", ansi.Strip(out))
	}

	idle := day
	idle.Idle = 8 * time.Hour
	m = tokenModel(t, idle, t0.Add(time.Hour))
	if plain := ansi.Strip(m.tokenRows(40)); !strings.Contains(plain, "idle limit 8h00m") || !strings.Contains(plain, "23h00m") {
		t.Errorf("idle: %q", plain)
	}

	m = tokenModel(t, kube.TokenLife{}, t0)
	if m.tokenRows(40) != "" {
		t.Error("unknown expiry must show nothing")
	}
	m = tokenModel(t, day, t0)
	m.cfg.UI.TokenBar = false
	if m.tokenRows(40) != "" || strings.Contains(m.infoBlock(40), "token") {
		t.Error("token_bar: false must hide it")
	}
}

func TestTokenClock(t *testing.T) {
	t0 := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	cur := t0
	old := now
	now = func() time.Time { return cur }
	t.Cleanup(func() { now = old })
	m := tokenModel(t, kube.TokenLife{Known: true, Start: t0, End: t0.Add(time.Hour)}, t0)
	cur = t0.Add(5 * time.Second)
	m.tokenClock()
	if !m.tok.at.Equal(t0) {
		t.Fatal("moved before 10 s")
	}
	cur = t0.Add(10 * time.Second)
	m.tokenClock()
	if !m.tok.at.Equal(cur) {
		t.Fatal("did not move after 10 s")
	}
	cur = t0.Add(50 * time.Minute)
	m.tokenClock()
	cur = cur.Add(time.Second)
	m.tokenClock()
	if !m.tok.at.Equal(cur) {
		t.Fatal("not every second near the end")
	}
}

func writeTokenKubeconfig(t *testing.T, path, user string, mtime time.Time) {
	t.Helper()
	body := "apiVersion: v1\nkind: Config\ncurrent-context: ctx\nclusters:\n- name: c\n  cluster: {server: https://127.0.0.1:6443}\ncontexts:\n- name: ctx\n  context: {cluster: c, user: me}\nusers:\n- name: me\n  user:\n" + user
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, mtime, mtime)
}

func runToken(t *testing.T, m *model) bool {
	t.Helper()
	cmd := m.tokenCmd()
	if cmd == nil {
		return false
	}
	for _, msg := range drainWait(cmd, 2*time.Second) {
		if tm, ok := msg.(tokenMsg); ok {
			m.applyToken(tm)
		}
	}
	if m.tok.loading {
		t.Fatal("lookup never finished")
	}
	return true
}

func TestTokenLookupAndRelogin(t *testing.T) {
	cfg := testEnv(t)
	path := filepath.Join(t.TempDir(), "config")
	const secret1, secret2 = "sha256~FirstSecretToken", "sha256~SecondSecretToken"
	created := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		for _, a := range r.Args {
			if strings.HasPrefix(a, "sha256~") {
				return shell.OK(fmt.Sprintf(`{"metadata":{"name":%q,"creationTimestamp":%q},"expiresIn":86400}`, a, created))
			}
		}
		return shell.Fail(1, "no cluster in tests")
	}}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context = path, "ctx"
	t1 := time.Now().Add(-time.Minute)
	writeTokenKubeconfig(t, path, "    token: "+secret1+"\n", t1)
	calls := func() int {
		n := 0
		for _, r := range f.Calls() {
			if strings.Contains(r.Line(), "useroauthaccesstokens") {
				n++
			}
		}
		return n
	}
	if !runToken(t, &m) || calls() != 1 || !m.tok.life.Known {
		t.Fatalf("first lookup: calls %d known %v", calls(), m.tok.life.Known)
	}
	if runToken(t, &m) || calls() != 1 {
		t.Fatal("nothing changed: no new lookup")
	}

	writeTokenKubeconfig(t, path, "    token: "+secret1+"\n", t1.Add(10*time.Second))
	if !runToken(t, &m) || calls() != 1 {
		t.Fatalf("same token: calls %d", calls())
	}

	writeTokenKubeconfig(t, path, "    token: "+secret2+"\n", t1.Add(20*time.Second))
	if !runToken(t, &m) || calls() != 2 {
		t.Fatalf("re-login: calls %d", calls())
	}
	last := f.Calls()[len(f.Calls())-1].Line()
	if !strings.Contains(last, kube.OAuthTokenName(secret2)) {
		t.Fatalf("re-login asked for %s", last)
	}

	m.context = "other"
	runToken(t, &m)
	if m.tok.life.Known {
		t.Fatal("other context kept the bar")
	}
	m.context = "ctx"
	runToken(t, &m)
	m.tokenClock()

	view := m.View()
	if !strings.Contains(ansi.Strip(view), "token") {
		t.Fatalf("no token row in the sidebar:\n%s", ansi.Strip(view))
	}
	var everything strings.Builder
	everything.WriteString(view + m.lastErr.text + m.status + fmt.Sprintf("%+v %#v", m.tok, m.tok))
	for _, r := range f.Calls() {
		everything.WriteString(r.Line() + strings.Join(r.Env, " ") + r.Stdin)
	}
	for _, s := range []string{"FirstSecretToken", "SecondSecretToken"} {
		if strings.Contains(everything.String(), s) {
			t.Fatalf("token %s leaked", s)
		}
	}
}

func TestLoginPopupReplacesToken(t *testing.T) {
	const secret = "sha256~popup-token"
	path := filepath.Join(t.TempDir(), "config")
	writeTokenKubeconfig(t, path, "    token: sha256~old\n", time.Now())
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = path, "ctx"
	m = press(t, m, "ctrl+l")
	if m.dlg == nil || m.dlg.act != "login" {
		t.Fatalf("popup %+v", m.dlg)
	}
	m.dlg.input.SetValue("  " + secret + "\n")
	next, _ := m.submitLogin()
	m = next.(model)
	if m.dlg != nil || !strings.Contains(m.status, "token updated") || strings.Contains(m.status, secret) {
		t.Fatalf("status %q dlg %v", m.status, m.dlg != nil)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"`+secret+`"`) || strings.Contains(string(data), "sha256~old") {
		t.Fatalf("not replaced: %v\n%s", err, data)
	}
	next, _ = m.runCommand("login")
	m = next.(model)
	if m.dlg == nil || m.dlg.act != "login" {
		t.Fatal(" :login did not open")
	}
}

func TestTokenNothingWithoutExpiry(t *testing.T) {
	jwtNoExp := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"system:serviceaccount:a:b"}`)) + ".sig"
	jwtExp := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"iat":%d,"exp":%d}`, time.Now().Add(-time.Hour).Unix(), time.Now().Add(3*time.Hour).Unix()))) + ".sig"
	cases := map[string]struct {
		user string
		show bool
	}{
		"kind cert":   {"    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg==\n    client-key-data: LS0tLS1CRUdJTiBSU0EgS0VZLS0tLS0K\n", false},
		"exec plugin": {"    exec: {apiVersion: client.authentication.k8s.io/v1, command: aws}\n", false},
		"opaque":      {"    token: static-opaque-token\n", false},
		"jwt no exp":  {"    token: " + jwtNoExp + "\n", false},
		"oc no exp":   {"    token: sha256~neverexpires\n", false},
		"no user":     {"    {}\n", false},
		"jwt exp":     {"    token: " + jwtExp + "\n", true},
	}
	for name, c := range cases {
		f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
			return shell.OK(`{"metadata":{"name":"x","creationTimestamp":"2026-10-23T08:00:00Z"},"expiresIn":0}`)
		}}
		m := newTestModel(t, testEnv(t), f)
		m.kubeconfig, m.context = filepath.Join(t.TempDir(), "config"), "ctx"
		writeTokenKubeconfig(t, m.kubeconfig, c.user, time.Now())
		runToken(t, &m)
		got := m.tokenRows(30) != ""
		if got != c.show {
			t.Errorf("%s: shown %v want %v (%q)", name, got, c.show, ansi.Strip(m.tokenRows(30)))
		}
		switch name {
		case "kind cert", "exec plugin", "no user", "jwt exp":
			if len(f.Calls()) != 0 {
				t.Errorf("%s: kubectl was called", name)
			}
		case "opaque", "jwt no exp":
			if len(f.Calls()) != 1 || !strings.Contains(f.Calls()[0].Line(), "--raw") {
				t.Errorf("%s: want one auth probe, calls %d", name, len(f.Calls()))
			}
		}
	}
}

func TestGarbageTokenShowsInvalid(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.Fail(1, "error: You must be logged in to the server (Unauthorized)")
	}}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context = filepath.Join(t.TempDir(), "config"), "ctx"
	writeTokenKubeconfig(t, m.kubeconfig, "    token: not-a-real-token\n", time.Now())
	if !runToken(t, &m) {
		t.Fatal("no lookup")
	}
	plain := ansi.Strip(m.tokenRows(40))
	if !strings.Contains(plain, "invalid") || !strings.Contains(plain, ":login") {
		t.Fatalf("sidebar %q", plain)
	}
}

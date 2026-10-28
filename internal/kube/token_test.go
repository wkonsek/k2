package kube

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func TestOAuthTokenName(t *testing.T) {
	const want = "sha256~H_yDqQSGAh0KDUJ0z96cxWLsi6WWSL-0sw0qqYcs01k"
	if got := OAuthTokenName("sha256~abcDEF123"); got != want {
		t.Fatalf("name %q, want %q", got, want)
	}
	if got := (Token{secret: "sha256~abcDEF123"}).OAuthName(); got != want {
		t.Fatalf("Token.OAuthName %q", got)
	}
	if (Token{secret: "eyJ.x.y"}).OAuthName() != "" {
		t.Fatal("a JWT has no OAuth object name")
	}
}

func jwt(payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(payload)) + ".sig"
}

func TestJWTLife(t *testing.T) {
	l := JWTLife(Token{secret: jwt(`{"iat":1700000000,"exp":1700003600,"sub":"me"}`)})
	if !l.Known || l.End.Unix() != 1700003600 || l.Start.Unix() != 1700000000 {
		t.Fatalf("life %+v", l)
	}
	l = JWTLife(Token{secret: jwt(`{"nbf":1700000100,"exp":1700003600}`)})
	if !l.Known || l.Start.Unix() != 1700000100 {
		t.Fatalf("nbf start %+v", l)
	}
	l = JWTLife(Token{secret: jwt(`{"exp":1700003600}`)})
	if !l.Known || !l.Start.IsZero() {
		t.Fatalf("no iat: %+v", l)
	}
	for _, tok := range []string{jwt(`{"sub":"no-exp"}`), "opaque-static-token", "a.b", "a.!!!.c", jwt(`not json`)} {
		if JWTLife(Token{secret: tok}).Known {
			t.Errorf("%q should have no known expiry", tok)
		}
	}
}

func writeKubeconfig(t *testing.T, dir, users string) string {
	t.Helper()
	path := filepath.Join(dir, "config")
	body := `apiVersion: v1
kind: Config
current-context: c-token
contexts:
- name: c-token
  context: {cluster: x, user: u-token}
- name: c-file
  context: {cluster: x, user: u-file}
- name: kind-kind
  context: {cluster: x, user: kind-kind}
- name: c-exec
  context: {cluster: x, user: u-exec}
- name: c-none
  context: {cluster: x}
users:
` + users
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const testUsers = `- name: u-token
  user: {token: sha256~secretvalue}
- name: u-file
  user: {tokenFile: tok.txt}
- name: kind-kind
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg==
    client-key-data: LS0tLS1CRUdJTiBSU0EgS0VZLS0tLS0K
- name: u-exec
  user:
    exec: {apiVersion: client.authentication.k8s.io/v1, command: aws}
`

func TestTokenOf(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, testUsers)
	os.WriteFile(filepath.Join(dir, "tok.txt"), []byte("filetoken\n"), 0o600)
	if tok := TokenOf(path, "c-token"); tok.secret != "sha256~secretvalue" || !tok.OpenShift() {
		t.Fatalf("token user: %v", tok.Empty())
	}
	if tok := TokenOf(path, ""); tok.secret != "sha256~secretvalue" {
		t.Fatal("context \"\" should use current-context")
	}
	if tok := TokenOf(path, "c-file"); tok.secret != "filetoken" {
		t.Fatal("tokenFile (relative to the kubeconfig) not read")
	}

	for _, ctx := range []string{"kind-kind", "c-exec", "c-none", "nope"} {
		if !TokenOf(path, ctx).Empty() {
			t.Errorf("%s: expected no token", ctx)
		}
	}
	if !TokenOf(filepath.Join(dir, "missing"), "c-token").Empty() {
		t.Error("missing file")
	}

	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing")+string(os.PathListSeparator)+path)
	if TokenOf("", "c-token").Empty() {
		t.Error("KUBECONFIG list not read")
	}
}

func TestSetContextToken(t *testing.T) {
	dir := t.TempDir()
	const secret = "sha256~new-token-value"
	body := "# keep\napiVersion: v1\ncurrent-context: c-token\ncontexts:\n- name: c-token\n  context: {cluster: x, user: u-token}\n- name: c-file\n  context: {cluster: x, user: u-file}\n- name: c-exec\n  context: {cluster: x, user: u-exec}\nusers:\n- name: u-other\n  user: {token: leave-me}\n- name: u-token\n  user:\n    token: sha256~old # stay\n- name: u-file\n  user:\n    tokenFile: tok.txt\n- name: u-exec\n  user:\n    exec: {command: aws}\n"
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "tok.txt"), []byte("filetoken\n"), 0o600)

	got, err := SetContextToken(path, "c-token", "  "+secret+"\n")
	if err != nil || got != path {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "# keep") || !strings.Contains(text, "leave-me") || !strings.Contains(text, "# stay") {
		t.Fatalf("rewrote too much:\n%s", text)
	}
	if !strings.Contains(text, `token: "`+secret+`"`) || strings.Contains(text, "sha256~old") {
		t.Fatalf("token line:\n%s", text)
	}
	if strings.Contains(errString(err), secret) {
		t.Fatal("error leaked the token")
	}
	if tok := TokenOf(path, "c-token"); tok.secret != secret {
		t.Fatalf("read back %v", tok.Empty())
	}

	if _, err := SetContextToken(path, "c-file", secret); err != nil {
		t.Fatal(err)
	}
	file, _ := os.ReadFile(filepath.Join(dir, "tok.txt"))
	if string(file) != secret+"\n" {
		t.Fatalf("tokenFile %q", file)
	}
	if strings.Contains(string(mustRead(path)), secret) && strings.Count(string(mustRead(path)), secret) != 1 {
		t.Fatal("tokenFile secret written into the kubeconfig")
	}

	if _, err := SetContextToken(path, "c-exec", secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("exec user: %v", err)
	}
	if _, err := SetContextToken(path, "missing", secret); err == nil {
		t.Fatal("missing context")
	}
	if _, err := SetContextToken(path, "c-token", "two\nlines"); err == nil {
		t.Fatal("newline token")
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func mustRead(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

func TestTokenRedacted(t *testing.T) {
	tok := Token{secret: "sha256~supersecret"}
	type wrap struct{ T Token }
	for _, s := range []string{fmt.Sprint(tok), fmt.Sprintf("%v %+v %#v %s", tok, tok, tok, tok), fmt.Sprintf("%+v %#v", wrap{tok}, wrap{tok}), tok.Fingerprint(), tok.OAuthName()} {
		if strings.Contains(s, "supersecret") {
			t.Fatalf("token leaked: %q", s)
		}
	}
	if tok.Fingerprint() == (Token{secret: "sha256~other"}).Fingerprint() || tok.Fingerprint() == "" {
		t.Fatal("fingerprints must differ per token")
	}
}

func oauthJSON(name, created string, expiresIn, idle int) string {
	return fmt.Sprintf(`{"apiVersion":"oauth.openshift.io/v1","kind":"UserOAuthAccessToken","metadata":{"name":%q,"creationTimestamp":%q},"expiresIn":%d,"inactivityTimeoutSeconds":%d,"userName":"me"}`, name, created, expiresIn, idle)
}

func TestOAuthLife(t *testing.T) {
	tok := Token{secret: "sha256~secretvalue"}
	name := tok.OAuthName()
	cases := []struct {
		name    string
		get     shell.Result
		list    shell.Result
		known   bool
		expired bool
		end     string
		idle    time.Duration
		listHit bool
	}{
		{name: "get", get: shell.OK(oauthJSON(name, "2026-10-23T08:00:00Z", 86400, 0)), known: true, end: "2026-10-24T08:00:00Z"},
		{name: "idle", get: shell.OK(oauthJSON(name, "2026-10-23T08:00:00Z", 3600, 600)), known: true, end: "2026-10-23T09:00:00Z", idle: 10 * time.Minute},
		{name: "no expiry", get: shell.OK(oauthJSON(name, "2026-10-23T08:00:00Z", 0, 0)), known: false},
		{name: "list fallback", get: shell.Fail(1, `Error from server (Forbidden): useroauthaccesstokens.oauth.openshift.io "x" is forbidden`),
			list: shell.OK(`{"items":[` + oauthJSON("sha256~someoneelse", "2026-10-20T00:00:00Z", 60, 0) + "," + oauthJSON(name, "2026-10-23T10:00:00Z", 7200, 0) + `]}`), known: true, end: "2026-10-23T12:00:00Z", listHit: true},
		{name: "not openshift", get: shell.Fail(1, `error: the server doesn't have a resource type "useroauthaccesstokens"`), list: shell.Fail(1, "no such resource"), known: false},
		{name: "unauthorized", get: shell.Fail(1, "error: You must be logged in to the server (Unauthorized)"), expired: true},
		{name: "stderr 401", get: shell.Result{Stderr: []byte("error: the server has asked for the client to provide credentials\n"), Code: 1, Err: fmt.Errorf("exit status 1")}, expired: true},
		{name: "list unauthorized", get: shell.Fail(1, `Error from server (Forbidden): useroauthaccesstokens.oauth.openshift.io "x" is forbidden`),
			list: shell.Fail(1, "error: You must be logged in to the server (Unauthorized)"), expired: true, listHit: true},
	}
	for _, c := range cases {
		f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
			for _, a := range r.Args {
				if a == name {
					return c.get
				}
			}
			return c.list
		}}
		l := New(f).Life("/k/config", "ctx", tok)
		if l.Known != c.known {
			t.Errorf("%s: known %v", c.name, l.Known)
			continue
		}
		if l.Expired != c.expired {
			t.Errorf("%s: expired %v", c.name, l.Expired)
		}
		if c.known && l.End.UTC().Format(time.RFC3339) != c.end {
			t.Errorf("%s: end %v want %s", c.name, l.End.UTC(), c.end)
		}
		if l.Idle != c.idle {
			t.Errorf("%s: idle %v", c.name, l.Idle)
		}
		for _, r := range f.Calls() {
			line := r.Line()
			if strings.Contains(line, "secretvalue") {
				t.Fatalf("%s: token in a kubectl argument: %s", c.name, line)
			}
			if !strings.Contains(line, "useroauthaccesstokens.oauth.openshift.io") || !strings.Contains(line, "--request-timeout=5s") || !strings.Contains(line, "--context ctx") {
				t.Errorf("%s: unexpected call %s", c.name, line)
			}
		}
	}

	f := &shell.Fake{}
	if New(f).Life("", "", Token{secret: jwt(`{"exp":1}`)}); len(f.Calls()) != 0 {
		t.Fatal("a JWT must be read locally")
	}
}
